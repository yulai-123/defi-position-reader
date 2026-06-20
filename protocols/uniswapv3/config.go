package uniswapv3

import (
	"fmt"

	"github.com/yulai-123/defi-position-reader/pkg/chain"
)

type MarketConfig struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	ChainID          int64  `json:"chainId"`
	Factory          string `json:"factory"`
	PositionManager  string `json:"positionManager"`
	DebankProtocolID string `json:"debankProtocolId"`
	StartBlock       uint64 `json:"startBlock"`
}

func marketConfigs(chainID int64) []MarketConfig {
	configs := map[int64][]MarketConfig{
		chain.EthereumChainID: {
			{
				ID:               "ethereum-core",
				Name:             "UniswapV3Ethereum",
				DisplayName:      "Uniswap V3 Ethereum",
				ChainID:          chain.EthereumChainID,
				Factory:          "0x1F98431c8aD98523631AE4a59f267346ea31F984",
				PositionManager:  "0xC36442b4a4522E871399CD717aBDD847Ab11FE88",
				DebankProtocolID: "uniswap3",
				StartBlock:       12369621,
			},
		},
		chain.ArbitrumChainID: {
			{
				ID:               "arbitrum-core",
				Name:             "UniswapV3Arbitrum",
				DisplayName:      "Uniswap V3 Arbitrum",
				ChainID:          chain.ArbitrumChainID,
				Factory:          "0x1F98431c8aD98523631AE4a59f267346ea31F984",
				PositionManager:  "0xC36442b4a4522E871399CD717aBDD847Ab11FE88",
				DebankProtocolID: "arb_uniswap3",
				StartBlock:       0,
			},
		},
		chain.BaseChainID: {
			{
				ID:               "base-core",
				Name:             "UniswapV3Base",
				DisplayName:      "Uniswap V3 Base",
				ChainID:          chain.BaseChainID,
				Factory:          "0x33128a8fC17869897dcE68Ed026d694621f6FDfD",
				PositionManager:  "0x03a520b32C04BF3bEEf7BEb72E919cf822Ed34f1",
				DebankProtocolID: "base_uniswap3",
				StartBlock:       0,
			},
		},
	}

	items := configs[chainID]
	out := make([]MarketConfig, len(items))
	copy(out, items)
	return out
}

func requireMarketConfigs(chainID int64) ([]MarketConfig, error) {
	markets := marketConfigs(chainID)
	if len(markets) == 0 {
		return nil, fmt.Errorf("uniswap v3 does not support chain %d", chainID)
	}
	return markets, nil
}
