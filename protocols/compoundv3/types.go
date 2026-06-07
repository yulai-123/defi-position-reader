package compoundv3

import "github.com/yulai-123/defi-position-reader/pkg/core"

const (
	ProtocolID                = "compound-v3"
	marketsNamespace          = "markets"
	collateralAssetsNamespace = "collateral-assets"
	rewardConfigsNamespace    = "reward-configs"
	metadataVersion           = "compound-v3-v1"
)

type Market struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	DisplayName      string     `json:"displayName"`
	ChainID          int64      `json:"chainId"`
	Comet            string     `json:"comet"`
	Rewards          string     `json:"rewards,omitempty"`
	DebankProtocolID string     `json:"debankProtocolId"`
	YieldPoolID      string     `json:"yieldPoolId"`
	LendingPoolID    string     `json:"lendingPoolId"`
	CometToken       core.Token `json:"cometToken"`
	BaseToken        core.Token `json:"baseToken"`
	BasePriceFeed    string     `json:"basePriceFeed"`
	BaseScale        string     `json:"baseScale"`
	NumAssets        uint8      `json:"numAssets"`
}

type CollateralAsset struct {
	MarketID                  string     `json:"marketId"`
	Asset                     core.Token `json:"asset"`
	PriceFeed                 string     `json:"priceFeed"`
	Scale                     string     `json:"scale"`
	BorrowCollateralFactor    string     `json:"borrowCollateralFactor"`
	LiquidateCollateralFactor string     `json:"liquidateCollateralFactor"`
	LiquidationFactor         string     `json:"liquidationFactor"`
	SupplyCap                 string     `json:"supplyCap"`
}

type RewardConfig struct {
	MarketID      string     `json:"marketId"`
	Rewards       string     `json:"rewards"`
	Supported     bool       `json:"supported"`
	RewardToken   core.Token `json:"rewardToken,omitempty"`
	RescaleFactor string     `json:"rescaleFactor,omitempty"`
	ShouldUpscale bool       `json:"shouldUpscale,omitempty"`
	Multiplier    string     `json:"multiplier,omitempty"`
}

type discoveryStep struct {
	Step         string `json:"step"`
	Operation    string `json:"operation"`
	Markets      int    `json:"markets,omitempty"`
	Calls        int    `json:"calls,omitempty"`
	Items        int    `json:"items,omitempty"`
	Status       string `json:"status"`
	Notes        string `json:"notes,omitempty"`
	AllowFailure bool   `json:"allowFailure,omitempty"`
}
