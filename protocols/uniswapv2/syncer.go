package uniswapv2

import (
	"context"
	"fmt"
	"math"
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

type pairSeed struct {
	market  Market
	address common.Address
	index   uint64
}

type pairBasics struct {
	seed     pairSeed
	token0   common.Address
	token1   common.Address
	symbol   string
	decimals uint8
}

type farmingBasics struct {
	config       FarmingPoolConfig
	stakingToken common.Address
	rewardsToken common.Address
	periodFinish *big.Int
}

type tokenMetadata struct {
	token core.Token
}

type discoveryOptions struct {
	mode        string
	hasMaxPairs bool
	maxPairs    uint64
	seedPairs   []common.Address
	owner       common.Address
	hasOwner    bool
}

const (
	discoveryModeUser    = "user"
	discoveryModeSeed    = "seed"
	discoveryModeLimited = "limited"
	discoveryModeFull    = "full"
)

var transferTopic = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

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
	farmingConfigs := farmingPoolConfigs(req.Chain.ID)
	options, err := discoveryOptionsFromEnv(req.Owner)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	seeds := make([]pairSeed, 0)
	if options.mode == discoveryModeUser {
		client, err := evm.NewClientFromEnv(req.Chain)
		if err != nil {
			return adapter.SyncResult{}, err
		}
		userSeeds, err := discoverOwnerPairSeeds(ctx, client, runner, markets, options.owner)
		if err != nil {
			return adapter.SyncResult{}, err
		}
		seeds = append(seeds, userSeeds...)
		farmingConfigs, err = activeFarmingPoolConfigs(ctx, runner, options.owner, farmingConfigs)
		if err != nil {
			return adapter.SyncResult{}, err
		}
	} else {
		for _, market := range markets {
			marketSeeds, err := discoverPairSeeds(ctx, runner, market, options)
			if err != nil {
				return adapter.SyncResult{}, err
			}
			seeds = append(seeds, marketSeeds...)
		}
	}
	seeds = ensureFarmingPairSeeds(seeds, markets, farmingConfigs)
	seeds = ensureSeedPairSeeds(seeds, markets, options.seedPairs)

	basics, err := loadPairBasics(ctx, runner, seeds)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	farming, err := loadFarmingBasics(ctx, runner, farmingConfigs)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	tokenAddresses := tokenAddressesFromPairBasics(basics)
	for _, item := range farming {
		if item.rewardsToken != (common.Address{}) {
			tokenAddresses[item.rewardsToken] = struct{}{}
		}
	}
	tokens, err := loadTokenMetadata(ctx, runner, req.Chain.ID, tokenAddresses)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	pairs, err := buildPairs(req.Chain.ID, basics, tokens)
	if err != nil {
		return adapter.SyncResult{}, err
	}
	farmingPools, err := buildFarmingPools(farming, pairs, tokens)
	if err != nil {
		return adapter.SyncResult{}, err
	}

	sort.Slice(markets, func(i, j int) bool { return markets[i].ID < markets[j].ID })
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].MarketID == pairs[j].MarketID {
			return pairs[i].Index < pairs[j].Index
		}
		return pairs[i].MarketID < pairs[j].MarketID
	})
	sort.Slice(farmingPools, func(i, j int) bool { return farmingPools[i].ID < farmingPools[j].ID })

	metadata := core.MetadataInfo{
		ChainID:     req.Chain.ID,
		Protocol:    ProtocolID,
		Namespace:   pairsNamespace,
		Version:     metadataVersion,
		BlockNumber: 0,
		UpdatedAt:   time.Now().UTC(),
		Source:      "uniswap-v2-factory-and-onchain-multicall",
	}
	if req.Store != nil {
		for _, item := range []struct {
			namespace string
			value     any
		}{
			{marketsNamespace, markets},
			{pairsNamespace, pairs},
			{farmingPoolsNamespace, farmingPools},
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
		Items:    len(markets) + len(pairs) + len(farmingPools),
		Details: map[string]any{
			"discovery": buildDiscoverySteps(len(markets), len(seeds), len(basics), len(tokens), len(farmingConfigs), len(farmingPools), options),
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
			Router:           common.HexToAddress(config.Router).Hex(),
			DebankProtocolID: config.DebankProtocolID,
			StartBlock:       config.StartBlock,
		})
	}
	return out
}

func discoverPairSeeds(ctx context.Context, runner MulticallRunner, market Market, options discoveryOptions) ([]pairSeed, error) {
	if options.mode != discoveryModeFull && options.mode != discoveryModeLimited {
		return nil, nil
	}

	data, err := packFactory("allPairsLength")
	if err != nil {
		return nil, err
	}
	results, err := runner.Aggregate3(ctx, []evm.Call{{
		Target:       common.HexToAddress(market.Factory),
		AllowFailure: false,
		CallData:     data,
	}})
	if err != nil {
		return nil, err
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("uniswap v2 market %s: got %d allPairsLength results, want 1", market.ID, len(results))
	}
	if !results[0].Success {
		return nil, fmt.Errorf("uniswap v2 market %s: allPairsLength failed", market.ID)
	}
	length, err := unpackAllPairsLength(results[0].ReturnData)
	if err != nil {
		return nil, fmt.Errorf("uniswap v2 market %s: %w", market.ID, err)
	}
	if !length.IsUint64() {
		return nil, fmt.Errorf("uniswap v2 market %s: allPairsLength %s overflows uint64", market.ID, length.String())
	}

	count := length.Uint64()
	if options.hasMaxPairs && options.maxPairs < count {
		count = options.maxPairs
	}
	calls := make([]evm.Call, 0, count)
	for i := uint64(0); i < count; i++ {
		data, err := packFactory("allPairs", new(big.Int).SetUint64(i))
		if err != nil {
			return nil, err
		}
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.Factory),
			AllowFailure: false,
			CallData:     data,
		})
	}
	results, err = aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 market %s: got %d allPairs results, want %d", market.ID, len(results), len(calls))
	}

	out := make([]pairSeed, 0, len(results))
	for i, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v2 market %s: allPairs(%d) failed", market.ID, i)
		}
		address, err := unpackAllPairs(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 market %s allPairs(%d): %w", market.ID, i, err)
		}
		if address == (common.Address{}) {
			continue
		}
		out = append(out, pairSeed{market: market, address: address, index: uint64(i)})
	}
	return out, nil
}

func discoverOwnerPairSeeds(ctx context.Context, client *evm.Client, runner MulticallRunner, markets []Market, owner common.Address) ([]pairSeed, error) {
	if owner == (common.Address{}) {
		return nil, nil
	}
	candidates, err := discoverOwnerTransferContracts(ctx, client, markets, owner)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return verifyPairCandidates(ctx, runner, markets, candidates)
}

func discoverOwnerTransferContracts(ctx context.Context, client *evm.Client, markets []Market, owner common.Address) ([]common.Address, error) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_USER_DISCOVERY_SOURCE"))) != "logs" {
		addresses, err := discoverOwnerTransferContractsFromAssetTransfers(ctx, client, owner)
		if err == nil {
			return addresses, nil
		}
		if strings.EqualFold(strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_USER_DISCOVERY_SOURCE")), "alchemy") {
			return nil, err
		}
	}

	latest, err := client.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("read latest block for user pair discovery: %w", err)
	}

	startBlock := uint64(0)
	for i, market := range markets {
		if i == 0 || market.StartBlock < startBlock {
			startBlock = market.StartBlock
		}
	}
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_USER_LOG_FROM_BLOCK")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DPR_UNISWAP_V2_USER_LOG_FROM_BLOCK: %w", err)
		}
		startBlock = value
	}
	if startBlock > latest {
		return nil, nil
	}
	chunkSize := uint64(100_000)
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_USER_LOG_BLOCK_CHUNK")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DPR_UNISWAP_V2_USER_LOG_BLOCK_CHUNK: %w", err)
		}
		if value > 0 {
			chunkSize = value
		}
	}

	ownerTopic := common.BytesToHash(owner.Bytes()).Hex()
	seen := make(map[string]common.Address)
	for fromBlock := startBlock; fromBlock <= latest; {
		toBlock := fromBlock + chunkSize - 1
		if toBlock > latest || toBlock < fromBlock {
			toBlock = latest
		}
		for _, topics := range [][]any{
			{transferTopic.Hex(), ownerTopic, nil},
			{transferTopic.Hex(), nil, ownerTopic},
		} {
			logs, err := client.FilterLogs(ctx, evm.LogQuery{
				FromBlock: fromBlock,
				ToBlock:   toBlock,
				Topics:    topics,
			})
			if err != nil {
				return nil, fmt.Errorf("discover owner transfer contracts blocks %d-%d: %w; user-scoped sync requires an RPC/indexer that supports topic-only eth_getLogs over historical ranges, or run default full sync", fromBlock, toBlock, err)
			}
			for _, item := range logs {
				if item.Address == (common.Address{}) {
					continue
				}
				seen[strings.ToLower(item.Address.Hex())] = item.Address
			}
		}
		if toBlock == latest {
			break
		}
		fromBlock = toBlock + 1
	}

	out := make([]common.Address, 0, len(seen))
	for _, address := range seen {
		out = append(out, address)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Hex()) < strings.ToLower(out[j].Hex())
	})
	return out, nil
}

func discoverOwnerTransferContractsFromAssetTransfers(ctx context.Context, client *evm.Client, owner common.Address) ([]common.Address, error) {
	maxPages := uint64(5)
	if override := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_USER_TRANSFER_PAGES")); override != "" {
		value, err := strconv.ParseUint(override, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DPR_UNISWAP_V2_USER_TRANSFER_PAGES: %w", err)
		}
		if value > 0 {
			maxPages = value
		}
	}

	seen := make(map[string]common.Address)
	for _, direction := range []string{"from", "to"} {
		pageKey := ""
		for page := uint64(0); page < maxPages; page++ {
			query := evm.AssetTransferQuery{
				PageKey:  pageKey,
				MaxCount: 1000,
			}
			if direction == "from" {
				query.FromAddress = &owner
			} else {
				query.ToAddress = &owner
			}
			result, err := client.AssetTransfers(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("discover owner transfer contracts with alchemy_getAssetTransfers direction=%s page=%d: %w", direction, page+1, err)
			}
			for _, transfer := range result.Transfers {
				address := transfer.RawContractAddress
				if address == (common.Address{}) {
					continue
				}
				seen[strings.ToLower(address.Hex())] = address
			}
			pageKey = result.PageKey
			if strings.TrimSpace(pageKey) == "" {
				break
			}
		}
	}

	out := make([]common.Address, 0, len(seen))
	for _, address := range seen {
		out = append(out, address)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Hex()) < strings.ToLower(out[j].Hex())
	})
	return out, nil
}

func verifyPairCandidates(ctx context.Context, runner MulticallRunner, markets []Market, candidates []common.Address) ([]pairSeed, error) {
	if len(candidates) == 0 || len(markets) == 0 {
		return nil, nil
	}

	methods := []string{"token0", "token1"}
	calls := make([]evm.Call, 0, len(candidates)*len(methods))
	for _, candidate := range candidates {
		for _, method := range methods {
			data, err := packPair(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, evm.Call{
				Target:       candidate,
				AllowFailure: true,
				CallData:     data,
			})
		}
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 pair candidate token reads: got %d multicall results, want %d", len(results), len(calls))
	}

	type pairCandidate struct {
		address common.Address
		token0  common.Address
		token1  common.Address
	}
	valid := make([]pairCandidate, 0, len(candidates))
	for i, candidate := range candidates {
		base := i * len(methods)
		if !results[base].Success || !results[base+1].Success {
			continue
		}
		token0, err := unpackToken0(results[base].ReturnData)
		if err != nil {
			continue
		}
		token1, err := unpackToken1(results[base+1].ReturnData)
		if err != nil {
			continue
		}
		if token0 == (common.Address{}) || token1 == (common.Address{}) {
			continue
		}
		valid = append(valid, pairCandidate{
			address: candidate,
			token0:  token0,
			token1:  token1,
		})
	}
	if len(valid) == 0 {
		return nil, nil
	}

	calls = make([]evm.Call, 0, len(valid)*len(markets))
	type pendingValidation struct {
		candidate pairCandidate
		market    Market
	}
	pending := make([]pendingValidation, 0, len(valid)*len(markets))
	for _, candidate := range valid {
		for _, market := range markets {
			data, err := packFactory("getPair", candidate.token0, candidate.token1)
			if err != nil {
				return nil, err
			}
			pending = append(pending, pendingValidation{candidate: candidate, market: market})
			calls = append(calls, evm.Call{
				Target:       common.HexToAddress(market.Factory),
				AllowFailure: true,
				CallData:     data,
			})
		}
	}
	results, err = aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 pair candidate factory validation: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]pairSeed, 0, len(valid))
	seen := make(map[string]struct{}, len(valid))
	for i, result := range results {
		if !result.Success {
			continue
		}
		pair, err := unpackGetPair(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 candidate %s getPair validation: %w", pending[i].candidate.address.Hex(), err)
		}
		candidate := pending[i].candidate.address
		if !sameAddress(pair, candidate) {
			continue
		}
		key := strings.ToLower(candidate.Hex())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pairSeed{
			market:  pending[i].market,
			address: candidate,
			index:   math.MaxUint64,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].address.Hex()) < strings.ToLower(out[j].address.Hex())
	})
	return out, nil
}

func activeFarmingPoolConfigs(ctx context.Context, runner MulticallRunner, owner common.Address, configs []FarmingPoolConfig) ([]FarmingPoolConfig, error) {
	if len(configs) == 0 || owner == (common.Address{}) {
		return nil, nil
	}
	balanceCall, err := packStaking("balanceOf", owner)
	if err != nil {
		return nil, err
	}
	earnedCall, err := packStaking("earned", owner)
	if err != nil {
		return nil, err
	}
	calls := make([]evm.Call, 0, len(configs)*2)
	for _, config := range configs {
		target := common.HexToAddress(config.StakingContract)
		calls = append(calls,
			evm.Call{Target: target, AllowFailure: false, CallData: balanceCall},
			evm.Call{Target: target, AllowFailure: true, CallData: earnedCall},
		)
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 active farming discovery: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]FarmingPoolConfig, 0, len(configs))
	for i, config := range configs {
		base := i * 2
		if !results[base].Success {
			return nil, fmt.Errorf("uniswap v2 farming %s: balanceOf failed", config.ID)
		}
		staked, err := unpackStakedBalance(results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 farming %s balance: %w", config.ID, err)
		}
		earned := big.NewInt(0)
		if results[base+1].Success {
			earned, err = unpackEarned(results[base+1].ReturnData)
			if err != nil {
				return nil, fmt.Errorf("uniswap v2 farming %s earned: %w", config.ID, err)
			}
		}
		if zeroIfNil(staked).Sign() > 0 || zeroIfNil(earned).Sign() > 0 {
			out = append(out, config)
		}
	}
	return out, nil
}

func ensureSeedPairSeeds(seeds []pairSeed, markets []Market, addresses []common.Address) []pairSeed {
	if len(addresses) == 0 || len(markets) == 0 {
		return seeds
	}
	seen := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		seen[strings.ToLower(seed.address.Hex())] = struct{}{}
	}
	market := markets[0]
	for _, address := range addresses {
		if address == (common.Address{}) {
			continue
		}
		key := strings.ToLower(address.Hex())
		if _, ok := seen[key]; ok {
			continue
		}
		seeds = append(seeds, pairSeed{
			market:  market,
			address: address,
			index:   math.MaxUint64,
		})
		seen[key] = struct{}{}
	}
	return seeds
}

func ensureFarmingPairSeeds(seeds []pairSeed, markets []Market, farmingConfigs []FarmingPoolConfig) []pairSeed {
	if len(farmingConfigs) == 0 {
		return seeds
	}
	marketsByID := make(map[string]Market, len(markets))
	for _, market := range markets {
		marketsByID[market.ID] = market
	}
	seen := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		seen[strings.ToLower(seed.address.Hex())] = struct{}{}
	}
	for _, config := range farmingConfigs {
		address := common.HexToAddress(config.Pair)
		key := strings.ToLower(address.Hex())
		if _, ok := seen[key]; ok {
			continue
		}
		market, ok := marketsByID[config.MarketID]
		if !ok {
			continue
		}
		seeds = append(seeds, pairSeed{
			market:  market,
			address: address,
			index:   math.MaxUint64,
		})
		seen[key] = struct{}{}
	}
	return seeds
}

func loadPairBasics(ctx context.Context, runner MulticallRunner, seeds []pairSeed) ([]pairBasics, error) {
	methods := []string{"token0", "token1", "symbol", "decimals"}
	calls := make([]evm.Call, 0, len(seeds)*len(methods))
	for _, seed := range seeds {
		target := seed.address
		for _, method := range methods {
			data, err := packPair(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, evm.Call{
				Target:       target,
				AllowFailure: method == "symbol" || method == "decimals",
				CallData:     data,
			})
		}
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 pair basics: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]pairBasics, 0, len(seeds))
	for i, seed := range seeds {
		base := i * len(methods)
		if !results[base].Success {
			return nil, fmt.Errorf("uniswap v2 pair %s: token0 failed", seed.address.Hex())
		}
		token0, err := unpackToken0(results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s: %w", seed.address.Hex(), err)
		}
		if !results[base+1].Success {
			return nil, fmt.Errorf("uniswap v2 pair %s: token1 failed", seed.address.Hex())
		}
		token1, err := unpackToken1(results[base+1].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s: %w", seed.address.Hex(), err)
		}
		symbol := "UNI-V2"
		if results[base+2].Success {
			if value, err := unpackPairSymbol(results[base+2].ReturnData); err == nil && strings.TrimSpace(value) != "" {
				symbol = strings.TrimSpace(value)
			}
		}
		decimals := uint8(18)
		if results[base+3].Success {
			if value, err := unpackPairDecimals(results[base+3].ReturnData); err == nil {
				decimals = value
			}
		}
		out = append(out, pairBasics{
			seed:     seed,
			token0:   token0,
			token1:   token1,
			symbol:   symbol,
			decimals: decimals,
		})
	}
	return out, nil
}

func loadFarmingBasics(ctx context.Context, runner MulticallRunner, configs []FarmingPoolConfig) ([]farmingBasics, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	methods := []string{"stakingToken", "rewardsToken", "periodFinish"}
	calls := make([]evm.Call, 0, len(configs)*len(methods))
	for _, config := range configs {
		target := common.HexToAddress(config.StakingContract)
		for _, method := range methods {
			data, err := packStaking(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, evm.Call{
				Target:       target,
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
		return nil, fmt.Errorf("uniswap v2 farming basics: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]farmingBasics, 0, len(configs))
	for i, config := range configs {
		base := i * len(methods)
		for j, method := range methods {
			if !results[base+j].Success {
				return nil, fmt.Errorf("uniswap v2 farming %s: %s failed", config.ID, method)
			}
		}
		stakingToken, err := unpackStakingToken(results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 farming %s: %w", config.ID, err)
		}
		rewardsToken, err := unpackRewardsToken(results[base+1].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 farming %s: %w", config.ID, err)
		}
		periodFinish, err := unpackPeriodFinish(results[base+2].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 farming %s: %w", config.ID, err)
		}
		out = append(out, farmingBasics{
			config:       config,
			stakingToken: stakingToken,
			rewardsToken: rewardsToken,
			periodFinish: zeroIfNil(periodFinish),
		})
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

	calls := make([]evm.Call, 0, len(sorted)*2)
	for _, address := range sorted {
		symbolCall, err := packERC20("symbol")
		if err != nil {
			return nil, err
		}
		decimalsCall, err := packERC20("decimals")
		if err != nil {
			return nil, err
		}
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
		return nil, fmt.Errorf("uniswap v2 token metadata: got %d multicall results, want %d", len(results), len(calls))
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

func buildPairs(chainID int64, basics []pairBasics, tokens map[common.Address]core.Token) ([]Pair, error) {
	pairs := make([]Pair, 0, len(basics))
	for _, basic := range basics {
		token0, ok := tokens[basic.token0]
		if !ok {
			return nil, fmt.Errorf("uniswap v2 pair %s: token0 metadata missing", basic.seed.address.Hex())
		}
		token1, ok := tokens[basic.token1]
		if !ok {
			return nil, fmt.Errorf("uniswap v2 pair %s: token1 metadata missing", basic.seed.address.Hex())
		}
		pairAddress := basic.seed.address.Hex()
		pairs = append(pairs, Pair{
			MarketID: basic.seed.market.ID,
			Address:  pairAddress,
			Index:    basic.seed.index,
			PairToken: core.Token{
				ChainID:  chainID,
				Address:  pairAddress,
				Symbol:   basic.symbol,
				Decimals: basic.decimals,
			},
			Token0:       token0,
			Token1:       token1,
			DebankPoolID: strings.ToLower(pairAddress),
		})
	}
	return pairs, nil
}

func buildFarmingPools(basics []farmingBasics, pairs []Pair, tokens map[common.Address]core.Token) ([]FarmingPool, error) {
	pairsByAddress := make(map[string]Pair, len(pairs))
	for _, pair := range pairs {
		pairsByAddress[strings.ToLower(pair.Address)] = pair
	}

	out := make([]FarmingPool, 0, len(basics))
	for _, basic := range basics {
		pairAddress := basic.stakingToken
		if pairAddress == (common.Address{}) {
			pairAddress = common.HexToAddress(basic.config.Pair)
		}
		pair, ok := pairsByAddress[strings.ToLower(pairAddress.Hex())]
		if !ok {
			pair, ok = pairsByAddress[strings.ToLower(common.HexToAddress(basic.config.Pair).Hex())]
		}
		if !ok {
			return nil, fmt.Errorf("uniswap v2 farming %s: pair metadata missing", basic.config.ID)
		}
		rewardToken, ok := tokens[basic.rewardsToken]
		if !ok {
			return nil, fmt.Errorf("uniswap v2 farming %s: reward token metadata missing", basic.config.ID)
		}
		stakingContract := common.HexToAddress(basic.config.StakingContract).Hex()
		out = append(out, FarmingPool{
			ID:              basic.config.ID,
			MarketID:        basic.config.MarketID,
			Name:            basic.config.Name,
			DisplayName:     basic.config.DisplayName,
			StakingContract: stakingContract,
			Pair:            pair.Address,
			PairToken:       pair.PairToken,
			Token0:          pair.Token0,
			Token1:          pair.Token1,
			RewardToken:     rewardToken,
			PeriodFinish:    bigString(basic.periodFinish),
			DebankPoolID:    strings.ToLower(stakingContract),
		})
	}
	return out, nil
}

func tokenAddressesFromPairBasics(basics []pairBasics) map[common.Address]struct{} {
	out := make(map[common.Address]struct{}, len(basics)*2)
	for _, basic := range basics {
		out[basic.token0] = struct{}{}
		out[basic.token1] = struct{}{}
	}
	return out
}

func aggregateInBatches(ctx context.Context, runner MulticallRunner, calls []evm.Call, batchSize int) ([]evm.Result, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	if batchSize <= 0 {
		batchSize = defaultMulticallBatchSz
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
		if len(results) != end-start {
			return nil, fmt.Errorf("multicall batch starting at %d returned %d results, want %d", start, len(results), end-start)
		}
		out = append(out, results...)
	}
	return out, nil
}

func buildDiscoverySteps(markets int, pairSeeds int, pairBasics int, tokens int, farmingConfigs int, farmingPools int, options discoveryOptions) []discoveryStep {
	pairNotes := "full factory pair list plus configured seed/farming pairs"
	pairOperation := "Factory.allPairsLength + allPairs"
	switch options.mode {
	case discoveryModeUser:
		pairNotes = "owner ERC20 transfers validated through Factory.getPair"
		pairOperation = "alchemy_getAssetTransfers or eth_getLogs + Pair.token0/token1 + Factory.getPair"
	case discoveryModeSeed:
		pairNotes = "seed and configured farming pairs only"
		pairOperation = "configured pair addresses"
	case discoveryModeFull:
		pairNotes = "full factory pair list plus configured seed/farming pairs"
		pairOperation = "Factory.allPairsLength + allPairs"
	case discoveryModeLimited:
		pairNotes = fmt.Sprintf("factory pair list limited to %d pairs plus configured seed/farming pairs", options.maxPairs)
		pairOperation = "Factory.allPairsLength + allPairs"
	}
	if len(options.seedPairs) > 0 {
		pairNotes += fmt.Sprintf("; %d seed pairs from DPR_UNISWAP_V2_SEED_PAIRS", len(options.seedPairs))
	}
	return []discoveryStep{
		{
			Step:      "market configs",
			Operation: "local uniswap v2 deployment config",
			Markets:   markets,
			Items:     markets,
			Status:    "ok",
			Notes:     "factory and router configs for selected chain",
		},
		{
			Step:      "discover pairs",
			Operation: pairOperation,
			Markets:   markets,
			Items:     pairSeeds,
			Status:    "ok",
			Notes:     fmt.Sprintf("mode=%s; %s", options.mode, pairNotes),
		},
		{
			Step:      "load pair metadata",
			Operation: "Pair.token0 + token1 + symbol + decimals",
			Items:     pairBasics,
			Status:    "ok",
			Notes:     "pair token metadata uses UNI-V2 fallback when needed",
		},
		{
			Step:         "load token metadata",
			Operation:    "ERC20.symbol + decimals",
			Items:        tokens,
			Status:       "ok",
			Notes:        "supports string and bytes32 symbol fallbacks",
			AllowFailure: true,
		},
		{
			Step:      "discover farming pools",
			Operation: "StakingRewards.stakingToken + rewardsToken + periodFinish",
			Items:     farmingPools,
			Status:    "ok",
			Notes:     fmt.Sprintf("%d configured pools for selected chain", farmingConfigs),
		},
		{
			Step:      "cache write",
			Operation: "SQLite Set(markets, pairs, farming-pools)",
			Items:     markets + pairBasics + farmingPools,
			Status:    "ok",
			Notes:     "fetcher requires all namespaces to exist",
		},
	}
}

func fallbackSymbol(address common.Address) string {
	hex := strings.TrimPrefix(address.Hex(), "0x")
	if len(hex) > 8 {
		hex = hex[:8]
	}
	return "UNKNOWN-" + strings.ToUpper(hex)
}

func discoveryOptionsFromEnv(owner string) (discoveryOptions, error) {
	options := discoveryOptions{mode: discoveryModeFull}
	modeSet := false
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_DISCOVERY_MODE")))
	if mode != "" {
		modeSet = true
		switch mode {
		case discoveryModeSeed, discoveryModeLimited, discoveryModeFull:
			options.mode = mode
		default:
			return discoveryOptions{}, fmt.Errorf("DPR_UNISWAP_V2_DISCOVERY_MODE must be seed, limited, or full, got %q", mode)
		}
	}

	owner = strings.TrimSpace(owner)
	if owner != "" {
		if !common.IsHexAddress(owner) {
			return discoveryOptions{}, fmt.Errorf("sync owner %q is not a valid EVM address", owner)
		}
		options.mode = discoveryModeUser
		options.owner = common.HexToAddress(owner)
		options.hasOwner = true
	}

	maxPairs := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_MAX_PAIRS"))
	if maxPairs != "" {
		value, err := strconv.ParseUint(maxPairs, 10, 64)
		if err != nil {
			return discoveryOptions{}, fmt.Errorf("parse DPR_UNISWAP_V2_MAX_PAIRS: %w", err)
		}
		options.hasMaxPairs = true
		options.maxPairs = value
		if !modeSet && !options.hasOwner {
			options.mode = discoveryModeLimited
		}
	}
	if options.mode == discoveryModeFull {
		options.hasMaxPairs = false
		options.maxPairs = 0
	}
	if options.mode == discoveryModeLimited && !options.hasMaxPairs {
		return discoveryOptions{}, fmt.Errorf("DPR_UNISWAP_V2_DISCOVERY_MODE=limited requires DPR_UNISWAP_V2_MAX_PAIRS")
	}
	if options.mode == discoveryModeSeed {
		options.hasMaxPairs = true
		options.maxPairs = 0
	}

	seedPairs := strings.TrimSpace(os.Getenv("DPR_UNISWAP_V2_SEED_PAIRS"))
	if seedPairs != "" {
		for _, part := range strings.Split(seedPairs, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !common.IsHexAddress(part) {
				return discoveryOptions{}, fmt.Errorf("DPR_UNISWAP_V2_SEED_PAIRS contains invalid address %q", part)
			}
			options.seedPairs = append(options.seedPairs, common.HexToAddress(part))
		}
	}
	return options, nil
}

func sameAddress(a common.Address, b common.Address) bool {
	return strings.EqualFold(a.Hex(), b.Hex())
}
