package uniswapv2

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
	Router           string `json:"router"`
	DebankProtocolID string `json:"debankProtocolId"`
	StartBlock       uint64 `json:"startBlock"`
}

type FarmingPoolConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DisplayName     string `json:"displayName"`
	MarketID        string `json:"marketId"`
	StakingContract string `json:"stakingContract"`
	Pair            string `json:"pair"`
}

func marketConfigs(chainID int64) []MarketConfig {
	configs := map[int64][]MarketConfig{
		chain.EthereumChainID: {
			{
				ID:               "ethereum-core",
				Name:             "UniswapV2Ethereum",
				DisplayName:      "Uniswap V2 Ethereum",
				ChainID:          chain.EthereumChainID,
				Factory:          "0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f",
				Router:           "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
				DebankProtocolID: "uniswap2",
				StartBlock:       10000835,
			},
		},
		chain.ArbitrumChainID: {
			{
				ID:               "arbitrum-core",
				Name:             "UniswapV2Arbitrum",
				DisplayName:      "Uniswap V2 Arbitrum",
				ChainID:          chain.ArbitrumChainID,
				Factory:          "0xf1D7CC64Fb4452F05c498126312eBE29f30Fbcf9",
				Router:           "0x4752ba5dbc23f44d87826276bf6fd6b1c372ad24",
				DebankProtocolID: "arb_uniswap2",
				StartBlock:       0,
			},
		},
		chain.BaseChainID: {
			{
				ID:               "base-core",
				Name:             "UniswapV2Base",
				DisplayName:      "Uniswap V2 Base",
				ChainID:          chain.BaseChainID,
				Factory:          "0x8909Dc15e40173Ff4699343b6eB8132c65e18eC6",
				Router:           "0x4752ba5dbc23f44d87826276bf6fd6b1c372ad24",
				DebankProtocolID: "base_uniswap2",
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
		return nil, fmt.Errorf("uniswap v2 does not support chain %d", chainID)
	}
	return markets, nil
}

func farmingPoolConfigs(chainID int64) []FarmingPoolConfig {
	if chainID != chain.EthereumChainID {
		return nil
	}
	return []FarmingPoolConfig{
		{
			ID:              "ethereum-eth-usdt-uni",
			Name:            "UniswapV2ETHUSDTUNIRewards",
			DisplayName:     "Uniswap V2 ETH / USDT Farming",
			MarketID:        "ethereum-core",
			StakingContract: "0x6C3e4cb2E96B01F4b866965A91ed4437839A121a",
			Pair:            "0x0d4a11d5EEaaC28EC3F61d100daF4d40471f1852",
		},
		{
			ID:              "ethereum-eth-usdc-uni",
			Name:            "UniswapV2ETHUSDCUNIRewards",
			DisplayName:     "Uniswap V2 ETH / USDC Farming",
			MarketID:        "ethereum-core",
			StakingContract: "0x7FBa4B8Dc5E7616e59622806932DBea72537A56b",
			Pair:            "0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc",
		},
		{
			ID:              "ethereum-eth-dai-uni",
			Name:            "UniswapV2ETHDAIUNIRewards",
			DisplayName:     "Uniswap V2 ETH / DAI Farming",
			MarketID:        "ethereum-core",
			StakingContract: "0xa1484C3aa22a66C62b77E0AE78E15258bd0cB711",
			Pair:            "0xA478c2975Ab1Ea89e8196811F51A7B7Ade33eB11",
		},
		{
			ID:              "ethereum-eth-wbtc-uni",
			Name:            "UniswapV2ETHWBTCUNIRewards",
			DisplayName:     "Uniswap V2 ETH / WBTC Farming",
			MarketID:        "ethereum-core",
			StakingContract: "0xCA35e32e7926b96A9988f61d510E038108d8068e",
			Pair:            "0xBb2b8038a1640196FbE3e38816F3e67Cba72D940",
		},
	}
}
