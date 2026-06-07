package compoundv3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const cometABIJSON = `[
  {"inputs":[],"name":"name","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"baseToken","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"baseTokenPriceFeed","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"baseScale","outputs":[{"internalType":"uint64","name":"","type":"uint64"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"numAssets","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},
  {
    "inputs":[{"internalType":"uint8","name":"i","type":"uint8"}],
    "name":"getAssetInfo",
    "outputs":[{
      "components":[
        {"internalType":"uint8","name":"offset","type":"uint8"},
        {"internalType":"address","name":"asset","type":"address"},
        {"internalType":"address","name":"priceFeed","type":"address"},
        {"internalType":"uint64","name":"scale","type":"uint64"},
        {"internalType":"uint64","name":"borrowCollateralFactor","type":"uint64"},
        {"internalType":"uint64","name":"liquidateCollateralFactor","type":"uint64"},
        {"internalType":"uint64","name":"liquidationFactor","type":"uint64"},
        {"internalType":"uint128","name":"supplyCap","type":"uint128"}
      ],
      "internalType":"struct CometCore.AssetInfo",
      "name":"",
      "type":"tuple"
    }],
    "stateMutability":"view",
    "type":"function"
  },
  {"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"borrowBalanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"},{"internalType":"address","name":"asset","type":"address"}],"name":"collateralBalanceOf","outputs":[{"internalType":"uint128","name":"","type":"uint128"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"isBorrowCollateralized","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"isLiquidatable","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"priceFeed","type":"address"}],"name":"getPrice","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

const erc20ABIJSON = `[
  {"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"}
]`

const rewardsABIJSON = `[
  {
    "inputs":[{"internalType":"address","name":"","type":"address"}],
    "name":"rewardConfig",
    "outputs":[
      {"internalType":"address","name":"token","type":"address"},
      {"internalType":"uint64","name":"rescaleFactor","type":"uint64"},
      {"internalType":"bool","name":"shouldUpscale","type":"bool"},
      {"internalType":"uint256","name":"multiplier","type":"uint256"}
    ],
    "stateMutability":"view",
    "type":"function"
  },
  {
    "inputs":[
      {"internalType":"address","name":"comet","type":"address"},
      {"internalType":"address","name":"account","type":"address"}
    ],
    "name":"getRewardOwed",
    "outputs":[{
      "components":[
        {"internalType":"address","name":"token","type":"address"},
        {"internalType":"uint256","name":"owed","type":"uint256"}
      ],
      "internalType":"struct CometRewards.RewardOwed",
      "name":"",
      "type":"tuple"
    }],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

const legacyRewardsABIJSON = `[
  {
    "inputs":[{"internalType":"address","name":"","type":"address"}],
    "name":"rewardConfig",
    "outputs":[
      {"internalType":"address","name":"token","type":"address"},
      {"internalType":"uint64","name":"rescaleFactor","type":"uint64"},
      {"internalType":"bool","name":"shouldUpscale","type":"bool"}
    ],
    "stateMutability":"view",
    "type":"function"
  }
]`

var (
	cometABI         = mustABI(cometABIJSON)
	erc20ABI         = mustABI(erc20ABIJSON)
	rewardsABI       = mustABI(rewardsABIJSON)
	legacyRewardsABI = mustABI(legacyRewardsABIJSON)
)

type assetInfoOutput struct {
	Offset                    uint8          `abi:"offset"`
	Asset                     common.Address `abi:"asset"`
	PriceFeed                 common.Address `abi:"priceFeed"`
	Scale                     uint64         `abi:"scale"`
	BorrowCollateralFactor    uint64         `abi:"borrowCollateralFactor"`
	LiquidateCollateralFactor uint64         `abi:"liquidateCollateralFactor"`
	LiquidationFactor         uint64         `abi:"liquidationFactor"`
	SupplyCap                 *big.Int       `abi:"supplyCap"`
}

type rewardConfigOutput struct {
	Token         common.Address `abi:"token"`
	RescaleFactor uint64         `abi:"rescaleFactor"`
	ShouldUpscale bool           `abi:"shouldUpscale"`
	Multiplier    *big.Int       `abi:"multiplier"`
}

type legacyRewardConfigOutput struct {
	Token         common.Address `abi:"token"`
	RescaleFactor uint64         `abi:"rescaleFactor"`
	ShouldUpscale bool           `abi:"shouldUpscale"`
}

type rewardOwedOutput struct {
	Token common.Address `abi:"token"`
	Owed  *big.Int       `abi:"owed"`
}

func mustABI(value string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(value))
	if err != nil {
		panic(err)
	}
	return parsed
}

func packComet(method string, args ...any) ([]byte, error) {
	data, err := cometABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack comet %s: %w", method, err)
	}
	return data, nil
}

func packERC20(method string, args ...any) ([]byte, error) {
	data, err := erc20ABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack erc20 %s: %w", method, err)
	}
	return data, nil
}

func packRewards(method string, args ...any) ([]byte, error) {
	data, err := rewardsABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack rewards %s: %w", method, err)
	}
	return data, nil
}

func unpackCometString(method string, data []byte) (string, error) {
	return unpackString(cometABI, method, data)
}

func unpackERC20Symbol(data []byte) (string, error) {
	return unpackString(erc20ABI, "symbol", data)
}

func unpackERC20Decimals(data []byte) (uint8, error) {
	return unpackUint8(erc20ABI, "decimals", data)
}

func unpackCometDecimals(data []byte) (uint8, error) {
	return unpackUint8(cometABI, "decimals", data)
}

func unpackBaseToken(data []byte) (common.Address, error) {
	return unpackAddress(cometABI, "baseToken", data)
}

func unpackBaseTokenPriceFeed(data []byte) (common.Address, error) {
	return unpackAddress(cometABI, "baseTokenPriceFeed", data)
}

func unpackBaseScale(data []byte) (uint64, error) {
	return unpackUint64(cometABI, "baseScale", data)
}

func unpackNumAssets(data []byte) (uint8, error) {
	return unpackUint8(cometABI, "numAssets", data)
}

func unpackAssetInfo(data []byte) (assetInfoOutput, error) {
	values, err := cometABI.Unpack("getAssetInfo", data)
	if err != nil {
		return assetInfoOutput{}, fmt.Errorf("unpack getAssetInfo: %w", err)
	}
	if len(values) != 1 {
		return assetInfoOutput{}, fmt.Errorf("unpack getAssetInfo: got %d values, want 1", len(values))
	}
	out := abi.ConvertType(values[0], new(assetInfoOutput)).(*assetInfoOutput)
	if out.SupplyCap == nil {
		out.SupplyCap = big.NewInt(0)
	}
	return *out, nil
}

func unpackBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(cometABI, "balanceOf", data)
}

func unpackBorrowBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(cometABI, "borrowBalanceOf", data)
}

func unpackCollateralBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(cometABI, "collateralBalanceOf", data)
}

func unpackIsBorrowCollateralized(data []byte) (bool, error) {
	return unpackBool(cometABI, "isBorrowCollateralized", data)
}

func unpackIsLiquidatable(data []byte) (bool, error) {
	return unpackBool(cometABI, "isLiquidatable", data)
}

func unpackPrice(data []byte) (*big.Int, error) {
	return unpackBigInt(cometABI, "getPrice", data)
}

func unpackRewardConfig(data []byte) (rewardConfigOutput, error) {
	var out rewardConfigOutput
	if err := rewardsABI.UnpackIntoInterface(&out, "rewardConfig", data); err != nil {
		var legacy legacyRewardConfigOutput
		legacyErr := legacyRewardsABI.UnpackIntoInterface(&legacy, "rewardConfig", data)
		if legacyErr != nil {
			return rewardConfigOutput{}, fmt.Errorf("unpack rewardConfig: %w", err)
		}
		out = rewardConfigOutput{
			Token:         legacy.Token,
			RescaleFactor: legacy.RescaleFactor,
			ShouldUpscale: legacy.ShouldUpscale,
			Multiplier:    big.NewInt(0),
		}
	}
	if out.Multiplier == nil {
		out.Multiplier = big.NewInt(0)
	}
	return out, nil
}

func unpackRewardOwed(data []byte) (rewardOwedOutput, error) {
	values, err := rewardsABI.Unpack("getRewardOwed", data)
	if err != nil {
		return rewardOwedOutput{}, fmt.Errorf("unpack getRewardOwed: %w", err)
	}
	if len(values) != 1 {
		return rewardOwedOutput{}, fmt.Errorf("unpack getRewardOwed: got %d values, want 1", len(values))
	}
	out := abi.ConvertType(values[0], new(rewardOwedOutput)).(*rewardOwedOutput)
	if out.Owed == nil {
		out.Owed = big.NewInt(0)
	}
	return *out, nil
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
		return common.Address{}, fmt.Errorf("unpack %s: value is %T, want common.Address", method, values[0])
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

func unpackUint64(parsed abi.ABI, method string, data []byte) (uint64, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return 0, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(uint64)
	if !ok {
		return 0, fmt.Errorf("unpack %s: value is %T, want uint64", method, values[0])
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

func unpackBool(parsed abi.ABI, method string, data []byte) (bool, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return false, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return false, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	value, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("unpack %s: value is %T, want bool", method, values[0])
	}
	return value, nil
}
