package indexing

import (
	"context"
	"fmt"
	"log/slog"

	"data-explorer/config"
	"data-explorer/decoding"
	"data-explorer/utils"
)

// EventHandler is called for each decoded event during backfill.
type EventHandler func(ctx context.Context, ev *utils.DecodedEvent) error

// BatchEventHandler is called with all decoded events from a chunk, the chunk end block, and block metadata for each distinct block in the chunk.
// Use chunkEndBlock for indexing_state (last fully processed block). When blocks is non-nil, the handler should insert them into the blocks table.
type BatchEventHandler func(ctx context.Context, events []*utils.DecodedEvent, chunkEndBlock int64, blocks []*utils.Block) error

// Backfill runs eth_getLogs with event topic filters over the block range,
// decodes each log, and invokes the handler. Uses chunked requests and
// automatic range splitting for large responses.
// If batchHandler is non-nil, events are batched per chunk and passed to it; otherwise handler is called per event.
func Backfill(ctx context.Context, cfg config.BackfillConfig, handler EventHandler, batchHandler BatchEventHandler) error {
	rpc := utils.NewRpcUrl(cfg.RPCURL)
	topics := utils.EventTopicFilters()

	toBlock := cfg.ToBlock
	if toBlock == 0 {
		latest, err := rpc.GetBlockNumber(ctx, 1, cfg.MaxRetry)
		if err != nil {
			return fmt.Errorf("get latest block: %w", err)
		}
		toBlock = latest
		slog.Info("backfill using latest block", "block", latest)
	}

	from := int(cfg.FromBlock)
	to := int(toBlock)
	if from > to {
		return fmt.Errorf("fromBlock %d > toBlock %d", from, to)
	}

	chunkSize := int(cfg.ChunkSize)
	if chunkSize <= 0 {
		chunkSize = 2000
	}

	slog.Info("backfill started",
		"from", from, "to", to, "chunkSize", chunkSize,
		"contract", cfg.ContractAddress.Hex())

	for start := from; start <= to; start += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := start + chunkSize - 1
		if end > to {
			end = to
		}

		rawLogs, err := rpc.GetLogs(ctx, 1, cfg.MaxRetry, start, end, cfg.ContractAddress, topics)
		if err != nil {
			return fmt.Errorf("GetLogs %d-%d: %w", start, end, err)
		}

		var batch []*utils.DecodedEvent
		for i, raw := range rawLogs {
			vLog, err := utils.RawLogToTypesLog(raw)
			if err != nil {
				return fmt.Errorf("parse log %d in block range %d-%d: %w", i, start, end, err)
			}

			decoded, err := decoding.DecodeAnyLog(vLog)
			if err != nil {
				slog.Debug("skip non-storage log", "tx", vLog.TxHash, "topics", len(vLog.Topics), "err", err)
				continue
			}

			if batchHandler != nil {
				batch = append(batch, decoded)
			} else if handler != nil {
				if err := handler(ctx, decoded); err != nil {
					return fmt.Errorf("handler for %s block %d tx %s: %w",
						decoded.EventName, decoded.BlockNumber, decoded.TxHash, err)
				}
			}
		}

		if batchHandler != nil {
			var blocks []*utils.Block
			if len(batch) > 0 {
				blockNums := make(map[uint64]struct{})
				for _, ev := range batch {
					blockNums[ev.BlockNumber] = struct{}{}
				}
				blocks = make([]*utils.Block, 0, len(blockNums))
				for num := range blockNums {
					b, err := rpc.GetBlockByNumber(ctx, 1, cfg.MaxRetry, num)
					if err != nil {
						return fmt.Errorf("get block %d: %w", num, err)
					}
					blocks = append(blocks, b)
				}
			}
			if err := batchHandler(ctx, batch, int64(end), blocks); err != nil {
				return fmt.Errorf("batch handler for blocks %d-%d: %w", start, end, err)
			}
		}

		slog.Info("backfill chunk done", "from", start, "to", end, "logs", len(rawLogs))
	}

	slog.Info("backfill completed", "from", from, "to", to)
	return nil
}

// NoOpHandler is a no-op EventHandler for testing or dry runs.
func NoOpHandler(ctx context.Context, ev *utils.DecodedEvent) error {
	return nil
}

// LoggingHandler logs each decoded event and passes through.
func LoggingHandler(ctx context.Context, ev *utils.DecodedEvent) error {
	slog.Info("event",
		"name", ev.EventName,
		"block", ev.BlockNumber,
		"tx", ev.TxHash,
		"logIndex", ev.LogIndex,
		"data", ev.Data)
	return nil
}

// LoggingBatchHandler logs each event in the batch.
func LoggingBatchHandler(ctx context.Context, events []*utils.DecodedEvent, _ int64, _ []*utils.Block) error {
	for _, ev := range events {
		slog.Info("event",
			"name", ev.EventName,
			"block", ev.BlockNumber,
			"tx", ev.TxHash,
			"logIndex", ev.LogIndex,
			"data", ev.Data)
	}
	return nil
}
