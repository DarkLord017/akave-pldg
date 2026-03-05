package config

import (
	"github.com/ethereum/go-ethereum/common"
	"data-explorer/utils"
)

// BackfillConfig holds settings for the eth_getLogs backfill.
type BackfillConfig struct {
	RPCURL          string
	ContractAddresses []common.Address
	FromBlock       uint64
	ToBlock         uint64 // 0 means use latest
	ChunkSize       uint64 // blocks per eth_getLogs call
	MaxRetry        int
	ChainID         string // for indexing_state
}

// DefaultBackfillConfig returns config with sensible defaults.
func DefaultBackfillConfig() BackfillConfig {
	addressses , err := utils.FetchStorageContractAddresses()
	if err != nil {
		panic("Failed to fetch storage contract addresses: " + err.Error())
	}
	return BackfillConfig{
		RPCURL:          "https://c6-us.akave.ai/ext/bc/56g16Hr1SHQRzdM8JLm3GKYv7APVHY8T2TyeZLvDVzCaTRS7W/rpc",
		ContractAddresses: addressses,
		FromBlock:       0,
		ToBlock:         0,
		ChunkSize:       2000,
		MaxRetry:        5,
		ChainID:         "default",
	}
}
