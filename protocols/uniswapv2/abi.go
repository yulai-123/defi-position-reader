package uniswapv2

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const factoryABIJSON = `[
  {"inputs":[],"name":"allPairsLength","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"uint256","name":"","type":"uint256"}],"name":"allPairs","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"","type":"address"},{"internalType":"address","name":"","type":"address"}],"name":"getPair","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"feeTo","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}
]`

const pairABIJSON = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"getReserves","outputs":[{"internalType":"uint112","name":"reserve0","type":"uint112"},{"internalType":"uint112","name":"reserve1","type":"uint112"},{"internalType":"uint32","name":"blockTimestampLast","type":"uint32"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"totalSupply","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"kLast","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

const erc20StringABIJSON = `[
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

const erc20Bytes32ABIJSON = `[
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

const stakingABIJSON = `[
  {"inputs":[],"name":"stakingToken","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"rewardsToken","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"periodFinish","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"earned","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

var (
	factoryABI      = mustABI(factoryABIJSON)
	pairABI         = mustABI(pairABIJSON)
	erc20StringABI  = mustABI(erc20StringABIJSON)
	erc20Bytes32ABI = mustABI(erc20Bytes32ABIJSON)
	stakingABI      = mustABI(stakingABIJSON)
)

type reservesOutput struct {
	Reserve0           *big.Int `abi:"reserve0"`
	Reserve1           *big.Int `abi:"reserve1"`
	BlockTimestampLast uint32   `abi:"blockTimestampLast"`
}

func mustABI(value string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(value))
	if err != nil {
		panic(err)
	}
	return parsed
}

func packFactory(method string, args ...any) ([]byte, error) {
	data, err := factoryABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack factory %s: %w", method, err)
	}
	return data, nil
}

func packPair(method string, args ...any) ([]byte, error) {
	data, err := pairABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack pair %s: %w", method, err)
	}
	return data, nil
}

func packERC20(method string, args ...any) ([]byte, error) {
	data, err := erc20StringABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack erc20 %s: %w", method, err)
	}
	return data, nil
}

func packStaking(method string, args ...any) ([]byte, error) {
	data, err := stakingABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack staking %s: %w", method, err)
	}
	return data, nil
}

func unpackAllPairsLength(data []byte) (*big.Int, error) {
	return unpackBigInt(factoryABI, "allPairsLength", data)
}

func unpackAllPairs(data []byte) (common.Address, error) {
	return unpackAddress(factoryABI, "allPairs", data)
}

func unpackGetPair(data []byte) (common.Address, error) {
	return unpackAddress(factoryABI, "getPair", data)
}

func unpackFeeTo(data []byte) (common.Address, error) {
	return unpackAddress(factoryABI, "feeTo", data)
}

func unpackToken0(data []byte) (common.Address, error) {
	return unpackAddress(pairABI, "token0", data)
}

func unpackToken1(data []byte) (common.Address, error) {
	return unpackAddress(pairABI, "token1", data)
}

func unpackPairSymbol(data []byte) (string, error) {
	return unpackString(pairABI, "symbol", data)
}

func unpackPairDecimals(data []byte) (uint8, error) {
	return unpackUint8(pairABI, "decimals", data)
}

func unpackReserves(data []byte) (reservesOutput, error) {
	values, err := pairABI.Unpack("getReserves", data)
	if err != nil {
		return reservesOutput{}, fmt.Errorf("unpack getReserves: %w", err)
	}
	if len(values) != 3 {
		return reservesOutput{}, fmt.Errorf("unpack getReserves: got %d values, want 3", len(values))
	}
	reserve0, ok := values[0].(*big.Int)
	if !ok {
		return reservesOutput{}, fmt.Errorf("unpack getReserves reserve0 is %T, want *big.Int", values[0])
	}
	reserve1, ok := values[1].(*big.Int)
	if !ok {
		return reservesOutput{}, fmt.Errorf("unpack getReserves reserve1 is %T, want *big.Int", values[1])
	}
	timestamp, ok := values[2].(uint32)
	if !ok {
		return reservesOutput{}, fmt.Errorf("unpack getReserves blockTimestampLast is %T, want uint32", values[2])
	}
	return reservesOutput{
		Reserve0:           reserve0,
		Reserve1:           reserve1,
		BlockTimestampLast: timestamp,
	}, nil
}

func unpackTotalSupply(data []byte) (*big.Int, error) {
	return unpackBigInt(pairABI, "totalSupply", data)
}

func unpackPairBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(pairABI, "balanceOf", data)
}

func unpackKLast(data []byte) (*big.Int, error) {
	return unpackBigInt(pairABI, "kLast", data)
}

func unpackTokenBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(erc20StringABI, "balanceOf", data)
}

func unpackERC20Symbol(data []byte) (string, error) {
	symbol, err := unpackString(erc20StringABI, "symbol", data)
	if err == nil {
		return strings.TrimSpace(symbol), nil
	}
	values, bytes32Err := erc20Bytes32ABI.Unpack("symbol", data)
	if bytes32Err != nil {
		return "", fmt.Errorf("unpack symbol as string: %w; as bytes32: %v", err, bytes32Err)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("unpack symbol bytes32: got %d values, want 1", len(values))
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return "", fmt.Errorf("unpack symbol bytes32: value is %T, want [32]byte", values[0])
	}
	return strings.TrimRight(string(value[:]), "\x00"), nil
}

func unpackERC20Decimals(data []byte) (uint8, error) {
	decimals, err := unpackUint8(erc20StringABI, "decimals", data)
	if err == nil {
		return decimals, nil
	}
	value, uintErr := unpackBigInt(erc20Bytes32ABI, "decimals", data)
	if uintErr != nil {
		return 0, fmt.Errorf("unpack decimals as uint8: %w; as uint256: %v", err, uintErr)
	}
	if !value.IsUint64() || value.Uint64() > 255 {
		return 0, fmt.Errorf("unpack decimals: %s overflows uint8", value.String())
	}
	return uint8(value.Uint64()), nil
}

func unpackStakingToken(data []byte) (common.Address, error) {
	return unpackAddress(stakingABI, "stakingToken", data)
}

func unpackRewardsToken(data []byte) (common.Address, error) {
	return unpackAddress(stakingABI, "rewardsToken", data)
}

func unpackPeriodFinish(data []byte) (*big.Int, error) {
	return unpackBigInt(stakingABI, "periodFinish", data)
}

func unpackStakedBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(stakingABI, "balanceOf", data)
}

func unpackEarned(data []byte) (*big.Int, error) {
	return unpackBigInt(stakingABI, "earned", data)
}

func unpackString(parsed abi.ABI, method string, data []byte) (string, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return "", fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(string)
	if !ok {
		return "", fmt.Errorf("unpack %s: value is %T, want string", method, values[0])
	}
	return value, nil
}

func unpackAddress(parsed abi.ABI, method string, data []byte) (common.Address, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return common.Address{}, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("unpack %s: value is %T, want address", method, values[0])
	}
	return value, nil
}

func unpackBigInt(parsed abi.ABI, method string, data []byte) (*big.Int, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unpack %s: value is %T, want *big.Int", method, values[0])
	}
	return value, nil
}

func unpackUint8(parsed abi.ABI, method string, data []byte) (uint8, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return 0, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(uint8)
	if !ok {
		return 0, fmt.Errorf("unpack %s: value is %T, want uint8", method, values[0])
	}
	return value, nil
}
