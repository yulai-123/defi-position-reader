package uniswapv2

import "github.com/yulai-123/defi-position-reader/pkg/core"

const (
	ProtocolID              = "uniswap-v2"
	marketsNamespace        = "markets"
	pairsNamespace          = "pairs"
	farmingPoolsNamespace   = "farming-pools"
	metadataVersion         = "uniswap-v2-v1"
	defaultMulticallBatchSz = 500
)

type Market struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	ChainID          int64  `json:"chainId"`
	Factory          string `json:"factory"`
	Router           string `json:"router"`
	DebankProtocolID string `json:"debankProtocolId"`
	StartBlock       uint64 `json:"startBlock,omitempty"`
}

type Pair struct {
	MarketID     string     `json:"marketId"`
	Address      string     `json:"address"`
	Index        uint64     `json:"index"`
	PairToken    core.Token `json:"pairToken"`
	Token0       core.Token `json:"token0"`
	Token1       core.Token `json:"token1"`
	DebankPoolID string     `json:"debankPoolId"`
}

type FarmingPool struct {
	ID              string     `json:"id"`
	MarketID        string     `json:"marketId"`
	Name            string     `json:"name"`
	DisplayName     string     `json:"displayName"`
	StakingContract string     `json:"stakingContract"`
	Pair            string     `json:"pair"`
	PairToken       core.Token `json:"pairToken"`
	Token0          core.Token `json:"token0"`
	Token1          core.Token `json:"token1"`
	RewardToken     core.Token `json:"rewardToken"`
	PeriodFinish    string     `json:"periodFinish,omitempty"`
	DebankPoolID    string     `json:"debankPoolId"`
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
