package derive

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const syncABI = `[{"inputs":[],"name":"sync","outputs":[{"internalType":"address[]","name":"__targets","type":"address[]"},{"internalType":"bytes[]","name":"__retdata","type":"bytes[]"},{"internalType":"bytes32[]","name":"__hashedCalldata","type":"bytes32[]"}],"stateMutability":"nonpayable","type":"function"}]`
const setStateABI = `[{"inputs":[{"internalType":"address[]","name":"_targets","type":"address[]"},{"internalType":"bytes[]","name":"_retdata","type":"bytes[]"},{"internalType":"bytes32[]","name":"_hashedCalldata","type":"bytes32[]"}],"name":"setState","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

func SyncTransactions(ctx context.Context, rollupCfg *rollup.Config, l1Client L1ReceiptsFetcher, blockNumber uint64, seqNumber uint64, hash common.Hash, l2BlockTime uint64) (hexutil.Bytes, error) {
	var syncContract common.Address
	var dep types.DepositTx
	var syncTx []byte
	// Hardcoded for now, needs to be added to rollup config
	syncContract = common.HexToAddress("0xCd271737d57b705dD49DaF3fBa20a53d05E410d8")
	addressZero := common.HexToAddress("0x0000000000000000000000000000000000000000")

	parsedABI, err := abi.JSON(strings.NewReader(syncABI))

	if err != nil {
		log.Fatalf("Failed to parse function call: %v", err)
		return nil, err
	}

	data, err := parsedABI.Pack("sync")
	if err != nil {
		log.Fatalf("Failed to pack sync function call: %v", err)
		return nil, err
	}

	callMsg := ethereum.CallMsg{
		From: addressZero,
		To:   &syncContract,
		Data: data,
		Gas:  1_000_000,
	}

	var result hexutil.Bytes

	blockAsHex := fmt.Sprintf("0x%x", blockNumber)

	args := msgToCallArgs(callMsg)

	result, err = l1Client.Call(ctx, args, blockAsHex)

	if err != nil {
		log.Fatalf("Failed to call function: %v", err)
		return nil, err
	}

	// Decode the result
	var __targets []common.Address
	var __retdata [][]byte
	var __hashedCalldata []common.Hash

	// Unpack the returned data into the three arrays
	err = parsedABI.UnpackIntoInterface(&[]interface{}{&__targets, &__retdata, &__hashedCalldata}, "sync", result)
	if err != nil {
		log.Fatalf("Failed to unpack result: %v", err)
		return nil, err
	}

	dep, err = BuildSyncTransaction(ctx, rollupCfg, seqNumber, hash, l2BlockTime, __hashedCalldata, __retdata, __targets)

	if err != nil {
		return nil, err
	}

	l2Tx := types.NewTx(&dep)

	syncTx, err = l2Tx.MarshalBinary()

	if err != nil {
		return nil, err
	}

	return syncTx, nil
}

func BuildSyncTransaction(ctx context.Context, rollupCfg *rollup.Config, seqNumber uint64, hash common.Hash, l2BlockTime uint64, hashedCalldata []common.Hash, retdata [][]byte, targets []common.Address) (types.DepositTx, error) {
	var err error
	var out types.DepositTx

	source := L1InfoDepositSource{
		L1BlockHash: hash,
		SeqNumber:   seqNumber,
	}

	address := common.HexToAddress("0x4200000000000000000000000000000000000027")
	L1InfoDepositerAddress := common.HexToAddress("0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001")

	out.To = &address
	out.From = L1InfoDepositerAddress
	out.SourceHash = source.SourceHash()
	out.Mint = nil
	out.Gas = 150_000_000
	out.Value = big.NewInt(0)
	out.IsSystemTransaction = true
	out.Data = nil

	parsedABI, err := abi.JSON(strings.NewReader(setStateABI))
	if err != nil {
		log.Fatalf("Failed to parse function call: %v", err)
		return out, err
	}

	data, err := parsedABI.Pack("setState", targets, retdata, hashedCalldata)
	if err != nil {
		log.Fatalf("Failed to pack setState function call: %v", err)
		return out, err
	}

	out.Data = data

	if rollupCfg.IsRegolith(l2BlockTime) {
		out.IsSystemTransaction = false
		out.Gas = RegolithSystemTxGas
	}

	return out, nil
}

func msgToCallArgs(msg ethereum.CallMsg) map[string]interface{} {
	args := map[string]interface{}{
		"to":   msg.To.Hex(),
		"data": hexutil.Encode(msg.Data),
	}

	if msg.From != (common.Address{}) {
		args["from"] = msg.From.Hex()
	}
	if msg.Gas != 0 {
		args["gas"] = fmt.Sprintf("0x%x", msg.Gas)
	}
	if msg.GasPrice != nil {
		args["gasPrice"] = fmt.Sprintf("0x%x", msg.GasPrice)
	}
	if msg.Value != nil {
		args["value"] = fmt.Sprintf("0x%x", msg.Value)
	}

	return args
}
