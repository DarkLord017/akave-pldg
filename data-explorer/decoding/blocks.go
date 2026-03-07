package decoding

import (
	"fmt"
	"time"

	"data-explorer/utils"

	"github.com/ethereum/go-ethereum/core/types"
)

func DecodeBlock(b *types.Block) (*utils.DecodedBlock, error) {
	if b == nil {
		return nil, fmt.Errorf("nil block")
	}

	block := &utils.Block{
		Num:        int64(b.NumberU64()),
		Hash:       b.Hash().Bytes(),
		ParentHash: b.ParentHash().Bytes(),
		Timestamp:  time.Unix(int64(b.Time()), 0),
	}

	var decodedTxs []*utils.DecodedTx
	for _, tx := range b.Transactions() {
		from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
		if err != nil {
			continue
		}
		to := tx.To()
		dtx, err := DecodeTransaction(tx, from, *to)
		if err != nil {
			// For block-level decoding we treat per-tx decode failures as non-fatal
			// and simply skip those transactions.
			continue
		}
		if dtx == nil {
			// tx not targeting our storage contract
			continue
		}
		decodedTxs = append(decodedTxs, dtx)
	}

	return &utils.DecodedBlock{
		Block: block,
		Txs:   decodedTxs,
	}, nil
}
