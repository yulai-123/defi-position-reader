package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

const (
	ProtocolID       = "demo"
	marketsNamespace = "markets"
	metadataVersion  = "demo-v1"
)

type Adapter struct{}

func New() Adapter {
	return Adapter{}
}

func (Adapter) Descriptor() core.ProtocolDescriptor {
	return core.ProtocolDescriptor{
		ID:          ProtocolID,
		Name:        "Demo Protocol",
		Category:    "fixture",
		Description: "离线演示用协议，展示 metadata sync、cache 和 position fetch 的完整流程。",
		SupportedChains: []int64{
			chain.EthereumChainID,
			chain.ArbitrumChainID,
			chain.BaseChainID,
		},
	}
}

func (Adapter) Syncer() adapter.MetadataSyncer {
	return Syncer{}
}

func (Adapter) Fetcher() adapter.PositionFetcher {
	return Fetcher{}
}

type Market struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	ShareToken  core.Token   `json:"shareToken"`
	Underlying  []core.Token `json:"underlying"`
}

type Syncer struct{}

func (Syncer) Sync(ctx context.Context, req adapter.SyncRequest) (adapter.SyncResult, error) {
	markets := defaultMarkets(req.Chain)
	metadata := core.MetadataInfo{
		ChainID:     req.Chain.ID,
		Protocol:    ProtocolID,
		Namespace:   marketsNamespace,
		Version:     metadataVersion,
		BlockNumber: 0,
		UpdatedAt:   time.Now().UTC(),
		Source:      "embedded-demo-fixture",
	}

	if req.Store != nil {
		if err := req.Store.Set(ctx, cache.Key{
			ChainID:   req.Chain.ID,
			Protocol:  ProtocolID,
			Namespace: marketsNamespace,
		}, markets, core.MetadataInfo{
			Version:   metadata.Version,
			UpdatedAt: metadata.UpdatedAt,
			Source:    metadata.Source,
		}); err != nil {
			return adapter.SyncResult{}, err
		}
	}

	return adapter.SyncResult{
		Metadata: metadata,
		Items:    len(markets),
	}, nil
}

type Fetcher struct{}

func (Fetcher) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	markets, err := loadMarkets(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return nil, nil
	}

	owner := strings.ToLower(strings.TrimSpace(req.Owner))
	market := markets[0]
	if len(market.Underlying) < 2 {
		return nil, fmt.Errorf("demo market %q has %d underlying tokens, want at least 2", market.ID, len(market.Underlying))
	}

	return []core.Position{
		{
			ID:          fmt.Sprintf("%s:%d:%s:%s", ProtocolID, req.Chain.ID, owner, market.ID),
			Protocol:    ProtocolID,
			ChainID:     req.Chain.ID,
			Owner:       owner,
			Type:        core.PositionTypeLiquidity,
			DisplayName: market.DisplayName,
			Shares: []core.TokenAmount{
				{
					Token:     market.ShareToken,
					Raw:       "1500000000000000000",
					Formatted: "1.5",
				},
			},
			Underlying: []core.TokenAmount{
				{
					Token:     market.Underlying[0],
					Raw:       "420000000000000000",
					Formatted: "0.42",
				},
				{
					Token:     market.Underlying[1],
					Raw:       "1234500000",
					Formatted: "1234.5",
				},
			},
			Extra: map[string]any{
				"metadata": "demo markets can be refreshed with sync-metadata",
			},
		},
	}, nil
}

func loadMarkets(ctx context.Context, req adapter.FetchRequest) ([]Market, error) {
	if req.Store == nil {
		return defaultMarkets(req.Chain), nil
	}

	var markets []Market
	_, err := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets)
	if err == nil {
		return markets, nil
	}
	if errors.Is(err, cache.ErrNotFound) {
		return defaultMarkets(req.Chain), nil
	}
	return nil, err
}

func defaultMarkets(target core.Chain) []Market {
	return []Market{
		{
			ID:          "weth-usdc-demo-lp",
			DisplayName: "Demo WETH / USDC LP",
			ShareToken: core.Token{
				ChainID:  target.ID,
				Address:  "0x000000000000000000000000000000000000dead",
				Symbol:   "dLP",
				Decimals: 18,
			},
			Underlying: []core.Token{
				wethToken(target.ID),
				usdcToken(target.ID),
			},
		},
	}
}

func wethToken(chainID int64) core.Token {
	address := map[int64]string{
		chain.EthereumChainID: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		chain.ArbitrumChainID: "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1",
		chain.BaseChainID:     "0x4200000000000000000000000000000000000006",
	}[chainID]
	return core.Token{
		ChainID:  chainID,
		Address:  address,
		Symbol:   "WETH",
		Decimals: 18,
	}
}

func usdcToken(chainID int64) core.Token {
	address := map[int64]string{
		chain.EthereumChainID: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		chain.ArbitrumChainID: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		chain.BaseChainID:     "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
	}[chainID]
	return core.Token{
		ChainID:  chainID,
		Address:  address,
		Symbol:   "USDC",
		Decimals: 6,
	}
}
