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
	},
	{
		ID:             ArbitrumChainID,
		Name:           "arbitrum",
		DisplayName:    "Arbitrum One",
		NativeCurrency: "ETH",
		RPCUrlEnv:      "ARBITRUM_RPC_URL",
	},
	{
		ID:             BaseChainID,
		Name:           "base",
		DisplayName:    "Base",
		NativeCurrency: "ETH",
		RPCUrlEnv:      "BASE_RPC_URL",
	},
}

func DefaultChains() []core.Chain {
	chains := make([]core.Chain, len(defaultChains))
	copy(chains, defaultChains)
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
