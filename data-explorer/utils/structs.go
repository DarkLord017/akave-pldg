package utils

import (
	"encoding/json"
	"math/big"

	"time"

	"github.com/ethereum/go-ethereum/common"
)

type RpcUrl struct {
	Url string
}

func (r *RpcUrl) GetUrl() string {
	return r.Url
}

func NewRpcUrl(url string) *RpcUrl {
	return &RpcUrl{
		Url: url,
	}
}

type Block struct {
	Num        int64
	Hash       []byte
	ParentHash []byte
	Timestamp  time.Time
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *JSONRPCError   `json:"error"`
	ID      int             `json:"id"`
}

// Structs as per ABI specifications

// Event structs

type CreateBucketEvent struct {
	Id    common.Hash    `json:"id"`
	Name  common.Hash    `json:"name"`
	Owner common.Address `json:"owner"`
}

type CreateFileEvent struct {
	Id       common.Hash    `json:"id"`
	BucketId common.Hash    `json:"bucket_id"`
	Name     common.Hash    `json:"name"`
	Owner    common.Address `json:"owner"`
}

type AddFileChunkEvent struct {
	Id       common.Hash    `json:"id"`
	BucketId common.Hash    `json:"bucket_id"`
	Name     common.Hash    `json:"name"`
	Owner    common.Address `json:"owner"`
}

type CommitFileEvent struct {
	Id       common.Hash    `json:"id"`
	BucketId common.Hash    `json:"bucket_id"`
	Name     common.Hash    `json:"name"`
	Owner    common.Address `json:"owner"`
}

type FillChunkBlockEvent struct {
	FileId     common.Hash `json:"file_id"`
	ChunkIndex *big.Int    `json:"chunk_index"`
	BlockIndex *big.Int    `json:"block_index"`
	BlockCID   common.Hash `json:"block_cid"`
	NodeId     common.Hash `json:"node_id"`
}

type AddFileBlocksEvent struct {
	Ids    common.Hash `json:"ids"`
	FileId common.Hash `json:"file_id"`
}

type AddPeerBlockEvent struct {
	BlockId common.Hash `json:"block_id"`
	PeerId  common.Hash `json:"peer_id"`
}

type DeleteBucketEvent struct {
	Id    common.Hash    `json:"id"`
	Name  common.Hash    `json:"name"`
	Owner common.Address `json:"owner"`
}

type DeletePeerBlockEvent struct {
	BlockId common.Hash `json:"block_id"`
	PeerId  common.Hash `json:"peer_id"`
}

type DeleteFileEvent struct {
	Id       common.Hash    `json:"id"`
	BucketId common.Hash    `json:"bucket_id"`
	Name     common.Hash    `json:"name"`
	Owner    common.Address `json:"owner"`
}

type EIP712DomainChangedEvent struct {
}

type InitializedEvent struct {
	Version uint64 `json:"version"`
}

type UpgradedEvent struct {
	Implementation common.Address `json:"implementation"`
}

type DecodedEvent struct {
	EventName       string                 `json:"event_name"`
	ContractAddress common.Address         `json:"contract_address"`
	BlockNumber     uint64                 `json:"block_number"`
	BlockHash       common.Hash            `json:"block_hash"`
	TxHash          common.Hash            `json:"tx_hash"`
	LogIndex        uint                   `json:"log_index"`
	Topics          []common.Hash          `json:"topics"`
	Data            map[string]interface{} `json:"data"`
}

type EventMeta struct {
	Name    string
	Factory func() interface{}
}

// Transaction Structs

type DecodedTx struct {
	TxHash     common.Hash    `json:"tx_hash"`
	MethodName string         `json:"method_name"` // placeholder for now
	From       common.Address `json:"from"`
	To         common.Address `json:"to"`
	Params     interface{}    `json:"params"`
	Value      *big.Int       `json:"value"`
}

type AddFileChunkTxParams struct {
	ChunkCID         []byte        `json:"chunkCID"`
	BucketId         common.Hash   `json:"bucketId"`
	FileName         string        `json:"fileName"`
	EncodedChunkSize *big.Int      `json:"encodedChunkSize"`
	Cids             []common.Hash `json:"cids"`
	ChunkBlocksSizes []*big.Int    `json:"chunkBlocksSizes"`
	ChunkIndex       *big.Int      `json:"chunkIndex"`
}

type AddFileChunksTxParams struct {
	Cids               [][]byte        `json:"cids"`
	BucketId           common.Hash     `json:"bucketId"`
	FileName           string          `json:"fileName"`
	EncodedChunkSizes  []*big.Int      `json:"encodedChunkSizes"`
	ChunkBlocksCIDs    [][]common.Hash `json:"chunkBlocksCIDs"`
	ChunkBlockSizes    [][]*big.Int    `json:"chunkBlockSizes"`
	StartingChunkIndex *big.Int        `json:"startingChunkIndex"`
}

type CommitFileTxParams struct {
	BucketId        common.Hash `json:"bucketId"`
	FileName        string      `json:"fileName"`
	EncodedFileSize *big.Int    `json:"encodedFileSize"`
	ActualSize      *big.Int    `json:"actualSize"`
	FileCID         []byte      `json:"fileCID"`
}

type CreateBucketTxParams struct {
	BucketName string `json:"bucketName"`
}

type CreateFileTxParams struct {
	BucketId common.Hash `json:"bucketId"`
	FileName string      `json:"fileName"`
}

type DeleteBucketTxParams struct {
	Id         common.Hash `json:"id"`
	BucketName string      `json:"bucketName"`
	Index      *big.Int    `json:"index"`
}

type DeleteFileTxParams struct {
	FileID   common.Hash `json:"fileID"`
	BucketId common.Hash `json:"bucketId"`
	FileName string      `json:"fileName"`
	Index    *big.Int    `json:"index"`
}

type FillChunkBlockArgs struct {
	BlockCID   common.Hash `json:"blockCID"`
	NodeId     common.Hash `json:"nodeId"`
	BucketId   common.Hash `json:"bucketId"`
	ChunkIndex *big.Int    `json:"chunkIndex"`
	Nonce      *big.Int    `json:"nonce"`
	BlockIndex uint8       `json:"blockIndex"`
	FileName   string      `json:"fileName"`
	Signature  []byte      `json:"signature"`
	Deadline   *big.Int    `json:"deadline"`
}

type FillChunkBlockTxParams struct {
	Args FillChunkBlockArgs `json:"fillChunkBlockArgs"`
}

type FillChunkBlocksTxParams struct {
	Args []FillChunkBlockArgs `json:"fillChunkBlocksArgs"`
}

type InitializeTxParams struct {
	TokenAddress common.Address `json:"tokenAddress"`
}

type SetAccessManagerTxParams struct {
	AccessManagerAddress common.Address `json:"accessManagerAddress"`
}

type SetAuthorityTxParams struct {
	UpgradeAuthority common.Address `json:"_upgradeAuthority"`
}

type UpgradeToAndCallTxParams struct {
	NewImplementation common.Address `json:"newImplementation"`
	Data              []byte         `json:"data"`
}
