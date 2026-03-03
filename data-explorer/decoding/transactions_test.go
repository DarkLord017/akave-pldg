package decoding

import (
	"data-explorer/utils"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestDecodeTransaction(t *testing.T) {
	contractABI := utils.GetABI()
	contractAddress := utils.GetAddress()
	chainId := big.NewInt(1)

	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	fromAddress := crypto.PubkeyToAddress(privKey.PublicKey)

	tests := []struct {
		name      string
		setupTx   func() *types.Transaction
		expectErr bool
		check     func(*testing.T, *DecodedTx)
	}{
		{
			name: "Success - CreateBucket",
			setupTx: func() *types.Transaction {
				method, err := contractABI.MethodById(contractABI.Methods["createBucket"].ID)
				if err != nil {
					t.Fatal(err)
				}
				data, err := method.Inputs.Pack("test-bucket")
				if err != nil {
					t.Fatal(err)
				}
				txData := append(contractABI.Methods["createBucket"].ID, data...)

				tx := types.NewTransaction(0, contractAddress, big.NewInt(0), 100000, big.NewInt(1), txData)
				signer := types.LatestSignerForChainID(chainId)
				signedTx, err := types.SignTx(tx, signer, privKey)
				if err != nil {
					t.Fatal(err)
				}
				return signedTx
			},
			expectErr: false,
			check: func(t *testing.T, decoded *DecodedTx) {
				if decoded == nil {
					t.Fatal("expected decoded tx, got nil")
				}
				if decoded.MethodName != "createBucket" {
					t.Errorf("expected createBucket, got %s", decoded.MethodName)
				}
				if decoded.From != fromAddress {
					t.Errorf("expected from %s, got %s", fromAddress.Hex(), decoded.From.Hex())
				}
				if decoded.To != contractAddress {
					t.Errorf("expected to %s, got %s", contractAddress.Hex(), decoded.To.Hex())
				}
				params, ok := decoded.Params.(utils.CreateBucketTxParams)
				if !ok {
					t.Fatalf("expected CreateBucketTxParams, got %T", decoded.Params)
				}
				if params.BucketName != "test-bucket" {
					t.Errorf("expected bucketName test-bucket, got %v", params.BucketName)
				}
			},
		},
		{
			name: "Wrong contract address",
			setupTx: func() *types.Transaction {
				tx := types.NewTransaction(0, common.HexToAddress("0x123"), big.NewInt(0), 100000, big.NewInt(1), []byte{1, 2, 3, 4})
				return tx
			},
			expectErr: false,
			check: func(t *testing.T, decoded *DecodedTx) {
				if decoded != nil {
					t.Errorf("expected nil for wrong address, got %v", decoded)
				}
			},
		},
		{
			name: "No method selector",
			setupTx: func() *types.Transaction {
				tx := types.NewTransaction(0, contractAddress, big.NewInt(0), 100000, big.NewInt(1), []byte{1, 2, 3})
				return tx
			},
			expectErr: true,
			check: func(t *testing.T, decoded *DecodedTx) {
				if decoded != nil {
					t.Errorf("expected nil for error case, got %v", decoded)
				}
			},
		},
		{
			name: "Unknown method",
			setupTx: func() *types.Transaction {
				tx := types.NewTransaction(0, contractAddress, big.NewInt(0), 100000, big.NewInt(1), []byte{1, 2, 3, 4, 5})
				return tx
			},
			expectErr: true,
			check: func(t *testing.T, decoded *DecodedTx) {
				if decoded != nil {
					t.Errorf("expected nil for error case, got %v", decoded)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := tt.setupTx()
			decoded, err := DecodeTransaction(tx)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
			if tt.check != nil {
				tt.check(t, decoded)
			}
		})
	}
}
