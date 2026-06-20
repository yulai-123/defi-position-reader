package uniswapv3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const factoryABIJSON = `[
  {"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"token0","type":"address"},{"indexed":true,"internalType":"address","name":"token1","type":"address"},{"indexed":true,"internalType":"uint24","name":"fee","type":"uint24"},{"indexed":false,"internalType":"int24","name":"tickSpacing","type":"int24"},{"indexed":false,"internalType":"address","name":"pool","type":"address"}],"name":"PoolCreated","type":"event"},
  {"inputs":[{"internalType":"address","name":"tokenA","type":"address"},{"internalType":"address","name":"tokenB","type":"address"},{"internalType":"uint24","name":"fee","type":"uint24"}],"name":"getPool","outputs":[{"internalType":"address","name":"pool","type":"address"}],"stateMutability":"view","type":"function"}
]`

const positionManagerABIJSON = `[
  {"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"owner","type":"address"},{"internalType":"uint256","name":"index","type":"uint256"}],"name":"tokenOfOwnerByIndex","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"uint256","name":"tokenId","type":"uint256"}],"name":"positions","outputs":[{"internalType":"uint96","name":"nonce","type":"uint96"},{"internalType":"address","name":"operator","type":"address"},{"internalType":"address","name":"token0","type":"address"},{"internalType":"address","name":"token1","type":"address"},{"internalType":"uint24","name":"fee","type":"uint24"},{"internalType":"int24","name":"tickLower","type":"int24"},{"internalType":"int24","name":"tickUpper","type":"int24"},{"internalType":"uint128","name":"liquidity","type":"uint128"},{"internalType":"uint256","name":"feeGrowthInside0LastX128","type":"uint256"},{"internalType":"uint256","name":"feeGrowthInside1LastX128","type":"uint256"},{"internalType":"uint128","name":"tokensOwed0","type":"uint128"},{"internalType":"uint128","name":"tokensOwed1","type":"uint128"}],"stateMutability":"view","type":"function"}
]`

const poolABIJSON = `[
  {"inputs":[],"name":"tickSpacing","outputs":[{"internalType":"int24","name":"","type":"int24"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"slot0","outputs":[{"internalType":"uint160","name":"sqrtPriceX96","type":"uint160"},{"internalType":"int24","name":"tick","type":"int24"},{"internalType":"uint16","name":"observationIndex","type":"uint16"},{"internalType":"uint16","name":"observationCardinality","type":"uint16"},{"internalType":"uint16","name":"observationCardinalityNext","type":"uint16"},{"internalType":"uint8","name":"feeProtocol","type":"uint8"},{"internalType":"bool","name":"unlocked","type":"bool"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"liquidity","outputs":[{"internalType":"uint128","name":"","type":"uint128"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"feeGrowthGlobal0X128","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"feeGrowthGlobal1X128","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"int24","name":"tick","type":"int24"}],"name":"ticks","outputs":[{"internalType":"uint128","name":"liquidityGross","type":"uint128"},{"internalType":"int128","name":"liquidityNet","type":"int128"},{"internalType":"uint256","name":"feeGrowthOutside0X128","type":"uint256"},{"internalType":"uint256","name":"feeGrowthOutside1X128","type":"uint256"},{"internalType":"int56","name":"tickCumulativeOutside","type":"int56"},{"internalType":"uint160","name":"secondsPerLiquidityOutsideX128","type":"uint160"},{"internalType":"uint32","name":"secondsOutside","type":"uint32"},{"internalType":"bool","name":"initialized","type":"bool"}],"stateMutability":"view","type":"function"}
]`

const erc20StringABIJSON = `[
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"}
]`

const erc20Bytes32ABIJSON = `[
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

var (
	factoryABI         = mustABI(factoryABIJSON)
	positionManagerABI = mustABI(positionManagerABIJSON)
	poolABI            = mustABI(poolABIJSON)
	erc20StringABI     = mustABI(erc20StringABIJSON)
	erc20Bytes32ABI    = mustABI(erc20Bytes32ABIJSON)
	poolCreatedEvent   = factoryABI.Events["PoolCreated"]
)

type poolCreatedLog struct {
	Token0      common.Address
	Token1      common.Address
	Fee         uint32
	TickSpacing int32
	Pool        common.Address
}

type positionOutput struct {
	Token0                   common.Address
	Token1                   common.Address
	Fee                      uint32
	TickLower                int32
	TickUpper                int32
	Liquidity                *big.Int
	FeeGrowthInside0LastX128 *big.Int
	FeeGrowthInside1LastX128 *big.Int
	TokensOwed0              *big.Int
	TokensOwed1              *big.Int
}

type slot0Output struct {
	SqrtPriceX96 *big.Int
	Tick         int32
	FeeProtocol  uint8
	Unlocked     bool
}

type tickOutput struct {
	LiquidityGross        *big.Int
	LiquidityNet          *big.Int
	FeeGrowthOutside0X128 *big.Int
	FeeGrowthOutside1X128 *big.Int
	Initialized           bool
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

func packPositionManager(method string, args ...any) ([]byte, error) {
	data, err := positionManagerABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack position manager %s: %w", method, err)
	}
	return data, nil
}

func packPool(method string, args ...any) ([]byte, error) {
	data, err := poolABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack pool %s: %w", method, err)
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

func unpackGetPool(data []byte) (common.Address, error) {
	return unpackAddress(factoryABI, "getPool", data)
}

func unpackPositionBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(positionManagerABI, "balanceOf", data)
}

func unpackTokenOfOwnerByIndex(data []byte) (*big.Int, error) {
	return unpackBigInt(positionManagerABI, "tokenOfOwnerByIndex", data)
}

func unpackPosition(data []byte) (positionOutput, error) {
	values, err := positionManagerABI.Unpack("positions", data)
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions: %w", err)
	}
	if len(values) != 12 {
		return positionOutput{}, fmt.Errorf("unpack positions: got %d values, want 12", len(values))
	}
	token0, ok := values[2].(common.Address)
	if !ok {
		return positionOutput{}, fmt.Errorf("unpack positions token0 is %T, want address", values[2])
	}
	token1, ok := values[3].(common.Address)
	if !ok {
		return positionOutput{}, fmt.Errorf("unpack positions token1 is %T, want address", values[3])
	}
	fee, err := abiUint32(values[4])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions fee: %w", err)
	}
	tickLower, err := abiInt32(values[5])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions tickLower: %w", err)
	}
	tickUpper, err := abiInt32(values[6])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions tickUpper: %w", err)
	}
	liquidity, err := abiBigInt(values[7])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions liquidity: %w", err)
	}
	feeGrowth0, err := abiBigInt(values[8])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions feeGrowthInside0LastX128: %w", err)
	}
	feeGrowth1, err := abiBigInt(values[9])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions feeGrowthInside1LastX128: %w", err)
	}
	tokensOwed0, err := abiBigInt(values[10])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions tokensOwed0: %w", err)
	}
	tokensOwed1, err := abiBigInt(values[11])
	if err != nil {
		return positionOutput{}, fmt.Errorf("unpack positions tokensOwed1: %w", err)
	}
	return positionOutput{
		Token0:                   token0,
		Token1:                   token1,
		Fee:                      fee,
		TickLower:                tickLower,
		TickUpper:                tickUpper,
		Liquidity:                liquidity,
		FeeGrowthInside0LastX128: feeGrowth0,
		FeeGrowthInside1LastX128: feeGrowth1,
		TokensOwed0:              tokensOwed0,
		TokensOwed1:              tokensOwed1,
	}, nil
}

func unpackSlot0(data []byte) (slot0Output, error) {
	values, err := poolABI.Unpack("slot0", data)
	if err != nil {
		return slot0Output{}, fmt.Errorf("unpack slot0: %w", err)
	}
	if len(values) != 7 {
		return slot0Output{}, fmt.Errorf("unpack slot0: got %d values, want 7", len(values))
	}
	sqrtPrice, err := abiBigInt(values[0])
	if err != nil {
		return slot0Output{}, fmt.Errorf("unpack slot0 sqrtPriceX96: %w", err)
	}
	tick, err := abiInt32(values[1])
	if err != nil {
		return slot0Output{}, fmt.Errorf("unpack slot0 tick: %w", err)
	}
	feeProtocol, err := abiUint8(values[5])
	if err != nil {
		return slot0Output{}, fmt.Errorf("unpack slot0 feeProtocol: %w", err)
	}
	unlocked, ok := values[6].(bool)
	if !ok {
		return slot0Output{}, fmt.Errorf("unpack slot0 unlocked is %T, want bool", values[6])
	}
	return slot0Output{
		SqrtPriceX96: sqrtPrice,
		Tick:         tick,
		FeeProtocol:  feeProtocol,
		Unlocked:     unlocked,
	}, nil
}

func unpackLiquidity(data []byte) (*big.Int, error) {
	return unpackBigInt(poolABI, "liquidity", data)
}

func unpackTickSpacing(data []byte) (int32, error) {
	values, err := poolABI.Unpack("tickSpacing", data)
	if err != nil {
		return 0, fmt.Errorf("unpack tickSpacing: %w", err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("unpack tickSpacing: got %d values, want 1", len(values))
	}
	return abiInt32(values[0])
}

func unpackFeeGrowthGlobal0(data []byte) (*big.Int, error) {
	return unpackBigInt(poolABI, "feeGrowthGlobal0X128", data)
}

func unpackFeeGrowthGlobal1(data []byte) (*big.Int, error) {
	return unpackBigInt(poolABI, "feeGrowthGlobal1X128", data)
}

func unpackTick(data []byte) (tickOutput, error) {
	values, err := poolABI.Unpack("ticks", data)
	if err != nil {
		return tickOutput{}, fmt.Errorf("unpack ticks: %w", err)
	}
	if len(values) != 8 {
		return tickOutput{}, fmt.Errorf("unpack ticks: got %d values, want 8", len(values))
	}
	liquidityGross, err := abiBigInt(values[0])
	if err != nil {
		return tickOutput{}, fmt.Errorf("unpack ticks liquidityGross: %w", err)
	}
	liquidityNet, err := abiBigInt(values[1])
	if err != nil {
		return tickOutput{}, fmt.Errorf("unpack ticks liquidityNet: %w", err)
	}
	feeGrowth0, err := abiBigInt(values[2])
	if err != nil {
		return tickOutput{}, fmt.Errorf("unpack ticks feeGrowthOutside0X128: %w", err)
	}
	feeGrowth1, err := abiBigInt(values[3])
	if err != nil {
		return tickOutput{}, fmt.Errorf("unpack ticks feeGrowthOutside1X128: %w", err)
	}
	initialized, ok := values[7].(bool)
	if !ok {
		return tickOutput{}, fmt.Errorf("unpack ticks initialized is %T, want bool", values[7])
	}
	return tickOutput{
		LiquidityGross:        liquidityGross,
		LiquidityNet:          liquidityNet,
		FeeGrowthOutside0X128: feeGrowth0,
		FeeGrowthOutside1X128: feeGrowth1,
		Initialized:           initialized,
	}, nil
}

func unpackPoolCreatedLog(topics []common.Hash, data []byte) (poolCreatedLog, error) {
	if len(topics) != 4 {
		return poolCreatedLog{}, fmt.Errorf("pool created log has %d topics, want 4", len(topics))
	}
	if topics[0] != poolCreatedEvent.ID {
		return poolCreatedLog{}, fmt.Errorf("pool created log topic mismatch")
	}
	values, err := poolCreatedEvent.Inputs.NonIndexed().Unpack(data)
	if err != nil {
		return poolCreatedLog{}, fmt.Errorf("unpack PoolCreated data: %w", err)
	}
	if len(values) != 2 {
		return poolCreatedLog{}, fmt.Errorf("unpack PoolCreated data: got %d values, want 2", len(values))
	}
	tickSpacing, err := abiInt32(values[0])
	if err != nil {
		return poolCreatedLog{}, fmt.Errorf("unpack PoolCreated tickSpacing: %w", err)
	}
	pool, ok := values[1].(common.Address)
	if !ok {
		return poolCreatedLog{}, fmt.Errorf("unpack PoolCreated pool is %T, want address", values[1])
	}
	fee, err := topicUint32(topics[3])
	if err != nil {
		return poolCreatedLog{}, fmt.Errorf("unpack PoolCreated fee: %w", err)
	}
	return poolCreatedLog{
		Token0:      common.BytesToAddress(topics[1].Bytes()[12:]),
		Token1:      common.BytesToAddress(topics[2].Bytes()[12:]),
		Fee:         fee,
		TickSpacing: tickSpacing,
		Pool:        pool,
	}, nil
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
	return abiBigInt(values[0])
}

func unpackUint8(parsed abi.ABI, method string, data []byte) (uint8, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return 0, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	return abiUint8(values[0])
}

func abiBigInt(value any) (*big.Int, error) {
	switch typed := value.(type) {
	case *big.Int:
		return new(big.Int).Set(typed), nil
	case uint8:
		return big.NewInt(int64(typed)), nil
	case uint16:
		return big.NewInt(int64(typed)), nil
	case uint32:
		return big.NewInt(int64(typed)), nil
	case uint64:
		return new(big.Int).SetUint64(typed), nil
	case int8:
		return big.NewInt(int64(typed)), nil
	case int16:
		return big.NewInt(int64(typed)), nil
	case int32:
		return big.NewInt(int64(typed)), nil
	case int64:
		return big.NewInt(typed), nil
	default:
		return nil, fmt.Errorf("value is %T, want integer", value)
	}
}

func abiUint8(value any) (uint8, error) {
	out, err := abiUint32(value)
	if err != nil {
		return 0, err
	}
	if out > 255 {
		return 0, fmt.Errorf("%d overflows uint8", out)
	}
	return uint8(out), nil
}

func abiUint32(value any) (uint32, error) {
	out, err := abiBigInt(value)
	if err != nil {
		return 0, err
	}
	if out.Sign() < 0 || !out.IsUint64() || out.Uint64() > uint64(^uint32(0)) {
		return 0, fmt.Errorf("%s overflows uint32", out.String())
	}
	return uint32(out.Uint64()), nil
}

func abiInt32(value any) (int32, error) {
	out, err := abiBigInt(value)
	if err != nil {
		return 0, err
	}
	if !out.IsInt64() || out.Int64() < -1<<31 || out.Int64() > 1<<31-1 {
		return 0, fmt.Errorf("%s overflows int32", out.String())
	}
	return int32(out.Int64()), nil
}

func topicUint32(topic common.Hash) (uint32, error) {
	value := new(big.Int).SetBytes(topic.Bytes())
	if !value.IsUint64() || value.Uint64() > uint64(^uint32(0)) {
		return 0, fmt.Errorf("%s overflows uint32", value.String())
	}
	return uint32(value.Uint64()), nil
}
