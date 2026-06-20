package uniswapv3

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
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

type poolSeed struct {
	market       Market
	address      common.Address
	token0       common.Address
	token1       common.Address
	fee          uint32
	tickSpacing  int32
	createdBlock uint64
}

type discoveryOptions struct {
	mode        string
	hasMaxPools bool
	maxPools    uint64
	owner       common.Address
	hasOwner    bool
}

const (
	discoveryModeUser    = "user"
	discoveryModeLimited = "limited"
	discoveryModeFull    = "full"
)

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

	markets := buildMarkets(req.Chain.ID, configs)
	options, err := discoveryOptionsFromEnv(req.Owner)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	var seeds []poolSeed
	if options.mode == discoveryModeUser {
		seeds, err = discoverOwnerPoolSeeds(ctx, runner, markets, options.owner)
		if err != nil {
			return adapter.SyncResult{}, err
		}
	} else {
		client, err := evm.NewClientFromEnv(req.Chain)
		if err != nil {
			return adapter.SyncResult{}, err
		}
		for _, market := range markets {
			marketSeeds, err := discoverPoolSeedsFromLogs(ctx, client, market, options)
			if err != nil {
				return adapter.SyncResult{}, err
			}
			seeds = append(seeds, marketSeeds...)
		}
	}
	seeds = uniquePoolSeeds(seeds)

	tokens, err := loadTokenMetadata(ctx, runner, req.Chain.ID, tokenAddressesFromPoolSeeds(seeds))
	if err != nil {
		return adapter.SyncResult{}, err
	}
	pools, err := buildPools(seeds, tokens)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	sort.Slice(markets, func(i, j int) bool { return markets[i].ID < markets[j].ID })
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].Token0.Address == pools[j].Token0.Address {
			if pools[i].Token1.Address == pools[j].Token1.Address {
				return pools[i].Fee < pools[j].Fee
			}
			return strings.ToLower(pools[i].Token1.Address) < strings.ToLower(pools[j].Token1.Address)
		}
		return strings.ToLower(pools[i].Token0.Address) < strings.ToLower(pools[j].Token0.Address)
	})

	metadata := core.MetadataInfo{
		ChainID:     req.Chain.ID,
		Protocol:    ProtocolID,
		Namespace:   poolsNamespace,
		Version:     metadataVersion,
		BlockNumber: 0,
		UpdatedAt:   time.Now().UTC(),
		Source:      syncSource(options),
	}
	if req.Store != nil {
		for _, item := range []struct {
			namespace string
			value     any
		}{
			{marketsNamespace, markets},
			{poolsNamespace, pools},
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
		Items:    len(markets) + len(pools),
		Details: map[string]any{
			"discovery": buildDiscoverySteps(len(markets), len(seeds), len(tokens), len(pools), options),
		},
	}, nil
}

func buildMarkets(chainID int64, configs []MarketConfig) []Market {
	out := make([]Market, 0, len(configs))
	for _, config := range configs {
		out = append(out, Market{
			ID:               config.ID,
			Name:             config.Name,
			DisplayName:      config.DisplayName,
			ChainID:          chainID,
			Factory:          common.HexToAddress(config.Factory).Hex(),
			PositionManager:  common.HexToAddress(config.PositionManager).Hex(),
			DebankProtocolID: config.DebankProtocolID,
			StartBlock:       config.StartBlock,
		})
	}
	return out
}

func discoverPoolSeedsFromLogs(ctx context.Context, client *evm.Client, market Market, options discoveryOptions) ([]poolSeed, error) {
	if options.mode != discoveryModeFull && options.mode != discoveryModeLimited {
		return nil, nil
	}
	latest, err := client.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("read latest block for pool discovery: %w", err)
	}

	startBlock := market.StartBlock
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V3_LOG_FROM_BLOCK")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DPR_UNISWAP_V3_LOG_FROM_BLOCK: %w", err)
		}
		startBlock = value
	}
	if startBlock > latest {
		return nil, nil
	}
	chunkSize := uint64(defaultLogBlockChunk)
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V3_LOG_BLOCK_CHUNK")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DPR_UNISWAP_V3_LOG_BLOCK_CHUNK: %w", err)
		}
		if value > 0 {
			chunkSize = value
		}
	}

	out := make([]poolSeed, 0)
	for fromBlock := startBlock; fromBlock <= latest; {
		toBlock := fromBlock + chunkSize - 1
		if toBlock > latest || toBlock < fromBlock {
			toBlock = latest
		}
		logs, err := client.FilterLogs(ctx, evm.LogQuery{
			FromBlock: fromBlock,
			ToBlock:   toBlock,
			Addresses: []common.Address{common.HexToAddress(market.Factory)},
			Topics:    []any{poolCreatedEvent.ID.Hex()},
		})
		if err != nil {
			return nil, fmt.Errorf("discover uniswap v3 pools blocks %d-%d: %w", fromBlock, toBlock, err)
		}
		for _, item := range logs {
			created, err := unpackPoolCreatedLog(item.Topics, item.Data)
			if err != nil {
				return nil, fmt.Errorf("decode PoolCreated log block=%d index=%d: %w", item.BlockNumber, item.LogIndex, err)
			}
			out = append(out, poolSeed{
				market:       market,
				address:      created.Pool,
				token0:       created.Token0,
				token1:       created.Token1,
				fee:          created.Fee,
				tickSpacing:  created.TickSpacing,
				createdBlock: item.BlockNumber,
			})
			if options.hasMaxPools && uint64(len(out)) >= options.maxPools {
				return out, nil
			}
		}
		if toBlock == latest {
			break
		}
		fromBlock = toBlock + 1
	}
	return out, nil
}

func discoverOwnerPoolSeeds(ctx context.Context, runner MulticallRunner, markets []Market, owner common.Address) ([]poolSeed, error) {
	if owner == (common.Address{}) {
		return nil, nil
	}
	positionsByMarket, err := fetchOwnerPositionMetadata(ctx, runner, markets, owner)
	if err != nil {
		return nil, err
	}
	type pendingPool struct {
		market   Market
		position positionOutput
	}
	pending := make([]pendingPool, 0)
	calls := make([]evm.Call, 0)
	for _, market := range markets {
		for _, position := range positionsByMarket[market.ID] {
			data, err := packFactory("getPool", position.Token0, position.Token1, big.NewInt(int64(position.Fee)))
			if err != nil {
				return nil, err
			}
			pending = append(pending, pendingPool{market: market, position: position})
			calls = append(calls, evm.Call{
				Target:       common.HexToAddress(market.Factory),
				AllowFailure: false,
				CallData:     data,
			})
		}
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v3 owner getPool: got %d results, want %d", len(results), len(calls))
	}

	type pendingTickSpacing struct {
		pool     common.Address
		market   Market
		position positionOutput
	}
	tickPending := make([]pendingTickSpacing, 0, len(results))
	tickCalls := make([]evm.Call, 0, len(results))
	for i, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 owner getPool failed for market %s", pending[i].market.ID)
		}
		pool, err := unpackGetPool(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 owner getPool decode: %w", err)
		}
		if pool == (common.Address{}) {
			continue
		}
		data, err := packPool("tickSpacing")
		if err != nil {
			return nil, err
		}
		tickPending = append(tickPending, pendingTickSpacing{
			pool:     pool,
			market:   pending[i].market,
			position: pending[i].position,
		})
		tickCalls = append(tickCalls, evm.Call{
			Target:       pool,
			AllowFailure: false,
			CallData:     data,
		})
	}
	tickResults, err := aggregateInBatches(ctx, runner, tickCalls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(tickResults) != len(tickCalls) {
		return nil, fmt.Errorf("uniswap v3 owner tickSpacing: got %d results, want %d", len(tickResults), len(tickCalls))
	}

	out := make([]poolSeed, 0, len(tickPending))
	for i, result := range tickResults {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 owner pool %s tickSpacing failed", tickPending[i].pool.Hex())
		}
		tickSpacing, err := unpackTickSpacing(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 owner pool %s tickSpacing: %w", tickPending[i].pool.Hex(), err)
		}
		position := tickPending[i].position
		out = append(out, poolSeed{
			market:      tickPending[i].market,
			address:     tickPending[i].pool,
			token0:      position.Token0,
			token1:      position.Token1,
			fee:         position.Fee,
			tickSpacing: tickSpacing,
		})
	}
	return out, nil
}

func fetchOwnerPositionMetadata(ctx context.Context, runner MulticallRunner, markets []Market, owner common.Address) (map[string][]positionOutput, error) {
	balanceCall, err := packPositionManager("balanceOf", owner)
	if err != nil {
		return nil, err
	}
	calls := make([]evm.Call, 0, len(markets))
	for _, market := range markets {
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.PositionManager),
			AllowFailure: false,
			CallData:     balanceCall,
		})
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v3 owner balances: got %d results, want %d", len(results), len(calls))
	}

	type tokenIndexCall struct {
		market Market
	}
	tokenPending := make([]tokenIndexCall, 0)
	tokenCalls := make([]evm.Call, 0)
	for i, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 market %s position balanceOf failed", markets[i].ID)
		}
		balance, err := unpackPositionBalance(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 market %s position balanceOf: %w", markets[i].ID, err)
		}
		if !balance.IsUint64() {
			return nil, fmt.Errorf("uniswap v3 market %s position balance %s overflows uint64", markets[i].ID, balance.String())
		}
		for index := uint64(0); index < balance.Uint64(); index++ {
			data, err := packPositionManager("tokenOfOwnerByIndex", owner, new(big.Int).SetUint64(index))
			if err != nil {
				return nil, err
			}
			tokenPending = append(tokenPending, tokenIndexCall{market: markets[i]})
			tokenCalls = append(tokenCalls, evm.Call{
				Target:       common.HexToAddress(markets[i].PositionManager),
				AllowFailure: false,
				CallData:     data,
			})
		}
	}
	tokenResults, err := aggregateInBatches(ctx, runner, tokenCalls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(tokenResults) != len(tokenCalls) {
		return nil, fmt.Errorf("uniswap v3 owner tokenOfOwnerByIndex: got %d results, want %d", len(tokenResults), len(tokenCalls))
	}

	type positionCall struct {
		market  Market
		tokenID *big.Int
	}
	positionPending := make([]positionCall, 0, len(tokenResults))
	positionCalls := make([]evm.Call, 0, len(tokenResults))
	for i, result := range tokenResults {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 market %s tokenOfOwnerByIndex failed", tokenPending[i].market.ID)
		}
		tokenID, err := unpackTokenOfOwnerByIndex(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 market %s tokenOfOwnerByIndex: %w", tokenPending[i].market.ID, err)
		}
		data, err := packPositionManager("positions", tokenID)
		if err != nil {
			return nil, err
		}
		positionPending = append(positionPending, positionCall{market: tokenPending[i].market, tokenID: tokenID})
		positionCalls = append(positionCalls, evm.Call{
			Target:       common.HexToAddress(tokenPending[i].market.PositionManager),
			AllowFailure: false,
			CallData:     data,
		})
	}
	positionResults, err := aggregateInBatches(ctx, runner, positionCalls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(positionResults) != len(positionCalls) {
		return nil, fmt.Errorf("uniswap v3 owner positions: got %d results, want %d", len(positionResults), len(positionCalls))
	}

	out := make(map[string][]positionOutput, len(markets))
	for i, result := range positionResults {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 market %s position %s failed", positionPending[i].market.ID, positionPending[i].tokenID.String())
		}
		position, err := unpackPosition(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 market %s position %s: %w", positionPending[i].market.ID, positionPending[i].tokenID.String(), err)
		}
		out[positionPending[i].market.ID] = append(out[positionPending[i].market.ID], position)
	}
	return out, nil
}

func loadTokenMetadata(ctx context.Context, runner MulticallRunner, chainID int64, addresses map[common.Address]struct{}) (map[common.Address]core.Token, error) {
	sorted := make([]common.Address, 0, len(addresses))
	for address := range addresses {
		if address != (common.Address{}) {
			sorted = append(sorted, address)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Hex()) < strings.ToLower(sorted[j].Hex())
	})

	symbolCall, err := packERC20("symbol")
	if err != nil {
		return nil, err
	}
	decimalsCall, err := packERC20("decimals")
	if err != nil {
		return nil, err
	}
	calls := make([]evm.Call, 0, len(sorted)*2)
	for _, address := range sorted {
		calls = append(calls,
			evm.Call{Target: address, AllowFailure: true, CallData: symbolCall},
			evm.Call{Target: address, AllowFailure: true, CallData: decimalsCall},
		)
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v3 token metadata: got %d results, want %d", len(results), len(calls))
	}

	out := make(map[common.Address]core.Token, len(sorted))
	for i, address := range sorted {
		base := i * 2
		symbol := fallbackSymbol(address)
		if results[base].Success {
			if value, err := unpackERC20Symbol(results[base].ReturnData); err == nil && strings.TrimSpace(value) != "" {
				symbol = strings.TrimSpace(value)
			}
		}
		decimals := uint8(18)
		if results[base+1].Success {
			if value, err := unpackERC20Decimals(results[base+1].ReturnData); err == nil {
				decimals = value
			}
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

func buildPools(seeds []poolSeed, tokens map[common.Address]core.Token) ([]Pool, error) {
	out := make([]Pool, 0, len(seeds))
	for _, seed := range seeds {
		token0, ok := tokens[seed.token0]
		if !ok {
			return nil, fmt.Errorf("uniswap v3 pool %s: token0 metadata missing for %s", seed.address.Hex(), seed.token0.Hex())
		}
		token1, ok := tokens[seed.token1]
		if !ok {
			return nil, fmt.Errorf("uniswap v3 pool %s: token1 metadata missing for %s", seed.address.Hex(), seed.token1.Hex())
		}
		poolAddress := seed.address.Hex()
		out = append(out, Pool{
			MarketID:     seed.market.ID,
			Address:      poolAddress,
			Token0:       token0,
			Token1:       token1,
			Fee:          seed.fee,
			FeeFormatted: feeFormatted(seed.fee),
			TickSpacing:  seed.tickSpacing,
			CreatedBlock: seed.createdBlock,
			DebankPoolID: strings.ToLower(poolAddress),
		})
	}
	return out, nil
}

func tokenAddressesFromPoolSeeds(seeds []poolSeed) map[common.Address]struct{} {
	out := make(map[common.Address]struct{}, len(seeds)*2)
	for _, seed := range seeds {
		if seed.token0 != (common.Address{}) {
			out[seed.token0] = struct{}{}
		}
		if seed.token1 != (common.Address{}) {
			out[seed.token1] = struct{}{}
		}
	}
	return out
}

func uniquePoolSeeds(seeds []poolSeed) []poolSeed {
	out := make([]poolSeed, 0, len(seeds))
	seen := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		key := strings.ToLower(seed.address.Hex())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, seed)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].createdBlock == out[j].createdBlock {
			return strings.ToLower(out[i].address.Hex()) < strings.ToLower(out[j].address.Hex())
		}
		return out[i].createdBlock < out[j].createdBlock
	})
	return out
}

func discoveryOptionsFromEnv(owner string) (discoveryOptions, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DPR_UNISWAP_V3_DISCOVERY_MODE")))
	if mode == "" {
		mode = discoveryModeFull
	}
	options := discoveryOptions{mode: mode}
	if strings.TrimSpace(owner) != "" {
		if !common.IsHexAddress(owner) {
			return discoveryOptions{}, fmt.Errorf("sync owner %q is not a valid EVM address", owner)
		}
		options.mode = discoveryModeUser
		options.owner = common.HexToAddress(owner)
		options.hasOwner = true
	}
	switch options.mode {
	case discoveryModeFull, discoveryModeLimited, discoveryModeUser:
	default:
		return discoveryOptions{}, fmt.Errorf("unknown DPR_UNISWAP_V3_DISCOVERY_MODE %q", mode)
	}
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V3_MAX_POOLS")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return discoveryOptions{}, fmt.Errorf("parse DPR_UNISWAP_V3_MAX_POOLS: %w", err)
		}
		if value > 0 {
			options.hasMaxPools = true
			options.maxPools = value
			if options.mode == discoveryModeFull {
				options.mode = discoveryModeLimited
			}
		}
	}
	return options, nil
}

func buildDiscoverySteps(markets, seeds, tokens, pools int, options discoveryOptions) []discoveryStep {
	poolOperation := "Factory.PoolCreated logs"
	poolNotes := "mode=" + options.mode
	if options.mode == discoveryModeUser {
		poolOperation = "NonfungiblePositionManager.positions + Factory.getPool"
		poolNotes += "; only pools referenced by owner-held position NFTs"
	}
	if options.hasMaxPools {
		poolNotes += fmt.Sprintf("; maxPools=%d", options.maxPools)
	}
	return []discoveryStep{
		{
			Step:      "load markets",
			Operation: "static market config",
			Markets:   markets,
			Items:     markets,
			Status:    "ok",
		},
		{
			Step:      "discover pools",
			Operation: poolOperation,
			Markets:   markets,
			Items:     seeds,
			Status:    "ok",
			Notes:     poolNotes,
		},
		{
			Step:         "load token metadata",
			Operation:    "ERC20.symbol + decimals",
			Calls:        tokens * 2,
			Items:        tokens,
			Status:       "ok",
			Notes:        "unique pool tokens",
			AllowFailure: true,
		},
		{
			Step:      "write metadata",
			Operation: "SQLite Set(markets, pools)",
			Items:     markets + pools,
			Status:    "ok",
		},
	}
}

func aggregateInBatches(ctx context.Context, runner MulticallRunner, calls []evm.Call, batchSize int) ([]evm.Result, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	if batchSize <= 0 {
		batchSize = len(calls)
	}
	out := make([]evm.Result, 0, len(calls))
	for start := 0; start < len(calls); start += batchSize {
		end := start + batchSize
		if end > len(calls) {
			end = len(calls)
		}
		results, err := runner.Aggregate3(ctx, calls[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, results...)
	}
	return out, nil
}

func feeFormatted(fee uint32) string {
	// Fee is denominated in hundredths of a bip, so 1_000_000 = 100%.
	value := new(big.Rat).SetFrac(new(big.Int).SetUint64(uint64(fee)), big.NewInt(10_000))
	return value.FloatString(4) + "%"
}

func fallbackSymbol(address common.Address) string {
	hex := strings.TrimPrefix(address.Hex(), "0x")
	if len(hex) > 6 {
		hex = hex[:6]
	}
	return "TOKEN-" + strings.ToUpper(hex)
}

func syncSource(options discoveryOptions) string {
	if options.mode == discoveryModeUser {
		return "uniswap-v3-position-manager-user-scoped"
	}
	return "uniswap-v3-factory-poolcreated-logs"
}
