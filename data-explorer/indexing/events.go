package indexing

import (
	"context"
	"data-explorer/decoding"
	"data-explorer/utils"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var contractAddress common.Address = utils.GetAddress()

func FetchAndDecode(client *ethclient.Client, fromBlock, toBlock int64) ([]*utils.DecodedEvent, error) {
	rpc := utils.NewRpcUrl(utils.GetRPCURL())
	rawChunks, err := rpc.GetLogs(context.Background(), 1, 3, int(fromBlock), int(toBlock), contractAddress, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs via RPC: %v", err)
	}

	var decodedEvents []*utils.DecodedEvent
	for _, chunk := range rawChunks {
		var logs []types.Log
		if err := json.Unmarshal(chunk, &logs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log chunk: %v", err)
		}

		for _, log := range logs {
			decoded, err := decoding.DecodeAnyLog(log)
			if err != nil {
				fmt.Printf("failed to decode log at block %d: %v\n", log.BlockNumber, err)
				continue
			}
			decodedEvents = append(decodedEvents, decoded)
		}
	}

	return decodedEvents, nil
}

func FetchAndDecodeInBatches(client *ethclient.Client, fromBlock, toBlock, batchSize int64) ([]*utils.DecodedEvent, error) {
	var allEvents []*utils.DecodedEvent

	for start := fromBlock; start <= toBlock; start += batchSize + 1 {
		end := start + batchSize
		if end > toBlock {
			end = toBlock
		}

		events, err := FetchAndDecode(client, start, end)
		if err != nil {
			return nil, fmt.Errorf("batch [%d-%d] failed: %v", start, end, err)
		}
		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}
