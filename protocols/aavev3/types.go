package aavev3

import "github.com/yulai-123/defi-position-reader/pkg/core"

const (
	marketsNamespace         = "markets"
	lendingReservesNamespace = "lending-reserves"
	yieldVaultsNamespace     = "yield-vaults"
	lendingMetadataVersion   = "aave-v3-lending-v1"
	yieldMetadataVersion     = "aave-v3-yield-v1"
)

type LendingReserve struct {
	MarketID                 string     `json:"marketId"`
	Asset                    core.Token `json:"asset"`
	AToken                   core.Token `json:"aToken"`
	StableDebtToken          core.Token `json:"stableDebtToken"`
	VariableDebtToken        core.Token `json:"variableDebtToken"`
	LTV                      string     `json:"ltv"`
	LiquidationThreshold     string     `json:"liquidationThreshold"`
	LiquidationBonus         string     `json:"liquidationBonus"`
	ReserveFactor            string     `json:"reserveFactor"`
	UsageAsCollateralEnabled bool       `json:"usageAsCollateralEnabled"`
	BorrowingEnabled         bool       `json:"borrowingEnabled"`
	StableBorrowRateEnabled  bool       `json:"stableBorrowRateEnabled"`
	IsActive                 bool       `json:"isActive"`
	IsFrozen                 bool       `json:"isFrozen"`
}

type LendingMetadata struct {
	Markets  []MarketConfig   `json:"markets"`
	Reserves []LendingReserve `json:"reserves"`
}

type YieldVault struct {
	MarketID     string       `json:"marketId"`
	Kind         string       `json:"kind"`
	Factory      string       `json:"factory"`
	VaultToken   core.Token   `json:"vaultToken"`
	Asset        core.Token   `json:"asset"`
	AToken       core.Token   `json:"aToken"`
	RewardTokens []core.Token `json:"rewardTokens,omitempty"`
}
