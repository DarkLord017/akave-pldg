package indexing

import (
	"bytes"
	"context"
	"fmt"

	"data-explorer/config"
	"data-explorer/database"
	"data-explorer/utils"
)

// DBHandler returns a BatchEventHandler that persists blocks, decoded events,
// decoded transactions, and failed transactions to the database, then updates indexing_state.
func DBHandler(db *database.DB, chainID string, backfillCfg config.BackfillConfig) BatchEventHandler {
	rpcURL := backfillCfg.RPCURL
	maxRetry := backfillCfg.MaxRetry

	return func(ctx context.Context, events []*utils.DecodedEvent, decodedTxs []*utils.DecodedTx, failedTxs []*utils.FailedTx, chunkEndBlock int64, blocks []*utils.Block) error {
		for _, block := range blocks {
			if block.Num > 0 {
				parentBlock := db.GetBlock(ctx, block.Num-1)
				if parentBlock != nil {
					if block.ParentHash != nil && !bytes.Equal(block.ParentHash, parentBlock.Hash) {
						if err := handleReorg(ctx, db, backfillCfg, block.Num , block.Num-1); err != nil {
							return fmt.Errorf("handle reorg at block %d: %w", block.Num, err)
						}
					}
				} else {
					parentBlockData, err := utils.NewRpcUrl(rpcURL).GetBlockByNumber(ctx, 1, maxRetry, uint64(block.Num-1))
					if err != nil {
						return fmt.Errorf("fetch parent block %d: %w", block.Num-1, err)
					}

					var parentParentBlock *utils.Block
					blockNum := block.Num - 1
					for parentParentBlock == nil {
						blockNum = blockNum - 1
						parentParentBlock = db.GetBlock(ctx, blockNum)
						if parentParentBlock != nil && !bytes.Equal(parentBlockData.ParentHash, parentParentBlock.Hash) {
							if err := handleReorg(ctx, db, backfillCfg, blockNum+1, block.Num-1); err != nil {
								return fmt.Errorf("handle reorg at block %d: %w", blockNum+1, err)
							}
							break
						}
					}
				}
			}
			if err := db.InsertBlock(ctx, block); err != nil {
				return fmt.Errorf("InsertBlock %d: %w", block.Num, err)
			}
		}

		if len(decodedTxs) > 0 {
			blockHashByNum := make(map[int64][]byte, len(blocks))
			for _, b := range blocks {
				blockHashByNum[b.Num] = b.Hash
			}

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

		if len(failedTxs) > 0 {
			if err := db.InsertFailedTxBatch(ctx, failedTxs); err != nil {
				return fmt.Errorf("InsertFailedTxBatch: %w", err)
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

func handleReorg(ctx context.Context, db *database.DB, baseCfg config.BackfillConfig, fromBlock int64, toBlock int64) error {
	rpc := utils.NewRpcUrl(baseCfg.RPCURL)
	for {
		dbBlock := db.GetBlock(ctx, fromBlock-1)
		if dbBlock == nil {
			fromBlock = fromBlock - 1
			continue
		}
		block, err := rpc.GetBlockByNumber(ctx, 1, baseCfg.MaxRetry, uint64(fromBlock-1))
		if err != nil {
			return fmt.Errorf("fetch block %d: %w", fromBlock-1, err)
		}
		if bytes.Equal(block.Hash, dbBlock.Hash) {
			break
		}
		fromBlock = fromBlock - 1
	}

	if err := db.DeleteFromBlock(ctx, fromBlock); err != nil {
		return fmt.Errorf("delete blocks from %d: %w", fromBlock, err)
	}

	reorgCfg := baseCfg
	reorgCfg.FromBlock = uint64(fromBlock)
	reorgCfg.ToBlock = uint64(toBlock)
	if err := Backfill(ctx, reorgCfg, NoOpHandler, nil); err != nil {
		return fmt.Errorf("backfill after reorg from %d to %d: %w", fromBlock, toBlock, err)
	}

	return nil
}
