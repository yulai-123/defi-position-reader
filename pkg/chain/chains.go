package chain

import (
	"strings"

	"github.com/yulai-123/defi-position-reader/pkg/core"
)

const (
	EthereumChainID = int64(1)
	ArbitrumChainID = int64(42161)
	BaseChainID     = int64(8453)
)

var defaultChains = []core.Chain{
	{
		ID:             EthereumChainID,
		Name:           "ethereum",
		DisplayName:    "Ethereum",
		NativeCurrency: "ETH",
		RPCUrlEnv:      "ETHEREUM_RPC_URL",
		PublicRPCURLs: []string{
			"https://ethereum-rpc.publicnode.com",
			"https://1rpc.io/eth",
		},
	},
	{
		ID:             ArbitrumChainID,
		Name:           "arbitrum",
		DisplayName:    "Arbitrum One",
		NativeCurrency: "ETH",
		RPCUrlEnv:      "ARBITRUM_RPC_URL",
		PublicRPCURLs: []string{
			"https://arbitrum-one-rpc.publicnode.com",
			"https://arb1.arbitrum.io/rpc",
			"https://1rpc.io/arb",
		},
	},
	{
		ID:             BaseChainID,
		Name:           "base",
		DisplayName:    "Base",
		NativeCurrency: "ETH",
		RPCUrlEnv:      "BASE_RPC_URL",
		PublicRPCURLs: []string{
			"https://base-rpc.publicnode.com",
			"https://mainnet.base.org",
			"https://1rpc.io/base",
		},
	},
}

func DefaultChains() []core.Chain {
	chains := make([]core.Chain, len(defaultChains))
	copy(chains, defaultChains)
	for i := range chains {
		chains[i].PublicRPCURLs = append([]string(nil), chains[i].PublicRPCURLs...)
	}
	return chains
}

func ByName(name string) (core.Chain, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, item := range defaultChains {
		if item.Name == name {
			return item, true
		}
	}
	return core.Chain{}, false
}

func ByID(chainID int64) (core.Chain, bool) {
	for _, item := range defaultChains {
		if item.ID == chainID {
			return item, true
		}
	}
	return core.Chain{}, false
}
