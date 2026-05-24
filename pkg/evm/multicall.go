package evm

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const Multicall3Address = "0xcA11bde05977b3631167028862bE2a173976CA11"

const multicall3ABI = `[
  {
    "inputs": [
      {
        "components": [
          {"internalType": "address", "name": "target", "type": "address"},
          {"internalType": "bool", "name": "allowFailure", "type": "bool"},
          {"internalType": "bytes", "name": "callData", "type": "bytes"}
        ],
        "internalType": "struct Multicall3.Call3[]",
        "name": "calls",
        "type": "tuple[]"
      }
    ],
    "name": "aggregate3",
    "outputs": [
      {
        "components": [
          {"internalType": "bool", "name": "success", "type": "bool"},
          {"internalType": "bytes", "name": "returnData", "type": "bytes"}
        ],
        "internalType": "struct Multicall3.Result[]",
        "name": "returnData",
        "type": "tuple[]"
      }
    ],
    "stateMutability": "payable",
    "type": "function"
  }
]`

type Multicaller struct {
	caller   ContractCaller
	address  common.Address
	contract abi.ABI
}

type Call struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type Result struct {
	Success    bool
	ReturnData []byte
}

type multicall3Call struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type multicall3Result struct {
	Success    bool
	ReturnData []byte
}

func NewMulticaller(caller ContractCaller, address common.Address) (*Multicaller, error) {
	if caller == nil {
		return nil, fmt.Errorf("multicall caller is nil")
	}
	if address == (common.Address{}) {
		address = common.HexToAddress(Multicall3Address)
	}
	contract, err := abi.JSON(strings.NewReader(multicall3ABI))
	if err != nil {
		return nil, fmt.Errorf("parse multicall abi: %w", err)
	}
	return &Multicaller{
		caller:   caller,
		address:  address,
		contract: contract,
	}, nil
}

func (m *Multicaller) Aggregate3(ctx context.Context, calls []Call) ([]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return nil, nil
	}
	if m == nil || m.caller == nil {
		return nil, fmt.Errorf("multicaller is nil")
	}

	packedCalls := make([]multicall3Call, len(calls))
	for i, call := range calls {
		packedCalls[i] = multicall3Call{
			Target:       call.Target,
			AllowFailure: call.AllowFailure,
			CallData:     call.CallData,
		}
	}

	data, err := m.contract.Pack("aggregate3", packedCalls)
	if err != nil {
		return nil, fmt.Errorf("pack aggregate3: %w", err)
	}
	raw, err := m.caller.CallContract(ctx, m.address, data)
	if err != nil {
		return nil, fmt.Errorf("call aggregate3: %w", err)
	}

	var decoded []multicall3Result
	if err := m.contract.UnpackIntoInterface(&decoded, "aggregate3", raw); err != nil {
		return nil, fmt.Errorf("unpack aggregate3: %w", err)
	}
	results := make([]Result, len(decoded))
	for i, item := range decoded {
		results[i] = Result{
			Success:    item.Success,
			ReturnData: item.ReturnData,
		}
	}
	return results, nil
}
