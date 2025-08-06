package derive

import (
	"context"
	"crypto/ecdsa"
	"io"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testlog"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/assert"
)

func TestAggregatedBlobDataSource(t *testing.T) {
	logger := testlog.Logger(t, log.LevelDebug)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(42))

	l1fetcher := &testutils.MockL1Source{}
	blobsFetcher := &testutils.MockBlobsFetcher{}

	batchInboxAddress := common.HexToAddress("0xA00000000000000000000000000000000000000A")

	blobAggregatorInboxAddress := common.HexToAddress("0xBOB00000000000000000000000000000000000B0B")

	batcherPrivateKey := testutils.InsecureRandomKey(rng)
	batcherPublicKey, _ := batcherPrivateKey.Public().(*ecdsa.PublicKey)
	batcherAddr := crypto.PubkeyToAddress(*batcherPublicKey)

	blobAggregatorPrivateKey := testutils.InsecureRandomKey(rng)
	blobAggregatorPublicKey, _ := blobAggregatorPrivateKey.Public().(*ecdsa.PublicKey)
	blobAggregatorSenderAddress := crypto.PubkeyToAddress(*blobAggregatorPublicKey)

	signer := types.NewIsthmusSigner(big.NewInt(1337))

	dsCfg := DataSourceConfig{
		l1Signer:                    signer,
		batchInboxAddress:           batchInboxAddress,
		altDAEnabled:                false,
		blobAggregatorInboxAddress:  &blobAggregatorInboxAddress,
		blobAggregatorSenderAddress: &blobAggregatorSenderAddress,
	}

	refA := testutils.RandomBlockRef(rng)

	testCases := []struct {
		name    string
		arrange func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob)
		assert  func(t *testing.T, data eth.Data, err error)
	}{
		{
			name: "Unrelated transaction with event",
			arrange: func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob) {
				tx := testutils.RandomTx(rng, big.NewInt(1), signer)
				return []*types.Transaction{tx}, types.Receipts{testutils.RandomReceipt(rng, signer, tx, 0, 0)}, nil, nil
			},
			assert: func(t *testing.T, data eth.Data, err error) {
				assert.ErrorIs(t, err, io.EOF)
				assert.Nil(t, data)
			},
		},
		{
			name: "Inbox transaction from blob aggregator address with event",
			arrange: func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob) {
				txData := &types.DynamicFeeTx{
					To: &blobAggregatorInboxAddress,
				}
				signed, err := types.SignNewTx(blobAggregatorPrivateKey, signer, txData)
				assert.NoError(t, err)

				contractABI := `[{"type":"event","name":"BlobSubmitted","inputs":[{"name":"_target","type":"address","indexed":false,"internalType":"address"},{"name":"_versionedHashes","type":"bytes32[]","indexed":false,"internalType":"bytes32[]"}],"anonymous":false}]`
				parsedABI, _ := abi.JSON(strings.NewReader(contractABI))
				versionHash := common.HexToHash("0xBEEFBEEFBEEF")

				data, _ := parsedABI.Events["BlobSubmitted"].Inputs.NonIndexed().Pack(
					batchInboxAddress,
					[]common.Hash{versionHash},
				)
				txReceipt := types.Receipt{
					Logs: []*types.Log{{
						Address: batcherAddr,
						Topics:  []common.Hash{parsedABI.Events["BlobSubmitted"].ID},
						Data:    data,
					}},
				}

				blobHashes := []eth.IndexedBlobHash{
					{
						Index: 0,
						Hash:  versionHash,
					},
				}
				testData := []byte("This is test blob data for the aggregated blob data source")
				blob := &eth.Blob{}
				err = blob.FromData(testData)
				assert.NoError(t, err)
				blobs := []*eth.Blob{blob}

				return []*types.Transaction{signed}, types.Receipts{&txReceipt}, blobHashes, blobs
			},
			assert: func(t *testing.T, data eth.Data, err error) {
				assert.NoError(t, err)
				assert.Equal(t, []byte("This is test blob data for the aggregated blob data source"), []byte(data))
			},
		},
		{
			name: "Inbox transaction from batcher address with event",
			arrange: func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob) {
				txData := &types.DynamicFeeTx{
					To: &batchInboxAddress,
				}
				signed, err := types.SignNewTx(batcherPrivateKey, signer, txData)
				assert.NoError(t, err)

				contractABI := `[{"type":"event","name":"BlobSubmitted","inputs":[{"name":"_target","type":"address","indexed":false,"internalType":"address"},{"name":"_versionedHashes","type":"bytes32[]","indexed":false,"internalType":"bytes32[]"}],"anonymous":false}]`
				parsedABI, _ := abi.JSON(strings.NewReader(contractABI))
				versionHash := common.HexToHash("0xBEEFBEEFBEEF")

				data, _ := parsedABI.Events["BlobSubmitted"].Inputs.NonIndexed().Pack(
					batchInboxAddress,
					[]common.Hash{versionHash},
				)
				txReceipt := types.Receipt{
					Logs: []*types.Log{{
						Address: batcherAddr,
						Topics:  []common.Hash{parsedABI.Events["BlobSubmitted"].ID},
						Data:    data,
					}},
				}

				blobHashes := []eth.IndexedBlobHash{
					{
						Index: 0,
						Hash:  versionHash,
					},
				}
				testData := []byte("This is test blob data for the aggregated blob data source")
				blob := &eth.Blob{}
				err = blob.FromData(testData)
				assert.NoError(t, err)
				blobs := []*eth.Blob{blob}

				return []*types.Transaction{signed}, types.Receipts{&txReceipt}, blobHashes, blobs
			},
			assert: func(t *testing.T, data eth.Data, err error) {
				assert.NoError(t, err)
				assert.Equal(t, []byte("This is test blob data for the aggregated blob data source"), []byte(data))
			},
		},
		{
			name: "Inbox transaction from batcher address",
			arrange: func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob) {
				txData := &types.DynamicFeeTx{
					To: &batcherAddr,
				}
				signed, _ := types.SignNewTx(batcherPrivateKey, signer, txData)

				return []*types.Transaction{signed}, types.Receipts{{}}, nil, nil
			},
			assert: func(t *testing.T, data eth.Data, err error) {
				assert.ErrorIs(t, err, io.EOF)
				assert.Nil(t, data)
			},
		},
		{
			name: "Inbox transaction from blob aggregator with no blobs",
			arrange: func() ([]*types.Transaction, types.Receipts, []eth.IndexedBlobHash, []*eth.Blob) {
				txData := &types.DynamicFeeTx{
					To: &blobAggregatorSenderAddress,
				}
				signed, _ := types.SignNewTx(batcherPrivateKey, signer, txData)

				return []*types.Transaction{signed}, types.Receipts{{}}, nil, nil
			},
			assert: func(t *testing.T, data eth.Data, err error) {
				assert.ErrorIs(t, err, io.EOF)
				assert.Nil(t, data)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blobDataSource := NewAggregatedBlobDataSource(ctx, logger, dsCfg, l1fetcher, blobsFetcher, refA, batcherAddr)

			txs, receipts, indexedBlobHashes, blobs := tc.arrange()

			blockInfo := testutils.RandomBlockInfo(rng)

			l1fetcher.ExpectInfoAndTxsByHash(refA.Hash, blockInfo, txs, nil)
			l1fetcher.ExpectFetchReceipts(refA.Hash, blockInfo, receipts, nil)
			blobsFetcher.ExpectOnGetBlobs(
				context.Background(),
				refA,
				indexedBlobHashes,
				blobs,
				nil,
			)

			data, err := blobDataSource.Next(ctx)
			tc.assert(t, data, err)
		})
	}

}
