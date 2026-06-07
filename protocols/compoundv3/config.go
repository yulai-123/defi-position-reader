package compoundv3

import (
	"fmt"

	"github.com/yulai-123/defi-position-reader/pkg/chain"
)

type MarketConfig struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	ChainID          int64  `json:"chainId"`
	Comet            string `json:"comet"`
	Rewards          string `json:"rewards,omitempty"`
	DebankProtocolID string `json:"debankProtocolId"`
}

func marketConfigs(chainID int64) []MarketConfig {
	configs := map[int64][]MarketConfig{
		chain.EthereumChainID: {
			{
				ID:               "ethereum-usdc",
				Name:             "CompoundV3EthereumUSDC",
				DisplayName:      "Compound V3 Ethereum USDC",
				ChainID:          chain.EthereumChainID,
				Comet:            "0xc3d688B66703497DAA19211EEdff47f25384cdc3",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
			{
				ID:               "ethereum-usds",
				Name:             "CompoundV3EthereumUSDS",
				DisplayName:      "Compound V3 Ethereum USDS",
				ChainID:          chain.EthereumChainID,
				Comet:            "0x5D409e56D886231aDAf00c8775665AD0f9897b56",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
			{
				ID:               "ethereum-usdt",
				Name:             "CompoundV3EthereumUSDT",
				DisplayName:      "Compound V3 Ethereum USDT",
				ChainID:          chain.EthereumChainID,
				Comet:            "0x3Afdc9BCA9213A35503b077a6072F3D0d5AB0840",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
			{
				ID:               "ethereum-wbtc",
				Name:             "CompoundV3EthereumWBTC",
				DisplayName:      "Compound V3 Ethereum WBTC",
				ChainID:          chain.EthereumChainID,
				Comet:            "0xe85Dc543813B8c2CFEaAc371517b925a166a9293",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
			{
				ID:               "ethereum-weth",
				Name:             "CompoundV3EthereumWETH",
				DisplayName:      "Compound V3 Ethereum WETH",
				ChainID:          chain.EthereumChainID,
				Comet:            "0xA17581A9E3356d9A858b789D68B4d866e593aE94",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
			{
				ID:               "ethereum-wsteth",
				Name:             "CompoundV3EthereumWstETH",
				DisplayName:      "Compound V3 Ethereum wstETH",
				ChainID:          chain.EthereumChainID,
				Comet:            "0x3D0bb1ccaB520A66e607822fC55BC921738fAFE3",
				Rewards:          "0x1B0e765F6224C21223AeA2af16c1C46E38885a40",
				DebankProtocolID: "compound3",
			},
		},
		chain.ArbitrumChainID: {
			{
				ID:               "arbitrum-usdce",
				Name:             "CompoundV3ArbitrumUSDCe",
				DisplayName:      "Compound V3 Arbitrum USDC.e",
				ChainID:          chain.ArbitrumChainID,
				Comet:            "0xA5EDBDD9646f8dFF606d7448e414884C7d905dCA",
				Rewards:          "0x88730d254A2f7e6AC8388c3198aFd694bA9f7fae",
				DebankProtocolID: "arb_compound3",
			},
			{
				ID:               "arbitrum-usdc",
				Name:             "CompoundV3ArbitrumUSDC",
				DisplayName:      "Compound V3 Arbitrum USDC",
				ChainID:          chain.ArbitrumChainID,
				Comet:            "0x9c4ec768c28520B50860ea7a15bd7213a9fF58bf",
				Rewards:          "0x88730d254A2f7e6AC8388c3198aFd694bA9f7fae",
				DebankProtocolID: "arb_compound3",
			},
			{
				ID:               "arbitrum-usdt",
				Name:             "CompoundV3ArbitrumUSDT",
				DisplayName:      "Compound V3 Arbitrum USDT",
				ChainID:          chain.ArbitrumChainID,
				Comet:            "0xd98Be00b5D27fc98112BdE293e487f8D4cA57d07",
				Rewards:          "0x88730d254A2f7e6AC8388c3198aFd694bA9f7fae",
				DebankProtocolID: "arb_compound3",
			},
			{
				ID:               "arbitrum-weth",
				Name:             "CompoundV3ArbitrumWETH",
				DisplayName:      "Compound V3 Arbitrum WETH",
				ChainID:          chain.ArbitrumChainID,
				Comet:            "0x6f7D514bbD4aFf3BcD1140B7344b32f063dEe486",
				Rewards:          "0x88730d254A2f7e6AC8388c3198aFd694bA9f7fae",
				DebankProtocolID: "arb_compound3",
			},
		},
		chain.BaseChainID: {
			{
				ID:               "base-aero",
				Name:             "CompoundV3BaseAERO",
				DisplayName:      "Compound V3 Base AERO",
				ChainID:          chain.BaseChainID,
				Comet:            "0x784efeB622244d2348d4F2522f8860B96fbEcE89",
				Rewards:          "0x123964802e6ABabBE1Bc9547D72Ef1B69B00A6b1",
				DebankProtocolID: "base_compound3",
			},
			{
				ID:               "base-usdbc",
				Name:             "CompoundV3BaseUSDbC",
				DisplayName:      "Compound V3 Base USDbC",
				ChainID:          chain.BaseChainID,
				Comet:            "0x9c4ec768c28520B50860ea7a15bd7213a9fF58bf",
				Rewards:          "0x123964802e6ABabBE1Bc9547D72Ef1B69B00A6b1",
				DebankProtocolID: "base_compound3",
			},
			{
				ID:               "base-usdc",
				Name:             "CompoundV3BaseUSDC",
				DisplayName:      "Compound V3 Base USDC",
				ChainID:          chain.BaseChainID,
				Comet:            "0xb125E6687d4313864e53df431d5425969c15Eb2F",
				Rewards:          "0x123964802e6ABabBE1Bc9547D72Ef1B69B00A6b1",
				DebankProtocolID: "base_compound3",
			},
			{
				ID:               "base-usds",
				Name:             "CompoundV3BaseUSDS",
				DisplayName:      "Compound V3 Base USDS",
				ChainID:          chain.BaseChainID,
				Comet:            "0x2c776041CCFe903071AF44aa147368a9c8EEA518",
				Rewards:          "0x123964802e6ABabBE1Bc9547D72Ef1B69B00A6b1",
				DebankProtocolID: "base_compound3",
			},
			{
				ID:               "base-weth",
				Name:             "CompoundV3BaseWETH",
				DisplayName:      "Compound V3 Base WETH",
				ChainID:          chain.BaseChainID,
				Comet:            "0x46e6b214b524310239732D51387075E0e70970bf",
				Rewards:          "0x123964802e6ABabBE1Bc9547D72Ef1B69B00A6b1",
				DebankProtocolID: "base_compound3",
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
		return nil, fmt.Errorf("compound v3 does not support chain %d", chainID)
	}
	return markets, nil
}
