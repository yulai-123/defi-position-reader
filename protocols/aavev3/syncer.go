package aavev3

import (
	"context"
	"fmt"
	"math/big"
	"sort"
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

type reserveSeed struct {
	market MarketConfig
	asset  tokenDataOutput
}

type reserveDetails struct {
	seed          reserveSeed
	tokens        reserveTokenAddressesOutput
	configuration reserveConfigurationOutput
}

type symbolCall struct {
	marketID string
	asset    string
	role     string
	address  common.Address
}

type yieldVaultSeed struct {
	market  MarketConfig
	vault   common.Address
	kind    string
	factory string
}

type yieldVaultBasics struct {
	seed         yieldVaultSeed
	asset        common.Address
	aToken       common.Address
	rewardTokens []common.Address
}

type yieldFactoryCall struct {
	market  MarketConfig
	factory string
	method  string
	kind    string
}

type syncDiscoveryStep struct {
	Step      string `json:"step"`
	Operation string `json:"operation"`
	Markets   int    `json:"markets,omitempty"`
	Calls     int    `json:"calls,omitempty"`
	Items     int    `json:"items,omitempty"`
	Status    string `json:"status"`
	Notes     string `json:"notes,omitempty"`
}

func (s Syncer) Sync(ctx context.Context, req adapter.SyncRequest) (adapter.SyncResult, error) {
	factory, err := requireMulticallFactory(s.multicallFactory)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	markets, err := requireMarketConfigs(req.Chain.ID)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	runner, err := factory(req.Chain)
	if err != nil {
		return adapter.SyncResult{}, fmt.Errorf("create multicall runner: %w", err)
	}

	seeds, err := discoverReserveSeeds(ctx, runner, markets)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	reserveDiscoveryCalls := len(markets)
	details, err := loadReserveDetails(ctx, runner, seeds)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	reserveDetailCalls := len(seeds) * 2
	symbols, err := loadTokenSymbols(ctx, runner, details)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	reserveSymbolCalls := countReserveSymbolCalls(details)

	reserves, err := buildLendingReserves(req.Chain.ID, details, symbols)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	sort.Slice(reserves, func(i, j int) bool {
		if reserves[i].MarketID == reserves[j].MarketID {
			return reserves[i].Asset.Symbol < reserves[j].Asset.Symbol
		}
		return reserves[i].MarketID < reserves[j].MarketID
	})

	vaultSeeds, err := discoverYieldVaultSeeds(ctx, runner, markets)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	yieldFactoryCalls := countYieldFactoryCalls(markets)
	vaultBasics, err := loadYieldVaultBasics(ctx, runner, vaultSeeds)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	vaultBasicCalls := len(vaultSeeds) * 3
	yieldTokenMetadata, err := loadYieldTokenMetadata(ctx, runner, req.Chain.ID, vaultBasics)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	yieldTokenMetadataCalls := len(yieldTokenMetadata) * 2
	yieldVaults, err := buildYieldVaults(vaultBasics, yieldTokenMetadata)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	sort.Slice(yieldVaults, func(i, j int) bool {
		if yieldVaults[i].MarketID == yieldVaults[j].MarketID {
			return yieldVaults[i].VaultToken.Symbol < yieldVaults[j].VaultToken.Symbol
		}
		return yieldVaults[i].MarketID < yieldVaults[j].MarketID
	})

	metadata := core.MetadataInfo{
		ChainID:     req.Chain.ID,
		Protocol:    ProtocolID,
		Namespace:   lendingReservesNamespace,
		Version:     lendingMetadataVersion,
		BlockNumber: 0,
		UpdatedAt:   time.Now().UTC(),
		Source:      "aave-address-book-and-onchain-multicall",
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
		if err := req.Store.Set(ctx, cache.Key{
			ChainID:   req.Chain.ID,
			Protocol:  ProtocolID,
			Namespace: lendingReservesNamespace,
		}, reserves, metadata); err != nil {
			return adapter.SyncResult{}, err
		}
		if err := req.Store.Set(ctx, cache.Key{
			ChainID:   req.Chain.ID,
			Protocol:  ProtocolID,
			Namespace: yieldVaultsNamespace,
		}, yieldVaults, core.MetadataInfo{
			ChainID:     req.Chain.ID,
			Protocol:    ProtocolID,
			Namespace:   yieldVaultsNamespace,
			Version:     yieldMetadataVersion,
			BlockNumber: 0,
			UpdatedAt:   metadata.UpdatedAt,
			Source:      metadata.Source,
		}); err != nil {
			return adapter.SyncResult{}, err
		}
	}

	return adapter.SyncResult{
		Metadata: metadata,
		Items:    len(reserves) + len(yieldVaults),
		Details: map[string]any{
			"discovery": buildSyncDiscoverySteps(
				len(markets),
				reserveDiscoveryCalls,
				len(seeds),
				reserveDetailCalls,
				len(details),
				reserveSymbolCalls,
				len(symbols),
				yieldFactoryCalls,
				len(vaultSeeds),
				vaultBasicCalls,
				len(vaultBasics),
				yieldTokenMetadataCalls,
				len(yieldTokenMetadata),
				len(reserves),
				len(yieldVaults),
			),
		},
	}, nil
}

func buildSyncDiscoverySteps(
	markets int,
	reserveDiscoveryCalls int,
	reserveSeeds int,
	reserveDetailCalls int,
	reserveDetails int,
	reserveSymbolCalls int,
	reserveSymbols int,
	yieldFactoryCalls int,
	vaultSeeds int,
	vaultBasicCalls int,
	vaultBasics int,
	yieldTokenMetadataCalls int,
	yieldTokenMetadata int,
	reserves int,
	yieldVaults int,
) []syncDiscoveryStep {
	return []syncDiscoveryStep{
		{
			Step:      "market configs",
			Operation: "local aave address book config",
			Markets:   markets,
			Items:     markets,
			Status:    "ok",
			Notes:     "configured markets for selected chain",
		},
		{
			Step:      "discover lending reserves",
			Operation: "DataProvider.getAllReservesTokens",
			Markets:   markets,
			Calls:     reserveDiscoveryCalls,
			Items:     reserveSeeds,
			Status:    "ok",
			Notes:     "one call per market",
		},
		{
			Step:      "load reserve metadata",
			Operation: "getReserveTokensAddresses + getReserveConfigurationData",
			Markets:   markets,
			Calls:     reserveDetailCalls,
			Items:     reserveDetails,
			Status:    "ok",
			Notes:     "two calls per reserve",
		},
		{
			Step:      "load reserve token symbols",
			Operation: "ERC20.symbol",
			Calls:     reserveSymbolCalls,
			Items:     reserveSymbols,
			Status:    "ok",
			Notes:     "aToken and debt token symbols",
		},
		{
			Step:      "discover yield vaults",
			Operation: "StataTokenFactory.getStataTokens + getStaticATokens",
			Markets:   markets,
			Calls:     yieldFactoryCalls,
			Items:     vaultSeeds,
			Status:    "ok",
			Notes:     "markets without factories are skipped",
		},
		{
			Step:      "load vault basics",
			Operation: "Vault.asset + Vault.aToken + Vault.rewardTokens",
			Calls:     vaultBasicCalls,
			Items:     vaultBasics,
			Status:    "ok",
			Notes:     "three calls per discovered vault",
		},
		{
			Step:      "load vault token metadata",
			Operation: "ERC20.symbol + ERC20.decimals",
			Calls:     yieldTokenMetadataCalls,
			Items:     yieldTokenMetadata,
			Status:    "ok",
			Notes:     "unique vault, asset, aToken and reward tokens",
		},
		{
			Step:      "cache write",
			Operation: "SQLite Set(markets, lending-reserves, yield-vaults)",
			Items:     markets + reserves + yieldVaults,
			Status:    "ok",
			Notes:     "fetcher requires all namespaces to exist",
		},
	}
}

func countReserveSymbolCalls(details []reserveDetails) int {
	calls := 0
	for _, detail := range details {
		for _, address := range []common.Address{
			detail.tokens.ATokenAddress,
			detail.tokens.StableDebtTokenAddress,
			detail.tokens.VariableDebtTokenAddress,
		} {
			if address != (common.Address{}) {
				calls++
			}
		}
	}
	return calls
}

func countYieldFactoryCalls(markets []MarketConfig) int {
	calls := 0
	for _, market := range markets {
		if market.StataFactory != "" {
			calls++
		}
		if market.LegacyStaticFactory != "" {
			calls++
		}
	}
	return calls
}

func discoverReserveSeeds(ctx context.Context, runner MulticallRunner, markets []MarketConfig) ([]reserveSeed, error) {
	calls := make([]evm.Call, 0, len(markets))
	for _, market := range markets {
		data, err := packDataProvider("getAllReservesTokens")
		if err != nil {
			return nil, err
		}
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.DataProvider),
			AllowFailure: false,
			CallData:     data,
		})
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(markets) {
		return nil, fmt.Errorf("discover reserves: got %d multicall results, want %d", len(results), len(markets))
	}

	seeds := make([]reserveSeed, 0)
	for i, result := range results {
		market := markets[i]
		if !result.Success {
			return nil, fmt.Errorf("discover reserves: market %s getAllReservesTokens failed", market.ID)
		}
		tokens, err := unpackAllReservesTokens(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("discover reserves: market %s: %w", market.ID, err)
		}
		for _, token := range tokens {
			seeds = append(seeds, reserveSeed{
				market: market,
				asset:  token,
			})
		}
	}
	return seeds, nil
}

func loadReserveDetails(ctx context.Context, runner MulticallRunner, seeds []reserveSeed) ([]reserveDetails, error) {
	const callsPerReserve = 2
	calls := make([]evm.Call, 0, len(seeds)*callsPerReserve)
	for _, seed := range seeds {
		tokensCall, err := packDataProvider("getReserveTokensAddresses", seed.asset.TokenAddress)
		if err != nil {
			return nil, err
		}
		configCall, err := packDataProvider("getReserveConfigurationData", seed.asset.TokenAddress)
		if err != nil {
			return nil, err
		}
		target := common.HexToAddress(seed.market.DataProvider)
		calls = append(calls,
			evm.Call{Target: target, AllowFailure: false, CallData: tokensCall},
			evm.Call{Target: target, AllowFailure: false, CallData: configCall},
		)
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("load reserve details: got %d multicall results, want %d", len(results), len(calls))
	}

	details := make([]reserveDetails, 0, len(seeds))
	for i, seed := range seeds {
		tokensResult := results[i*callsPerReserve]
		configResult := results[i*callsPerReserve+1]
		if !tokensResult.Success {
			return nil, fmt.Errorf("load reserve details: market %s asset %s token addresses failed", seed.market.ID, seed.asset.TokenAddress.Hex())
		}
		if !configResult.Success {
			return nil, fmt.Errorf("load reserve details: market %s asset %s configuration failed", seed.market.ID, seed.asset.TokenAddress.Hex())
		}

		tokens, err := unpackReserveTokenAddresses(tokensResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load reserve details: market %s asset %s: %w", seed.market.ID, seed.asset.TokenAddress.Hex(), err)
		}
		configuration, err := unpackReserveConfiguration(configResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load reserve details: market %s asset %s: %w", seed.market.ID, seed.asset.TokenAddress.Hex(), err)
		}
		details = append(details, reserveDetails{
			seed:          seed,
			tokens:        tokens,
			configuration: configuration,
		})
	}
	return details, nil
}

func loadTokenSymbols(ctx context.Context, runner MulticallRunner, details []reserveDetails) (map[string]string, error) {
	symbolData, err := packERC20("symbol")
	if err != nil {
		return nil, err
	}

	symbolCalls := make([]symbolCall, 0, len(details)*3)
	calls := make([]evm.Call, 0, len(details)*3)
	for _, detail := range details {
		candidates := []symbolCall{
			{marketID: detail.seed.market.ID, asset: detail.seed.asset.Symbol, role: "aToken", address: detail.tokens.ATokenAddress},
			{marketID: detail.seed.market.ID, asset: detail.seed.asset.Symbol, role: "stableDebtToken", address: detail.tokens.StableDebtTokenAddress},
			{marketID: detail.seed.market.ID, asset: detail.seed.asset.Symbol, role: "variableDebtToken", address: detail.tokens.VariableDebtTokenAddress},
		}
		for _, candidate := range candidates {
			if candidate.address == (common.Address{}) {
				continue
			}
			symbolCalls = append(symbolCalls, candidate)
			calls = append(calls, evm.Call{
				Target:       candidate.address,
				AllowFailure: false,
				CallData:     symbolData,
			})
		}
	}
	if len(calls) == 0 {
		return map[string]string{}, nil
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("load token symbols: got %d multicall results, want %d", len(results), len(calls))
	}

	symbols := make(map[string]string, len(symbolCalls))
	for i, result := range results {
		call := symbolCalls[i]
		if !result.Success {
			return nil, fmt.Errorf("load token symbols: market %s asset %s %s symbol failed", call.marketID, call.asset, call.role)
		}
		symbol, err := unpackSymbol(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load token symbols: market %s asset %s %s: %w", call.marketID, call.asset, call.role, err)
		}
		if symbol == "" {
			symbol = fallbackTokenSymbol(call.role, call.asset)
		}
		symbols[tokenSymbolKey(call.marketID, call.role, call.address)] = symbol
	}
	return symbols, nil
}

func discoverYieldVaultSeeds(ctx context.Context, runner MulticallRunner, markets []MarketConfig) ([]yieldVaultSeed, error) {
	factoryCalls := make([]yieldFactoryCall, 0)
	calls := make([]evm.Call, 0)
	for _, market := range markets {
		candidates := []yieldFactoryCall{
			{market: market, factory: market.StataFactory, method: "getStataTokens", kind: "stata-token"},
			{market: market, factory: market.LegacyStaticFactory, method: "getStaticATokens", kind: "static-a-token"},
		}
		for _, candidate := range candidates {
			if candidate.factory == "" {
				continue
			}
			data, err := packStataFactory(candidate.method)
			if err != nil {
				return nil, err
			}
			factoryCalls = append(factoryCalls, candidate)
			calls = append(calls, evm.Call{
				Target:       common.HexToAddress(candidate.factory),
				AllowFailure: false,
				CallData:     data,
			})
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
		return nil, fmt.Errorf("discover yield vaults: got %d multicall results, want %d", len(results), len(calls))
	}

	seeds := make([]yieldVaultSeed, 0)
	for i, result := range results {
		factoryCall := factoryCalls[i]
		market := factoryCall.market
		if !result.Success {
			return nil, fmt.Errorf("discover yield vaults: market %s %s failed", market.ID, factoryCall.method)
		}
		var vaults []common.Address
		var err error
		switch factoryCall.method {
		case "getStataTokens":
			vaults, err = unpackStataTokens(result.ReturnData)
		case "getStaticATokens":
			vaults, err = unpackStaticATokens(result.ReturnData)
		default:
			err = fmt.Errorf("unsupported factory method %s", factoryCall.method)
		}
		if err != nil {
			return nil, fmt.Errorf("discover yield vaults: market %s %s: %w", market.ID, factoryCall.method, err)
		}
		for _, vault := range vaults {
			if vault == (common.Address{}) {
				continue
			}
			seeds = append(seeds, yieldVaultSeed{
				market:  market,
				vault:   vault,
				kind:    factoryCall.kind,
				factory: factoryCall.factory,
			})
		}
	}
	return seeds, nil
}

func loadYieldVaultBasics(ctx context.Context, runner MulticallRunner, seeds []yieldVaultSeed) ([]yieldVaultBasics, error) {
	const callsPerVault = 3
	calls := make([]evm.Call, 0, len(seeds)*callsPerVault)
	for _, seed := range seeds {
		assetCall, err := packYieldVault("asset")
		if err != nil {
			return nil, err
		}
		aTokenCall, err := packYieldVault("aToken")
		if err != nil {
			return nil, err
		}
		rewardTokensCall, err := packYieldVault("rewardTokens")
		if err != nil {
			return nil, err
		}
		calls = append(calls,
			evm.Call{Target: seed.vault, AllowFailure: false, CallData: assetCall},
			evm.Call{Target: seed.vault, AllowFailure: false, CallData: aTokenCall},
			evm.Call{Target: seed.vault, AllowFailure: false, CallData: rewardTokensCall},
		)
	}
	if len(calls) == 0 {
		return nil, nil
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("load yield vault basics: got %d multicall results, want %d", len(results), len(calls))
	}

	basics := make([]yieldVaultBasics, 0, len(seeds))
	for i, seed := range seeds {
		assetResult := results[i*callsPerVault]
		aTokenResult := results[i*callsPerVault+1]
		rewardTokensResult := results[i*callsPerVault+2]
		if !assetResult.Success {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s asset failed", seed.market.ID, seed.vault.Hex())
		}
		if !aTokenResult.Success {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s aToken failed", seed.market.ID, seed.vault.Hex())
		}
		if !rewardTokensResult.Success {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s rewardTokens failed", seed.market.ID, seed.vault.Hex())
		}

		asset, err := unpackVaultAsset(assetResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s: %w", seed.market.ID, seed.vault.Hex(), err)
		}
		aToken, err := unpackVaultAToken(aTokenResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s: %w", seed.market.ID, seed.vault.Hex(), err)
		}
		rewardTokens, err := unpackVaultRewardTokens(rewardTokensResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load yield vault basics: market %s vault %s: %w", seed.market.ID, seed.vault.Hex(), err)
		}
		basics = append(basics, yieldVaultBasics{
			seed:         seed,
			asset:        asset,
			aToken:       aToken,
			rewardTokens: rewardTokens,
		})
	}
	return basics, nil
}

func loadYieldTokenMetadata(ctx context.Context, runner MulticallRunner, chainID int64, basics []yieldVaultBasics) (map[common.Address]core.Token, error) {
	addressSet := make(map[common.Address]struct{})
	for _, basic := range basics {
		addressSet[basic.seed.vault] = struct{}{}
		addressSet[basic.asset] = struct{}{}
		addressSet[basic.aToken] = struct{}{}
		for _, rewardToken := range basic.rewardTokens {
			if rewardToken != (common.Address{}) {
				addressSet[rewardToken] = struct{}{}
			}
		}
	}
	if len(addressSet) == 0 {
		return map[common.Address]core.Token{}, nil
	}

	addresses := make([]common.Address, 0, len(addressSet))
	for address := range addressSet {
		if address != (common.Address{}) {
			addresses = append(addresses, address)
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Hex() < addresses[j].Hex()
	})

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
		return nil, fmt.Errorf("load yield token metadata: got %d multicall results, want %d", len(results), len(calls))
	}

	tokens := make(map[common.Address]core.Token, len(addresses))
	for i, address := range addresses {
		symbolResult := results[i*callsPerToken]
		decimalsResult := results[i*callsPerToken+1]
		if !symbolResult.Success {
			return nil, fmt.Errorf("load yield token metadata: token %s symbol failed", address.Hex())
		}
		if !decimalsResult.Success {
			return nil, fmt.Errorf("load yield token metadata: token %s decimals failed", address.Hex())
		}
		symbol, err := unpackSymbol(symbolResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load yield token metadata: token %s: %w", address.Hex(), err)
		}
		decimals, err := unpackTokenDecimals(decimalsResult.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("load yield token metadata: token %s: %w", address.Hex(), err)
		}
		tokens[address] = core.Token{
			ChainID:  chainID,
			Address:  address.Hex(),
			Symbol:   symbol,
			Decimals: decimals,
		}
	}
	return tokens, nil
}

func buildYieldVaults(basics []yieldVaultBasics, tokens map[common.Address]core.Token) ([]YieldVault, error) {
	vaults := make([]YieldVault, 0, len(basics))
	for _, basic := range basics {
		vaultToken, ok := tokens[basic.seed.vault]
		if !ok {
			return nil, fmt.Errorf("build yield vaults: vault %s metadata missing", basic.seed.vault.Hex())
		}
		asset, ok := tokens[basic.asset]
		if !ok {
			return nil, fmt.Errorf("build yield vaults: vault %s asset %s metadata missing", basic.seed.vault.Hex(), basic.asset.Hex())
		}
		aToken, ok := tokens[basic.aToken]
		if !ok {
			return nil, fmt.Errorf("build yield vaults: vault %s aToken %s metadata missing", basic.seed.vault.Hex(), basic.aToken.Hex())
		}
		rewards := make([]core.Token, 0, len(basic.rewardTokens))
		for _, rewardToken := range basic.rewardTokens {
			if rewardToken == (common.Address{}) {
				continue
			}
			reward, ok := tokens[rewardToken]
			if !ok {
				return nil, fmt.Errorf("build yield vaults: vault %s reward %s metadata missing", basic.seed.vault.Hex(), rewardToken.Hex())
			}
			rewards = append(rewards, reward)
		}
		sort.Slice(rewards, func(i, j int) bool {
			if rewards[i].Symbol == rewards[j].Symbol {
				return rewards[i].Address < rewards[j].Address
			}
			return rewards[i].Symbol < rewards[j].Symbol
		})

		vaults = append(vaults, YieldVault{
			MarketID:     basic.seed.market.ID,
			Kind:         basic.seed.kind,
			Factory:      basic.seed.factory,
			VaultToken:   vaultToken,
			Asset:        asset,
			AToken:       aToken,
			RewardTokens: rewards,
		})
	}
	return vaults, nil
}

func buildLendingReserves(chainID int64, details []reserveDetails, symbols map[string]string) ([]LendingReserve, error) {
	reserves := make([]LendingReserve, 0, len(details))
	for _, detail := range details {
		decimals, err := uint8FromBig(detail.configuration.Decimals)
		if err != nil {
			return nil, fmt.Errorf("market %s asset %s decimals: %w", detail.seed.market.ID, detail.seed.asset.TokenAddress.Hex(), err)
		}
		asset := core.Token{
			ChainID:  chainID,
			Address:  detail.seed.asset.TokenAddress.Hex(),
			Symbol:   detail.seed.asset.Symbol,
			Decimals: decimals,
		}
		reserves = append(reserves, LendingReserve{
			MarketID: detail.seed.market.ID,
			Asset:    asset,
			AToken: core.Token{
				ChainID:  chainID,
				Address:  detail.tokens.ATokenAddress.Hex(),
				Symbol:   symbolFromMap(symbols, detail.seed.market.ID, "aToken", detail.tokens.ATokenAddress, fallbackTokenSymbol("aToken", asset.Symbol)),
				Decimals: decimals,
			},
			StableDebtToken: core.Token{
				ChainID:  chainID,
				Address:  detail.tokens.StableDebtTokenAddress.Hex(),
				Symbol:   symbolFromMap(symbols, detail.seed.market.ID, "stableDebtToken", detail.tokens.StableDebtTokenAddress, fallbackTokenSymbol("stableDebtToken", asset.Symbol)),
				Decimals: decimals,
			},
			VariableDebtToken: core.Token{
				ChainID:  chainID,
				Address:  detail.tokens.VariableDebtTokenAddress.Hex(),
				Symbol:   symbolFromMap(symbols, detail.seed.market.ID, "variableDebtToken", detail.tokens.VariableDebtTokenAddress, fallbackTokenSymbol("variableDebtToken", asset.Symbol)),
				Decimals: decimals,
			},
			LTV:                      bigString(detail.configuration.LTV),
			LiquidationThreshold:     bigString(detail.configuration.LiquidationThreshold),
			LiquidationBonus:         bigString(detail.configuration.LiquidationBonus),
			ReserveFactor:            bigString(detail.configuration.ReserveFactor),
			UsageAsCollateralEnabled: detail.configuration.UsageAsCollateralEnabled,
			BorrowingEnabled:         detail.configuration.BorrowingEnabled,
			StableBorrowRateEnabled:  detail.configuration.StableBorrowRateEnabled,
			IsActive:                 detail.configuration.IsActive,
			IsFrozen:                 detail.configuration.IsFrozen,
		})
	}
	return reserves, nil
}

func uint8FromBig(value *big.Int) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("value is nil")
	}
	if value.Sign() < 0 || value.BitLen() > 8 {
		return 0, fmt.Errorf("value %s does not fit uint8", value.String())
	}
	return uint8(value.Uint64()), nil
}

func bigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func tokenSymbolKey(marketID string, role string, address common.Address) string {
	return marketID + ":" + role + ":" + address.Hex()
}

func symbolFromMap(symbols map[string]string, marketID string, role string, address common.Address, fallback string) string {
	if value := symbols[tokenSymbolKey(marketID, role, address)]; value != "" {
		return value
	}
	return fallback
}

func fallbackTokenSymbol(role string, asset string) string {
	switch role {
	case "aToken":
		return "a" + asset
	case "stableDebtToken":
		return "stableDebt" + asset
	case "variableDebtToken":
		return "variableDebt" + asset
	default:
		return asset
	}
}
