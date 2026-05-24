package aavev3

import (
	"fmt"

	"github.com/yulai-123/defi-position-reader/pkg/chain"
)

type MarketConfig struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	DisplayName           string `json:"displayName"`
	ChainID               int64  `json:"chainId"`
	PoolAddressesProvider string `json:"poolAddressesProvider"`
	Pool                  string `json:"pool"`
	DataProvider          string `json:"dataProvider"`
	StataFactory          string `json:"stataFactory,omitempty"`
	LegacyStaticFactory   string `json:"legacyStaticFactory,omitempty"`
}

func marketConfigs(chainID int64) []MarketConfig {
	configs := map[int64][]MarketConfig{
		chain.EthereumChainID: {
			{
				ID:                    "ethereum-core",
				Name:                  "AaveV3Ethereum",
				DisplayName:           "Aave V3 Ethereum Core Lending",
				ChainID:               chain.EthereumChainID,
				PoolAddressesProvider: "0x2f39d218133AFaB8F2B819B1066c7E434Ad94E9e",
				Pool:                  "0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2",
				DataProvider:          "0x0a16f2FCC0D44FaE41cc54e079281D84A363bECD",
				StataFactory:          "0xCb0b5cA20b6C5C02A9A3B2cE433650768eD2974F",
				LegacyStaticFactory:   "0x411D79b8cC43384FDE66CaBf9b6a17180c842511",
			},
			{
				ID:                    "ethereum-etherfi",
				Name:                  "AaveV3EthereumEtherFi",
				DisplayName:           "Aave V3 Ethereum EtherFi Lending",
				ChainID:               chain.EthereumChainID,
				PoolAddressesProvider: "0xeBa440B438Ad808101d1c451C1C5322c90BEFCdA",
				Pool:                  "0x0AA97c284e98396202b6A04024F5E2c65026F3c0",
				DataProvider:          "0x7c8509591f9693D21280d96e149a08A3bf69Cd0c",
			},
			{
				ID:                    "ethereum-lido",
				Name:                  "AaveV3EthereumLido",
				DisplayName:           "Aave V3 Ethereum Lido Lending",
				ChainID:               chain.EthereumChainID,
				PoolAddressesProvider: "0xcfBf336fe147D643B9Cb705648500e101504B16d",
				Pool:                  "0x4e033931ad43597d96D6bcc25c280717730B58B1",
				DataProvider:          "0xB85B2bFEbeC4F5f401dbf92ac147A3076391fCD5",
				StataFactory:          "0x347C75d19718a05148687E13dca259aD016aB411",
			},
			{
				ID:                    "ethereum-horizon",
				Name:                  "AaveV3EthereumHorizon",
				DisplayName:           "Aave V3 Ethereum Horizon Lending",
				ChainID:               chain.EthereumChainID,
				PoolAddressesProvider: "0x5D39E06b825C1F2B80bf2756a73e28eFAA128ba0",
				Pool:                  "0xAe05Cd22df81871bc7cC2a04BeCfb516bFe332C8",
				DataProvider:          "0x53519c32f73fE1797d10210c4950fFeBa3b21504",
			},
		},
		chain.ArbitrumChainID: {
			{
				ID:                    "arbitrum-core",
				Name:                  "AaveV3Arbitrum",
				DisplayName:           "Aave V3 Arbitrum Core Lending",
				ChainID:               chain.ArbitrumChainID,
				PoolAddressesProvider: "0xa97684ead0e402dC232d5A977953DF7ECBaB3CDb",
				Pool:                  "0x794a61358D6845594F94dc1DB02A252b5b4814aD",
				DataProvider:          "0x243Aa95cAC2a25651eda86e80bEe66114413c43b",
				StataFactory:          "0xd85922fFF51ba4130cEC7c499db4Ac3Eb9981EaD",
				LegacyStaticFactory:   "0x411D79b8cC43384FDE66CaBf9b6a17180c842511",
			},
		},
		chain.BaseChainID: {
			{
				ID:                    "base-core",
				Name:                  "AaveV3Base",
				DisplayName:           "Aave V3 Base Core Lending",
				ChainID:               chain.BaseChainID,
				PoolAddressesProvider: "0xe20fCBdBfFC4Dd138cE8b2E6FBb6CB49777ad64D",
				Pool:                  "0xA238Dd80C259a72e81d7e4664a9801593F98d1c5",
				DataProvider:          "0x0F43731EB8d45A581f4a36DD74F5f358bc90C73A",
				StataFactory:          "0x78d33BF0014ab169725F2Ea5a62b200F2977faeE",
				LegacyStaticFactory:   "0x940F9a5d5F9ED264990D0eaee1F3DD60B4Cb9A22",
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
		return nil, fmt.Errorf("aave v3 does not support chain %d", chainID)
	}
	return markets, nil
}
