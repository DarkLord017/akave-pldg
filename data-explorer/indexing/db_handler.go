package indexing

import (
	"context"
	"fmt"

	"data-explorer/database"
	"data-explorer/utils"
)

// DBHandler returns a BatchEventHandler that persists blocks, decoded events,
// decoded transactions, and failed transactions to the database, then updates indexing_state.
func DBHandler(db *database.DB, chainID string) BatchEventHandler {
	return func(ctx context.Context, events []*utils.DecodedEvent, decodedTxs []*utils.DecodedTx, failedTxs []*utils.FailedTx, chunkEndBlock int64, blocks []*utils.Block) error {
		// 1. Insert block metadata so the blocks table is always populated.
		for _, block := range blocks {
			if err := db.InsertBlock(ctx, block); err != nil {
				return fmt.Errorf("InsertBlock %d: %w", block.Num, err)
			}
		}

		// 2. Persist decoded transactions grouped by block.
		if len(decodedTxs) > 0 {
			// Build a hash lookup so we can pass blockHash to InsertTransactionBatch.
			blockHashByNum := make(map[int64][]byte, len(blocks))
			for _, b := range blocks {
				blockHashByNum[b.Num] = b.Hash
			}

			// Group txs by block number.
			byBlock := make(map[int64][]*utils.DecodedTx)
			for _, tx := range decodedTxs {
				byBlock[tx.BlockNum] = append(byBlock[tx.BlockNum], tx)
			}

			for blockNum, txsForBlock := range byBlock {
				blockHash := blockHashByNum[blockNum]
				if err := db.InsertTransactionBatch(ctx, blockNum, blockHash, txsForBlock); err != nil {
					return fmt.Errorf("InsertTransactionBatch block %d: %w", blockNum, err)
				}
			}
		}

		// 3. Persist failed transactions so they can be retried later.
		if len(failedTxs) > 0 {
			if err := db.InsertFailedTxBatch(ctx, failedTxs); err != nil {
				return fmt.Errorf("InsertFailedTxBatch: %w", err)
			}
		}

		// 4. Persist decoded events.
		if len(events) > 0 {
			eventsByTxHash := make(map[string][]*utils.DecodedEvent)
			for _, ev := range events {
				txHashStr := ev.TxHash.Hex()
				eventsByTxHash[txHashStr] = append(eventsByTxHash[txHashStr], ev)
			}

			blockGroups := make(map[int64]map[string][]*utils.DecodedEvent)
			for txHashStr, evs := range eventsByTxHash {
				blockNum := int64(evs[0].BlockNumber)
				if blockGroups[blockNum] == nil {
					blockGroups[blockNum] = make(map[string][]*utils.DecodedEvent)
				}
				blockGroups[blockNum][txHashStr] = evs
			}

			for blockNum, byTx := range blockGroups {
				if err := db.UpdateEventsBatch(ctx, blockNum, byTx); err != nil {
					return fmt.Errorf("UpdateEventsBatch block %d: %w", blockNum, err)
				}
			}
		}

		// 5. Advance the indexing cursor.
		if chunkEndBlock > 0 && chainID != "" {
			if err := db.SetLastIndexedBlock(ctx, chainID, chunkEndBlock); err != nil {
				return fmt.Errorf("SetLastIndexedBlock: %w", err)
			}
		}

		return nil
	}
}
