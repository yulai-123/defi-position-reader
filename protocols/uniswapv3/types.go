package uniswapv3

import "github.com/yulai-123/defi-position-reader/pkg/core"

const (
	ProtocolID              = "uniswap-v3"
	marketsNamespace        = "markets"
	poolsNamespace          = "pools"
	metadataVersion         = "uniswap-v3-v1"
	defaultMulticallBatchSz = 500
	defaultLogBlockChunk    = 100_000
)

type Market struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	ChainID          int64  `json:"chainId"`
	Factory          string `json:"factory"`
	PositionManager  string `json:"positionManager"`
	DebankProtocolID string `json:"debankProtocolId"`
	StartBlock       uint64 `json:"startBlock,omitempty"`
}

type Pool struct {
	MarketID     string     `json:"marketId"`
	Address      string     `json:"address"`
	Token0       core.Token `json:"token0"`
	Token1       core.Token `json:"token1"`
	Fee          uint32     `json:"fee"`
	FeeFormatted string     `json:"feeFormatted"`
	TickSpacing  int32      `json:"tickSpacing"`
	CreatedBlock uint64     `json:"createdBlock,omitempty"`
	DebankPoolID string     `json:"debankPoolId"`
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
