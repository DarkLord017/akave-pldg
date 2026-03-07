package decoding

import (
	"data-explorer/utils"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// normalizeArgsForJSON converts [32]byte and [20]byte values in the map to hex
// strings so that json.Unmarshal can populate common.Hash and common.Address
// (which expect hex strings, not byte arrays).
func normalizeArgsForJSON(m map[string]interface{}) {
	for k, v := range m {
		m[k] = normalizeVal(v)
	}
}

func normalizeVal(v interface{}) interface{} {
	if v == nil {
		return v
	}
	switch val := v.(type) {
	case [32]byte:
		return common.Hash(val).Hex()
	case [20]byte:
		return common.Address(val).Hex()
	case [][32]byte:
		out := make([]string, len(val))
		for i, b := range val {
			out[i] = common.Hash(b).Hex()
		}
		return out
	case [][][32]byte:
		out := make([][]string, len(val))
		for i, inner := range val {
			innerOut := make([]string, len(inner))
			for j, b := range inner {
				innerOut[j] = common.Hash(b).Hex()
			}
			out[i] = innerOut
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, e := range val {
			out[i] = normalizeVal(e)
		}
		return out
	case map[string]interface{}:
		normalizeArgsForJSON(val)
		return val
	default:
		// Struct from ABI (e.g. FillChunkBlockArgs tuple) - recurse on fields
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Struct {
			out := make(map[string]interface{})
			typ := rv.Type()
			for i := 0; i < rv.NumField(); i++ {
				f := rv.Field(i)
				if f.CanInterface() {
					jsonTag := typ.Field(i).Tag.Get("json")
					name := strings.Split(jsonTag, ",")[0]
					if name == "" || name == "-" {
						// ABI structs use PascalCase; our target expects camelCase
						n := typ.Field(i).Name
						if len(n) > 0 {
							name = strings.ToLower(n[:1]) + n[1:]
						} else {
							name = n
						}
					}
					out[name] = normalizeVal(f.Interface())
				}
			}
			return out
		}
		return v
	}
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

func DecodeTransaction(tx *types.Transaction, from common.Address, to common.Address) (*utils.DecodedTx, error) {
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

	// Convert [32]byte and [20]byte to hex strings so json.Unmarshal works with common.Hash/common.Address
	normalizeArgsForJSON(argsMap)

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

	return &utils.DecodedTx{
		TxHash:     tx.Hash(),
		MethodName: meta.name,
		From:       from,
		To:         to,
		Params:     args,
		Value:      tx.Value(),
	}, nil
}
