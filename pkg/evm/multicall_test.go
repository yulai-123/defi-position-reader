package evm

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type fakeContractCaller struct {
	t       *testing.T
	wantTo  common.Address
	wantSig []byte
	raw     []byte
}

func (c fakeContractCaller) CallContract(_ context.Context, to common.Address, data []byte) ([]byte, error) {
	if to != c.wantTo {
		c.t.Fatalf("unexpected target: got %s want %s", to.Hex(), c.wantTo.Hex())
	}
	if !bytes.Equal(data[:4], c.wantSig) {
		c.t.Fatalf("unexpected method selector: %x", data[:4])
	}
	return c.raw, nil
}

func TestMulticallerAggregate3(t *testing.T) {
	t.Parallel()

	contract, err := abi.JSON(strings.NewReader(multicall3ABI))
	if err != nil {
		t.Fatalf("parse abi: %v", err)
	}
	raw, err := contract.Methods["aggregate3"].Outputs.Pack([]multicall3Result{
		{Success: true, ReturnData: []byte{0x01, 0x02}},
		{Success: false, ReturnData: []byte{0x03}},
	})
	if err != nil {
		t.Fatalf("pack output: %v", err)
	}

	address := common.HexToAddress(Multicall3Address)
	multicaller, err := NewMulticaller(fakeContractCaller{
		t:       t,
		wantTo:  address,
		wantSig: contract.Methods["aggregate3"].ID,
		raw:     raw,
	}, address)
	if err != nil {
		t.Fatalf("new multicaller: %v", err)
	}

	results, err := multicaller.Aggregate3(context.Background(), []Call{
		{Target: common.HexToAddress("0x0000000000000000000000000000000000000001"), AllowFailure: true, CallData: []byte{0xaa}},
		{Target: common.HexToAddress("0x0000000000000000000000000000000000000002"), AllowFailure: true, CallData: []byte{0xbb}},
	})
	if err != nil {
		t.Fatalf("aggregate3: %v", err)
	}
	if len(results) != 2 || !results[0].Success || !bytes.Equal(results[0].ReturnData, []byte{0x01, 0x02}) || results[1].Success {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestMulticallerAggregate3Empty(t *testing.T) {
	t.Parallel()

	multicaller, err := NewMulticaller(fakeContractCaller{t: t}, common.HexToAddress(Multicall3Address))
	if err != nil {
		t.Fatalf("new multicaller: %v", err)
	}
	results, err := multicaller.Aggregate3(context.Background(), nil)
	if err != nil {
		t.Fatalf("aggregate empty: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results, got %#v", results)
	}
}
