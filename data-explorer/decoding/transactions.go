package decoding

import (
	"data-explorer/utils"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type DecodedTx struct {
	MethodName string         `json:"method_name"`
	From       common.Address `json:"from"`
	To         common.Address `json:"to"`
	Params     interface{}    `json:"params"`
	Value      *big.Int       `json:"value"`
}

type txMeta struct {
	name    string
	factory func() interface{}
}

var (
	txRegistry map[[4]byte]txMeta
)

func init() {
	txRegistry = make(map[[4]byte]txMeta)
	contractABI = utils.GetABI()

	factories := map[string]func() interface{}{
		"addFileChunk":     func() interface{} { return &utils.AddFileChunkTxParams{} },
		"addFileChunks":    func() interface{} { return &utils.AddFileChunksTxParams{} },
		"commitFile":       func() interface{} { return &utils.CommitFileTxParams{} },
		"createBucket":     func() interface{} { return &utils.CreateBucketTxParams{} },
		"createFile":       func() interface{} { return &utils.CreateFileTxParams{} },
		"deleteBucket":     func() interface{} { return &utils.DeleteBucketTxParams{} },
		"deleteFile":       func() interface{} { return &utils.DeleteFileTxParams{} },
		"fillChunkBlock":   func() interface{} { return &utils.FillChunkBlockTxParams{} },
		"fillChunkBlocks":  func() interface{} { return &utils.FillChunkBlocksTxParams{} },
		"initialize":       func() interface{} { return &utils.InitializeTxParams{} },
		"setAccessManager": func() interface{} { return &utils.SetAccessManagerTxParams{} },
		"setAuthority":     func() interface{} { return &utils.SetAuthorityTxParams{} },
		"upgradeToAndCall": func() interface{} { return &utils.UpgradeToAndCallTxParams{} },
	}

	for name, method := range contractABI.Methods {
		if factory, ok := factories[name]; ok {
			var selector [4]byte
			copy(selector[:], method.ID)
			txRegistry[selector] = txMeta{
				name:    name,
				factory: factory,
			}
		}
	}
}

func DecodeTransaction(tx *types.Transaction) (*DecodedTx, error) {
	var to common.Address
	txTo := tx.To()
	if txTo == nil || *txTo != utils.GetAddress() {
		return nil, nil
	}
	to = *txTo

	txData := tx.Data()
	if len(txData) < 4 {
		return nil, fmt.Errorf("no method selector")
	}

	if tx.ChainId() == nil {
		return nil, fmt.Errorf("transaction has no chain ID")
	}

	var selector [4]byte
	copy(selector[:], txData[:4])

	meta, ok := txRegistry[selector]
	if !ok {
		return nil, fmt.Errorf("unknown method")
	}

	method, err := contractABI.MethodById(selector[:])
	if err != nil {
		return nil, fmt.Errorf("failed to get method: %w", err)
	}

	argsMap := make(map[string]interface{})
	err = method.Inputs.UnpackIntoMap(argsMap, txData[4:])
	if err != nil {
		return nil, err
	}

	jsonBytes, err := json.Marshal(argsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args map: %w", err)
	}

	params := meta.factory()
	err = json.Unmarshal(jsonBytes, params)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal into %s params: %w", meta.name, err)
	}
	args := reflect.ValueOf(params).Elem().Interface()

	var from common.Address
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err = types.Sender(signer, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover sender: %w", err)
	}

	return &DecodedTx{
		MethodName: meta.name,
		From:       from,
		To:         to,
		Params:     args,
		Value:      tx.Value(),
	}, nil
}
