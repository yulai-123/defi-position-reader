package compoundv3

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
	"github.com/yulai-123/defi-position-reader/pkg/evm"
)

type Syncer struct {
	multicallFactory MulticallFactory
}

type marketBasics struct {
	config        MarketConfig
	name          string
	symbol        string
	decimals      uint8
	baseToken     common.Address
	basePriceFeed common.Address
	baseScale     uint64
	numAssets     uint8
}

type collateralSeed struct {
	market Market
	info   assetInfoOutput
}

type rewardSeed struct {
	market Market
	config rewardConfigOutput
}

func (s Syncer) Sync(ctx context.Context, req adapter.SyncRequest) (adapter.SyncResult, error) {
	factory, err := requireMulticallFactory(s.multicallFactory)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	configs, err := requireMarketConfigs(req.Chain.ID)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	runner, err := factory(req.Chain)
	if err != nil {
		return adapter.SyncResult{}, fmt.Errorf("create multicall runner: %w", err)
	}

	basics, err := loadMarketBasics(ctx, runner, configs)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	baseTokenMetadata, err := loadTokenMetadata(ctx, runner, req.Chain.ID, baseTokenAddresses(basics))
	if err != nil {
		return adapter.SyncResult{}, err
	}
	markets, err := buildMarkets(req.Chain.ID, basics, baseTokenMetadata)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	seeds, err := discoverCollateralSeeds(ctx, runner, markets)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	collateralTokenMetadata, err := loadTokenMetadata(ctx, runner, req.Chain.ID, collateralTokenAddresses(seeds))
	if err != nil {
		return adapter.SyncResult{}, err
	}
	collaterals, err := buildCollateralAssets(seeds, collateralTokenMetadata)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	rewardSeeds, err := discoverRewardSeeds(ctx, runner, markets)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	rewardTokenMetadata, err := loadTokenMetadata(ctx, runner, req.Chain.ID, rewardTokenAddresses(rewardSeeds))
	if err != nil {
		return adapter.SyncResult{}, err
	}
	rewards, err := buildRewardConfigs(rewardSeeds, rewardTokenMetadata)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	sort.Slice(markets, func(i, j int) bool { return markets[i].ID < markets[j].ID })
	sort.Slice(collaterals, func(i, j int) bool {
		if collaterals[i].MarketID == collaterals[j].MarketID {
			return collaterals[i].Asset.Symbol < collaterals[j].Asset.Symbol
		}
		return collaterals[i].MarketID < collaterals[j].MarketID
	})
	sort.Slice(rewards, func(i, j int) bool { return rewards[i].MarketID < rewards[j].MarketID })

	metadata := core.MetadataInfo{
		ChainID:     req.Chain.ID,
		Protocol:    ProtocolID,
		Namespace:   marketsNamespace,
		Version:     metadataVersion,
		BlockNumber: 0,
		UpdatedAt:   time.Now().UTC(),
		Source:      "compound-comet-deployments-and-onchain-multicall",
	}
	if req.Store != nil {
		for _, item := range []struct {
			namespace string
			value     any
		}{
			{marketsNamespace, markets},
			{collateralAssetsNamespace, collaterals},
			{rewardConfigsNamespace, rewards},
		} {
			if err := req.Store.Set(ctx, cache.Key{
				ChainID:   req.Chain.ID,
				Protocol:  ProtocolID,
				Namespace: item.namespace,
			}, item.value, core.MetadataInfo{
				Version:   metadata.Version,
				UpdatedAt: metadata.UpdatedAt,
				Source:    metadata.Source,
			}); err != nil {
				return adapter.SyncResult{}, err
			}
		}
	}

	return adapter.SyncResult{
		Metadata: metadata,
		Items:    len(markets) + len(collaterals) + len(rewards),
		Details: map[string]any{
			"discovery": buildDiscoverySteps(
				len(configs),
				len(configs)*7,
				len(baseTokenMetadata)*2,
				len(seeds),
				len(seeds),
				len(collateralTokenMetadata)*2,
				len(rewardSeeds),
				len(rewardSeeds),
				len(rewardTokenMetadata)*2,
				len(markets),
				len(collaterals),
				len(rewards),
			),
		},
	}, nil
}

func loadMarketBasics(ctx context.Context, runner MulticallRunner, configs []MarketConfig) ([]marketBasics, error) {
	methods := []string{"name", "symbol", "decimals", "baseToken", "baseTokenPriceFeed", "baseScale", "numAssets"}
	calls := make([]evm.Call, 0, len(configs)*len(methods))
	for _, config := range configs {
		target := common.HexToAddress(config.Comet)
		for _, method := range methods {
			data, err := packComet(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, evm.Call{Target: target, AllowFailure: false, CallData: data})
		}
	}
	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("compound v3 market basics: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]marketBasics, 0, len(configs))
	for i, config := range configs {
		base := i * len(methods)
		for j, method := range methods {
			if !results[base+j].Success {
				return nil, fmt.Errorf("compound v3 market %s: %s failed", config.ID, method)
			}
		}
		name, err := unpackCometString("name", results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		symbol, err := unpackCometString("symbol", results[base+1].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		decimals, err := unpackCometDecimals(results[base+2].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		baseToken, err := unpackBaseToken(results[base+3].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		basePriceFeed, err := unpackBaseTokenPriceFeed(results[base+4].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		baseScale, err := unpackBaseScale(results[base+5].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		numAssets, err := unpackNumAssets(results[base+6].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", config.ID, err)
		}
		out = append(out, marketBasics{
			config:        config,
			name:          name,
			symbol:        symbol,
			decimals:      decimals,
			baseToken:     baseToken,
			basePriceFeed: basePriceFeed,
			baseScale:     baseScale,
			numAssets:     numAssets,
		})
	}
	return out, nil
}

func buildMarkets(chainID int64, basics []marketBasics, tokens map[common.Address]core.Token) ([]Market, error) {
	markets := make([]Market, 0, len(basics))
	for _, basic := range basics {
		baseToken, ok := tokens[basic.baseToken]
		if !ok {
			return nil, fmt.Errorf("compound v3 market %s: base token metadata missing", basic.config.ID)
		}
		cometAddress := common.HexToAddress(basic.config.Comet).Hex()
		markets = append(markets, Market{
			ID:               basic.config.ID,
			Name:             basic.name,
			DisplayName:      basic.config.DisplayName,
			ChainID:          chainID,
			Comet:            cometAddress,
			Rewards:          common.HexToAddress(basic.config.Rewards).Hex(),
			DebankProtocolID: basic.config.DebankProtocolID,
			YieldPoolID:      strings.ToLower(cometAddress) + ":yield",
			LendingPoolID:    strings.ToLower(cometAddress) + ":lending",
			CometToken: core.Token{
				ChainID:  chainID,
				Address:  cometAddress,
				Symbol:   basic.symbol,
				Decimals: basic.decimals,
			},
			BaseToken:     baseToken,
			BasePriceFeed: basic.basePriceFeed.Hex(),
			BaseScale:     fmt.Sprint(basic.baseScale),
			NumAssets:     basic.numAssets,
		})
	}
	return markets, nil
}

func discoverCollateralSeeds(ctx context.Context, runner MulticallRunner, markets []Market) ([]collateralSeed, error) {
	calls := make([]evm.Call, 0)
	callMarkets := make([]Market, 0)
	for _, market := range markets {
		for i := uint8(0); i < market.NumAssets; i++ {
			data, err := packComet("getAssetInfo", i)
			if err != nil {
				return nil, err
			}
			calls = append(calls, evm.Call{
				Target:       common.HexToAddress(market.Comet),
				AllowFailure: false,
				CallData:     data,
			})
			callMarkets = append(callMarkets, market)
		}
	}
	if len(calls) == 0 {
		return nil, nil
	}
	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("compound v3 collateral discovery: got %d multicall results, want %d", len(results), len(calls))
	}
	seeds := make([]collateralSeed, 0, len(calls))
	for i, result := range results {
		market := callMarkets[i]
		if !result.Success {
			return nil, fmt.Errorf("compound v3 market %s: getAssetInfo failed", market.ID)
		}
		info, err := unpackAssetInfo(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s: %w", market.ID, err)
		}
		seeds = append(seeds, collateralSeed{market: market, info: info})
	}
	return seeds, nil
}

func buildCollateralAssets(seeds []collateralSeed, tokens map[common.Address]core.Token) ([]CollateralAsset, error) {
	out := make([]CollateralAsset, 0, len(seeds))
	for _, seed := range seeds {
		token, ok := tokens[seed.info.Asset]
		if !ok {
			return nil, fmt.Errorf("compound v3 market %s: collateral token metadata missing for %s", seed.market.ID, seed.info.Asset.Hex())
		}
		out = append(out, CollateralAsset{
			MarketID:                  seed.market.ID,
			Asset:                     token,
			PriceFeed:                 seed.info.PriceFeed.Hex(),
			Scale:                     fmt.Sprint(seed.info.Scale),
			BorrowCollateralFactor:    fmt.Sprint(seed.info.BorrowCollateralFactor),
			LiquidateCollateralFactor: fmt.Sprint(seed.info.LiquidateCollateralFactor),
			LiquidationFactor:         fmt.Sprint(seed.info.LiquidationFactor),
			SupplyCap:                 bigString(seed.info.SupplyCap),
		})
	}
	return out, nil
}

func discoverRewardSeeds(ctx context.Context, runner MulticallRunner, markets []Market) ([]rewardSeed, error) {
	calls := make([]evm.Call, 0, len(markets))
	callMarkets := make([]Market, 0, len(markets))
	for _, market := range markets {
		if market.Rewards == "" || market.Rewards == (common.Address{}).Hex() {
			continue
		}
		data, err := packRewards("rewardConfig", common.HexToAddress(market.Comet))
		if err != nil {
			return nil, err
		}
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.Rewards),
			AllowFailure: true,
			CallData:     data,
		})
		callMarkets = append(callMarkets, market)
	}
	if len(calls) == 0 {
		return nil, nil
	}
	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("compound v3 reward discovery: got %d multicall results, want %d", len(results), len(calls))
	}
	seeds := make([]rewardSeed, 0, len(results))
	for i, result := range results {
		market := callMarkets[i]
		if !result.Success {
			seeds = append(seeds, rewardSeed{market: market})
			continue
		}
		config, err := unpackRewardConfig(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 market %s rewards: %w", market.ID, err)
		}
		seeds = append(seeds, rewardSeed{market: market, config: config})
	}
	return seeds, nil
}

func buildRewardConfigs(seeds []rewardSeed, tokens map[common.Address]core.Token) ([]RewardConfig, error) {
	out := make([]RewardConfig, 0, len(seeds))
	for _, seed := range seeds {
		if seed.config.Token == (common.Address{}) {
			out = append(out, RewardConfig{
				MarketID:  seed.market.ID,
				Rewards:   seed.market.Rewards,
				Supported: false,
			})
			continue
		}
		token, ok := tokens[seed.config.Token]
		if !ok {
			return nil, fmt.Errorf("compound v3 market %s: reward token metadata missing for %s", seed.market.ID, seed.config.Token.Hex())
		}
		out = append(out, RewardConfig{
			MarketID:      seed.market.ID,
			Rewards:       seed.market.Rewards,
			Supported:     true,
			RewardToken:   token,
			RescaleFactor: fmt.Sprint(seed.config.RescaleFactor),
			ShouldUpscale: seed.config.ShouldUpscale,
			Multiplier:    bigString(seed.config.Multiplier),
		})
	}
	return out, nil
}

func loadTokenMetadata(ctx context.Context, runner MulticallRunner, chainID int64, addresses []common.Address) (map[common.Address]core.Token, error) {
	addresses = uniqueAddresses(addresses)
	if len(addresses) == 0 {
		return map[common.Address]core.Token{}, nil
	}
	symbolCall, err := packERC20("symbol")
	if err != nil {
		return nil, err
	}
	decimalsCall, err := packERC20("decimals")
	if err != nil {
		return nil, err
	}
	const callsPerToken = 2
	calls := make([]evm.Call, 0, len(addresses)*callsPerToken)
	for _, address := range addresses {
		calls = append(calls,
			evm.Call{Target: address, AllowFailure: false, CallData: symbolCall},
			evm.Call{Target: address, AllowFailure: false, CallData: decimalsCall},
		)
	}
	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("compound v3 token metadata: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make(map[common.Address]core.Token, len(addresses))
	for i, address := range addresses {
		symbolResult := results[i*callsPerToken]
		decimalsResult := results[i*callsPerToken+1]
		if !symbolResult.Success {
			return nil, fmt.Errorf("compound v3 token %s: symbol failed", address.Hex())
		}
		if !decimalsResult.Success {
			return nil, fmt.Errorf("compound v3 token %s: decimals failed", address.Hex())
		}
		symbol, err := unpackERC20Symbol(symbolResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 token %s: %w", address.Hex(), err)
		}
		decimals, err := unpackERC20Decimals(decimalsResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("compound v3 token %s: %w", address.Hex(), err)
		}
		out[address] = core.Token{
			ChainID:  chainID,
			Address:  address.Hex(),
			Symbol:   symbol,
			Decimals: decimals,
		}
	}
	return out, nil
}

func baseTokenAddresses(basics []marketBasics) []common.Address {
	out := make([]common.Address, 0, len(basics))
	for _, basic := range basics {
		out = append(out, basic.baseToken)
	}
	return out
}

func collateralTokenAddresses(seeds []collateralSeed) []common.Address {
	out := make([]common.Address, 0, len(seeds))
	for _, seed := range seeds {
		out = append(out, seed.info.Asset)
	}
	return out
}

func rewardTokenAddresses(seeds []rewardSeed) []common.Address {
	out := make([]common.Address, 0, len(seeds))
	for _, seed := range seeds {
		if seed.config.Token != (common.Address{}) {
			out = append(out, seed.config.Token)
		}
	}
	return out
}

func uniqueAddresses(addresses []common.Address) []common.Address {
	seen := make(map[common.Address]struct{}, len(addresses))
	out := make([]common.Address, 0, len(addresses))
	for _, address := range addresses {
		if address == (common.Address{}) {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hex() < out[j].Hex() })
	return out
}

func buildDiscoverySteps(markets, marketCalls, baseTokenCalls, collateralItems, collateralCalls, collateralTokenCalls, rewardItems, rewardCalls, rewardTokenCalls, marketItems, collateralMetadataItems, rewardMetadataItems int) []discoveryStep {
	return []discoveryStep{
		{
			Step:      "market configs",
			Operation: "local compound comet deployment config",
			Markets:   markets,
			Items:     markets,
			Status:    "ok",
			Notes:     "configured Comet markets for selected chain",
		},
		{
			Step:      "load market basics",
			Operation: "Comet.name/symbol/decimals/baseToken/baseTokenPriceFeed/baseScale/numAssets",
			Markets:   markets,
			Calls:     marketCalls,
			Items:     marketItems,
			Status:    "ok",
			Notes:     "one Comet market has one base asset",
		},
		{
			Step:      "load base token metadata",
			Operation: "ERC20.symbol + ERC20.decimals",
			Calls:     baseTokenCalls,
			Items:     baseTokenCalls / 2,
			Status:    "ok",
			Notes:     "unique base tokens",
		},
		{
			Step:      "discover collateral assets",
			Operation: "Comet.getAssetInfo",
			Markets:   markets,
			Calls:     collateralCalls,
			Items:     collateralItems,
			Status:    "ok",
			Notes:     "collateral assets are market-specific and oracle-backed",
		},
		{
			Step:      "load collateral token metadata",
			Operation: "ERC20.symbol + ERC20.decimals",
			Calls:     collateralTokenCalls,
			Items:     collateralTokenCalls / 2,
			Status:    "ok",
			Notes:     "unique collateral tokens",
		},
		{
			Step:         "discover reward configs",
			Operation:    "CometRewards.rewardConfig",
			Markets:      markets,
			Calls:        rewardCalls,
			Items:        rewardItems,
			Status:       "ok",
			Notes:        "unsupported reward configs are kept as disabled",
			AllowFailure: true,
		},
		{
			Step:      "load reward token metadata",
			Operation: "ERC20.symbol + ERC20.decimals",
			Calls:     rewardTokenCalls,
			Items:     rewardTokenCalls / 2,
			Status:    "ok",
			Notes:     "unique reward tokens",
		},
		{
			Step:      "cache write",
			Operation: "SQLite Set(markets, collateral-assets, reward-configs)",
			Items:     marketItems + collateralMetadataItems + rewardMetadataItems,
			Status:    "ok",
			Notes:     "fetcher requires all namespaces to exist",
		},
	}
}
