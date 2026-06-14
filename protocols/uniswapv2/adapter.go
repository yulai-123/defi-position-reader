package uniswapv2

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
	"github.com/yulai-123/defi-position-reader/pkg/evm"
)

type MulticallRunner interface {
	Aggregate3(ctx context.Context, calls []evm.Call) ([]evm.Result, error)
}

type MulticallFactory func(chain core.Chain) (MulticallRunner, error)

type Adapter struct {
	multicallFactory MulticallFactory
}

type Option func(*Adapter)

func New(options ...Option) Adapter {
	item := Adapter{
		multicallFactory: defaultMulticallFactory,
	}
	for _, option := range options {
		option(&item)
	}
	return item
}

func WithMulticallFactory(factory MulticallFactory) Option {
	return func(a *Adapter) {
		if factory != nil {
			a.multicallFactory = factory
		}
	}
}

func (a Adapter) Descriptor() core.ProtocolDescriptor {
	return core.ProtocolDescriptor{
		ID:          ProtocolID,
		Name:        "Uniswap V2",
		Category:    "dex",
		Description: "Uniswap V2 constant-product liquidity pool and legacy UNI farming positions.",
		SupportedChains: []int64{
			chain.EthereumChainID,
			chain.ArbitrumChainID,
			chain.BaseChainID,
		},
	}
}

func (a Adapter) Syncer() adapter.MetadataSyncer {
	return Syncer{multicallFactory: a.multicallFactory}
}

func (a Adapter) Fetcher() adapter.PositionFetcher {
	return Fetcher{multicallFactory: a.multicallFactory}
}

func defaultMulticallFactory(target core.Chain) (MulticallRunner, error) {
	client, err := evm.NewClientFromEnv(target)
	if err != nil {
		return nil, err
	}
	runner, err := evm.NewMulticaller(client, common.HexToAddress(evm.Multicall3Address))
	if err != nil {
		return nil, err
	}
	return runner, nil
}

func requireMulticallFactory(factory MulticallFactory) (MulticallFactory, error) {
	if factory == nil {
		return nil, fmt.Errorf("uniswap v2 multicall factory is nil")
	}
	return factory, nil
}
