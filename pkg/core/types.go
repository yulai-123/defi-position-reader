package core

import "time"

// Chain describes an EVM network. RPCUrlEnv only stores the environment
// variable name, keeping sensitive endpoint configuration out of source code.
type Chain struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	DisplayName    string   `json:"displayName"`
	NativeCurrency string   `json:"nativeCurrency"`
	RPCUrlEnv      string   `json:"rpcUrlEnv"`
	PublicRPCURLs  []string `json:"publicRpcUrls,omitempty"`
}

type ProtocolDescriptor struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Category        string  `json:"category"`
	Description     string  `json:"description,omitempty"`
	SupportedChains []int64 `json:"supportedChains"`
}

func (d ProtocolDescriptor) SupportsChain(chainID int64) bool {
	for _, supported := range d.SupportedChains {
		if supported == chainID {
			return true
		}
	}
	return false
}

type PositionType string

const (
	PositionTypeUnknown   PositionType = "unknown"
	PositionTypeStaking   PositionType = "staking"
	PositionTypeLending   PositionType = "lending"
	PositionTypeLiquidity PositionType = "liquidity"
	PositionTypeYield     PositionType = "yield"
	PositionTypeReward    PositionType = "reward"
)

type Token struct {
	ChainID  int64  `json:"chainId"`
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals uint8  `json:"decimals"`
}

type TokenAmount struct {
	Token     Token  `json:"token"`
	Raw       string `json:"raw"`
	Formatted string `json:"formatted"`
}

// Position 是面向用户的协议资产结果，不承诺所有数据来自同一个区块快照。
// 调度器 metadata 的区块信息会单独保存在 MetadataInfo 中，后续可用于 trace 功能。
type Position struct {
	ID          string         `json:"id"`
	ChainID     int64          `json:"chainId"`
	Protocol    string         `json:"protocol"`
	Owner       string         `json:"owner"`
	Type        PositionType   `json:"type"`
	DisplayName string         `json:"displayName"`
	Shares      []TokenAmount  `json:"shares,omitempty"`
	Underlying  []TokenAmount  `json:"underlying,omitempty"`
	Rewards     []TokenAmount  `json:"rewards,omitempty"`
	Debt        []TokenAmount  `json:"debt,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// MetadataInfo describes the version and source of a cached protocol metadata dataset.
type MetadataInfo struct {
	ChainID     int64     `json:"chainId"`
	Protocol    string    `json:"protocol"`
	Namespace   string    `json:"namespace"`
	Version     string    `json:"version"`
	BlockNumber uint64    `json:"blockNumber,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Source      string    `json:"source,omitempty"`
}
