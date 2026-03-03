package indexing

import (
	"context"
	"fmt"

	"data-explorer/database"
	"data-explorer/utils"
)

// DBHandler returns a BatchEventHandler that persists blocks and events to the database and updates indexing_state.
func DBHandler(db *database.DB, chainID string) BatchEventHandler {
	return func(ctx context.Context, events []*utils.DecodedEvent, chunkEndBlock int64, blocks []*utils.Block) error {
		// Insert block metadata first so blocks table is populated whenever we index a block's events.
		for _, block := range blocks {
			if err := db.InsertBlock(ctx, block); err != nil {
				return fmt.Errorf("InsertBlock %d: %w", block.Num, err)
			}
		}

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

		if chunkEndBlock > 0 && chainID != "" {
			if err := db.SetLastIndexedBlock(ctx, chainID, chunkEndBlock); err != nil {
				return fmt.Errorf("SetLastIndexedBlock: %w", err)
			}
		}

		return nil
	}
}
