package aavev3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const dataProviderABIJSON = `[
  {
    "inputs": [],
    "name": "getAllReservesTokens",
    "outputs": [
      {
        "components": [
          {"internalType": "string", "name": "symbol", "type": "string"},
          {"internalType": "address", "name": "tokenAddress", "type": "address"}
        ],
        "internalType": "struct IPoolDataProvider.TokenData[]",
        "name": "",
        "type": "tuple[]"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "asset", "type": "address"}],
    "name": "getReserveTokensAddresses",
    "outputs": [
      {"internalType": "address", "name": "aTokenAddress", "type": "address"},
      {"internalType": "address", "name": "stableDebtTokenAddress", "type": "address"},
      {"internalType": "address", "name": "variableDebtTokenAddress", "type": "address"}
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "asset", "type": "address"}],
    "name": "getReserveConfigurationData",
    "outputs": [
      {"internalType": "uint256", "name": "decimals", "type": "uint256"},
      {"internalType": "uint256", "name": "ltv", "type": "uint256"},
      {"internalType": "uint256", "name": "liquidationThreshold", "type": "uint256"},
      {"internalType": "uint256", "name": "liquidationBonus", "type": "uint256"},
      {"internalType": "uint256", "name": "reserveFactor", "type": "uint256"},
      {"internalType": "bool", "name": "usageAsCollateralEnabled", "type": "bool"},
      {"internalType": "bool", "name": "borrowingEnabled", "type": "bool"},
      {"internalType": "bool", "name": "stableBorrowRateEnabled", "type": "bool"},
      {"internalType": "bool", "name": "isActive", "type": "bool"},
      {"internalType": "bool", "name": "isFrozen", "type": "bool"}
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "address", "name": "asset", "type": "address"},
      {"internalType": "address", "name": "user", "type": "address"}
    ],
    "name": "getUserReserveData",
    "outputs": [
      {"internalType": "uint256", "name": "currentATokenBalance", "type": "uint256"},
      {"internalType": "uint256", "name": "currentStableDebt", "type": "uint256"},
      {"internalType": "uint256", "name": "currentVariableDebt", "type": "uint256"},
      {"internalType": "uint256", "name": "principalStableDebt", "type": "uint256"},
      {"internalType": "uint256", "name": "scaledVariableDebt", "type": "uint256"},
      {"internalType": "uint256", "name": "stableBorrowRate", "type": "uint256"},
      {"internalType": "uint256", "name": "liquidityRate", "type": "uint256"},
      {"internalType": "uint40", "name": "stableRateLastUpdated", "type": "uint40"},
      {"internalType": "bool", "name": "usageAsCollateralEnabled", "type": "bool"}
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const poolABIJSON = `[
  {
    "inputs": [{"internalType": "address", "name": "user", "type": "address"}],
    "name": "getUserAccountData",
    "outputs": [
      {"internalType": "uint256", "name": "totalCollateralBase", "type": "uint256"},
      {"internalType": "uint256", "name": "totalDebtBase", "type": "uint256"},
      {"internalType": "uint256", "name": "availableBorrowsBase", "type": "uint256"},
      {"internalType": "uint256", "name": "currentLiquidationThreshold", "type": "uint256"},
      {"internalType": "uint256", "name": "ltv", "type": "uint256"},
      {"internalType": "uint256", "name": "healthFactor", "type": "uint256"}
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const erc20ABIJSON = `[
  {
    "inputs": [],
    "name": "symbol",
    "outputs": [{"internalType": "string", "name": "", "type": "string"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "decimals",
    "outputs": [{"internalType": "uint8", "name": "", "type": "uint8"}],
    "stateMutability": "view",
    "type": "function"
  }
]`

const stataFactoryABIJSON = `[
  {
    "inputs": [],
    "name": "getStataTokens",
    "outputs": [{"internalType": "address[]", "name": "", "type": "address[]"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "underlying", "type": "address"}],
    "name": "getStataToken",
    "outputs": [{"internalType": "address", "name": "", "type": "address"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "getStaticATokens",
    "outputs": [{"internalType": "address[]", "name": "", "type": "address[]"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "underlying", "type": "address"}],
    "name": "getStaticAToken",
    "outputs": [{"internalType": "address", "name": "", "type": "address"}],
    "stateMutability": "view",
    "type": "function"
  }
]`

const yieldVaultABIJSON = `[
  {
    "inputs": [],
    "name": "asset",
    "outputs": [{"internalType": "address", "name": "", "type": "address"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "aToken",
    "outputs": [{"internalType": "address", "name": "", "type": "address"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "rewardTokens",
    "outputs": [{"internalType": "address[]", "name": "", "type": "address[]"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "owner", "type": "address"}],
    "name": "balanceOf",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "uint256", "name": "shares", "type": "uint256"}],
    "name": "previewRedeem",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "uint256", "name": "shares", "type": "uint256"}],
    "name": "convertToAssets",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "owner", "type": "address"}],
    "name": "maxWithdraw",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "address", "name": "owner", "type": "address"}],
    "name": "maxRedeem",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "address", "name": "user", "type": "address"},
      {"internalType": "address", "name": "reward", "type": "address"}
    ],
    "name": "getClaimableRewards",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  }
]`

var (
	dataProviderABI = mustABI(dataProviderABIJSON)
	poolABI         = mustABI(poolABIJSON)
	erc20ABI        = mustABI(erc20ABIJSON)
	stataFactoryABI = mustABI(stataFactoryABIJSON)
	yieldVaultABI   = mustABI(yieldVaultABIJSON)
)

type tokenDataOutput struct {
	Symbol       string         `abi:"symbol"`
	TokenAddress common.Address `abi:"tokenAddress"`
}

type reserveTokenAddressesOutput struct {
	ATokenAddress            common.Address `abi:"aTokenAddress"`
	StableDebtTokenAddress   common.Address `abi:"stableDebtTokenAddress"`
	VariableDebtTokenAddress common.Address `abi:"variableDebtTokenAddress"`
}

type reserveConfigurationOutput struct {
	Decimals                 *big.Int `abi:"decimals"`
	LTV                      *big.Int `abi:"ltv"`
	LiquidationThreshold     *big.Int `abi:"liquidationThreshold"`
	LiquidationBonus         *big.Int `abi:"liquidationBonus"`
	ReserveFactor            *big.Int `abi:"reserveFactor"`
	UsageAsCollateralEnabled bool     `abi:"usageAsCollateralEnabled"`
	BorrowingEnabled         bool     `abi:"borrowingEnabled"`
	StableBorrowRateEnabled  bool     `abi:"stableBorrowRateEnabled"`
	IsActive                 bool     `abi:"isActive"`
	IsFrozen                 bool     `abi:"isFrozen"`
}

type userReserveDataOutput struct {
	CurrentATokenBalance     *big.Int `abi:"currentATokenBalance"`
	CurrentStableDebt        *big.Int `abi:"currentStableDebt"`
	CurrentVariableDebt      *big.Int `abi:"currentVariableDebt"`
	PrincipalStableDebt      *big.Int `abi:"principalStableDebt"`
	ScaledVariableDebt       *big.Int `abi:"scaledVariableDebt"`
	StableBorrowRate         *big.Int `abi:"stableBorrowRate"`
	LiquidityRate            *big.Int `abi:"liquidityRate"`
	StableRateLastUpdated    *big.Int `abi:"stableRateLastUpdated"`
	UsageAsCollateralEnabled bool     `abi:"usageAsCollateralEnabled"`
}

type userAccountDataOutput struct {
	TotalCollateralBase         *big.Int `abi:"totalCollateralBase"`
	TotalDebtBase               *big.Int `abi:"totalDebtBase"`
	AvailableBorrowsBase        *big.Int `abi:"availableBorrowsBase"`
	CurrentLiquidationThreshold *big.Int `abi:"currentLiquidationThreshold"`
	LTV                         *big.Int `abi:"ltv"`
	HealthFactor                *big.Int `abi:"healthFactor"`
}

func mustABI(value string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(value))
	if err != nil {
		panic(err)
	}
	return parsed
}

func packDataProvider(method string, args ...any) ([]byte, error) {
	data, err := dataProviderABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack data provider %s: %w", method, err)
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
	data, err := erc20ABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack erc20 %s: %w", method, err)
	}
	return data, nil
}

func packStataFactory(method string, args ...any) ([]byte, error) {
	data, err := stataFactoryABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack stata factory %s: %w", method, err)
	}
	return data, nil
}

func packYieldVault(method string, args ...any) ([]byte, error) {
	data, err := yieldVaultABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack yield vault %s: %w", method, err)
	}
	return data, nil
}

func unpackAllReservesTokens(data []byte) ([]tokenDataOutput, error) {
	var out []tokenDataOutput
	if err := dataProviderABI.UnpackIntoInterface(&out, "getAllReservesTokens", data); err != nil {
		return nil, fmt.Errorf("unpack getAllReservesTokens: %w", err)
	}
	return out, nil
}

func unpackReserveTokenAddresses(data []byte) (reserveTokenAddressesOutput, error) {
	var out reserveTokenAddressesOutput
	if err := dataProviderABI.UnpackIntoInterface(&out, "getReserveTokensAddresses", data); err != nil {
		return reserveTokenAddressesOutput{}, fmt.Errorf("unpack getReserveTokensAddresses: %w", err)
	}
	return out, nil
}

func unpackReserveConfiguration(data []byte) (reserveConfigurationOutput, error) {
	var out reserveConfigurationOutput
	if err := dataProviderABI.UnpackIntoInterface(&out, "getReserveConfigurationData", data); err != nil {
		return reserveConfigurationOutput{}, fmt.Errorf("unpack getReserveConfigurationData: %w", err)
	}
	return out, nil
}

func unpackUserReserveData(data []byte) (userReserveDataOutput, error) {
	var out userReserveDataOutput
	if err := dataProviderABI.UnpackIntoInterface(&out, "getUserReserveData", data); err != nil {
		return userReserveDataOutput{}, fmt.Errorf("unpack getUserReserveData: %w", err)
	}
	return out, nil
}

func unpackUserAccountData(data []byte) (userAccountDataOutput, error) {
	var out userAccountDataOutput
	if err := poolABI.UnpackIntoInterface(&out, "getUserAccountData", data); err != nil {
		return userAccountDataOutput{}, fmt.Errorf("unpack getUserAccountData: %w", err)
	}
	return out, nil
}

func unpackSymbol(data []byte) (string, error) {
	values, err := erc20ABI.Unpack("symbol", data)
	if err != nil {
		return "", fmt.Errorf("unpack symbol: %w", err)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("unpack symbol: got %d values, want 1", len(values))
	}
	symbol, ok := values[0].(string)
	if !ok {
		return "", fmt.Errorf("unpack symbol: value is %T, want string", values[0])
	}
	return symbol, nil
}

func unpackTokenDecimals(data []byte) (uint8, error) {
	values, err := erc20ABI.Unpack("decimals", data)
	if err != nil {
		return 0, fmt.Errorf("unpack decimals: %w", err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("unpack decimals: got %d values, want 1", len(values))
	}
	decimals, ok := values[0].(uint8)
	if !ok {
		return 0, fmt.Errorf("unpack decimals: value is %T, want uint8", values[0])
	}
	return decimals, nil
}

func unpackStataTokens(data []byte) ([]common.Address, error) {
	return unpackAddressSlice(stataFactoryABI, "getStataTokens", data)
}

func unpackStaticATokens(data []byte) ([]common.Address, error) {
	return unpackAddressSlice(stataFactoryABI, "getStaticATokens", data)
}

func unpackVaultAsset(data []byte) (common.Address, error) {
	return unpackAddress(yieldVaultABI, "asset", data)
}

func unpackVaultAToken(data []byte) (common.Address, error) {
	return unpackAddress(yieldVaultABI, "aToken", data)
}

func unpackVaultRewardTokens(data []byte) ([]common.Address, error) {
	return unpackAddressSlice(yieldVaultABI, "rewardTokens", data)
}

func unpackVaultBalance(data []byte) (*big.Int, error) {
	return unpackBigInt(yieldVaultABI, "balanceOf", data)
}

func unpackPreviewRedeem(data []byte) (*big.Int, error) {
	return unpackBigInt(yieldVaultABI, "previewRedeem", data)
}

func unpackMaxWithdraw(data []byte) (*big.Int, error) {
	return unpackBigInt(yieldVaultABI, "maxWithdraw", data)
}

func unpackMaxRedeem(data []byte) (*big.Int, error) {
	return unpackBigInt(yieldVaultABI, "maxRedeem", data)
}

func unpackClaimableRewards(data []byte) (*big.Int, error) {
	return unpackBigInt(yieldVaultABI, "getClaimableRewards", data)
}

func unpackAddress(parsed abi.ABI, method string, data []byte) (common.Address, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return common.Address{}, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	address, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("unpack %s: value is %T, want common.Address", method, values[0])
	}
	return address, nil
}

func unpackAddressSlice(parsed abi.ABI, method string, data []byte) ([]common.Address, error) {
	values, err := parsed.Unpack(method, data)
	if err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("unpack %s: got %d values, want 1", method, len(values))
	}
	addresses, ok := values[0].([]common.Address)
	if !ok {
		return nil, fmt.Errorf("unpack %s: value is %T, want []common.Address", method, values[0])
	}
	return addresses, nil
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
