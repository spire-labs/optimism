package derive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"
)

// BlobDataSource fetches blobs or calldata as appropriate and transforms them into usable rollup
// data.
type AggregatedBlobDataSource struct {
	data         []blobOrCalldata
	ref          eth.L1BlockRef
	batcherAddr  common.Address // this is the original sender of the batcher tx
	dsCfg        DataSourceConfig
	fetcher      L1TransactionFetcher
	blobsFetcher L1BlobsFetcher
	log          log.Logger
}

// NewBlobDataSource creates a new blob data source.
func NewAggregatedBlobDataSource(ctx context.Context, log log.Logger, dsCfg DataSourceConfig, fetcher L1TransactionFetcher, blobsFetcher L1BlobsFetcher, ref eth.L1BlockRef, batcherAddr common.Address) DataIter {
	return &AggregatedBlobDataSource{
		ref:          ref,
		dsCfg:        dsCfg,
		fetcher:      fetcher,
		log:          log.New("origin", ref),
		batcherAddr:  batcherAddr,
		blobsFetcher: blobsFetcher,
	}
}

// Next returns the next piece of batcher data, or an io.EOF error if no data remains. It returns
// ResetError if it cannot find the referenced block or a referenced blob, or TemporaryError for
// any other failure to fetch a block or blob.
func (ds *AggregatedBlobDataSource) Next(ctx context.Context) (eth.Data, error) {
	if ds.data == nil {
		var err error
		if ds.data, err = ds.open(ctx); err != nil {
			return nil, err
		}
	}

	if len(ds.data) == 0 {
		return nil, io.EOF
	}

	next := ds.data[0]
	ds.data = ds.data[1:]
	if next.calldata != nil {
		return *next.calldata, nil
	}

	data, err := next.blob.ToData()
	if err != nil {
		ds.log.Error("ignoring blob due to parse failure", "err", err)
		return ds.Next(ctx)
	}
	return data, nil
}

// open fetches and returns the blob from all valid batcher
// transactions in the referenced block. Returns an empty (non-nil) array if no batcher
// transactions are found. It returns ResetError if it cannot find the referenced block or a
// referenced blob, or TemporaryError for any other failure to fetch a block or blob.
func (ds *AggregatedBlobDataSource) open(ctx context.Context) ([]blobOrCalldata, error) {
	var data []blobOrCalldata
	_, txs, err := ds.fetcher.InfoAndTxsByHash(ctx, ds.ref.Hash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil, NewResetError(fmt.Errorf("failed to open blob data source: %w", err))
		}
		return nil, NewTemporaryError(fmt.Errorf("failed to open blob data source: %w", err))
	}

	_, receipts, err := ds.fetcher.FetchReceipts(ctx, ds.ref.Hash)
	if err != nil {
		return nil, NewTemporaryError(fmt.Errorf("failed to open block receipts: %w", err))
	}

	var versionHashes []eth.IndexedBlobHash
	blobIndex := 0 // index of each blob in the block's blob sidecar
	for i, tx := range txs {
		if tx.To() == nil ||
			(*tx.To() != ds.dsCfg.batchInboxAddress &&
				*tx.To() != *ds.dsCfg.blobAggregatorInboxAddress) {
			continue
		}

		// transactions can still be sent to the mempool directly by the batcher
		// it must be signed by the batcher or the blob aggregation service
		if !isValidBatchTx(tx, ds.dsCfg.l1Signer, *ds.dsCfg.blobAggregatorInboxAddress, *ds.dsCfg.blobAggregatorSenderAddress, ds.log) &&
			!isValidBatchTx(tx, ds.dsCfg.l1Signer, ds.dsCfg.batchInboxAddress, ds.batcherAddr, ds.log) {
			blobIndex += len(tx.BlobHashes())
			continue
		}

		logs := receipts[i].Logs

		contractABI := `[{"type":"event","name":"BlobSubmitted","inputs":[{"name":"_target","type":"address","indexed":false,"internalType":"address"},{"name":"_versionedHashes","type":"bytes32[]","indexed":false,"internalType":"bytes32[]"}],"anonymous":false}]`
		parsedABI, _ := abi.JSON(strings.NewReader(contractABI))

		for _, l := range logs {
			// the event must be emitted by the batcherAddr ie the proposer
			if l.Address == ds.batcherAddr {
				var event struct {
					Target          common.Address
					VersionedHashes []common.Hash
				}

				_ = parsedABI.UnpackIntoInterface(&event, "BlobSubmitted", l.Data)

				if event.Target != ds.dsCfg.batchInboxAddress {
					continue
				}

				for _, hash := range event.VersionedHashes {
					versionHashes = append(versionHashes, eth.IndexedBlobHash{
						Hash:  hash,
						Index: uint64(blobIndex),
					})
					blobIndex += 1
					data = append(data, blobOrCalldata{nil, nil}) // will fill in blob pointers after we download them below
				}
			}
		}

	}

	if len(versionHashes) == 0 {
		// there are no blobs to fetch so we can return immediately
		return data, nil
	}

	// download the actual blob bodies corresponding to the indexed blob hashes
	blobs, err := ds.blobsFetcher.GetBlobs(ctx, ds.ref, versionHashes)
	if errors.Is(err, ethereum.NotFound) {
		// If the L1 block was available, then the blobs should be available too. The only
		// exception is if the blob retention window has expired, which we will ultimately handle
		// by failing over to a blob archival service.
		return nil, NewResetError(fmt.Errorf("failed to fetch blobs: %w", err))
	} else if err != nil {
		return nil, NewTemporaryError(fmt.Errorf("failed to fetch blobs: %w", err))
	}

	// go back over the data array and populate the blob pointers
	if err := fillBlobPointers(data, blobs); err != nil {
		// this shouldn't happen unless there is a bug in the blobs fetcher
		return nil, NewResetError(fmt.Errorf("failed to fill blob pointers: %w", err))
	}
	return data, nil
}
