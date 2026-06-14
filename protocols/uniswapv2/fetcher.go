package uniswapv2

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
	"github.com/yulai-123/defi-position-reader/pkg/evm"
)

type Fetcher struct {
	multicallFactory MulticallFactory
}

const defaultMetadataMaxAge = 24 * time.Hour

var shareScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

type activePairBalance struct {
	pair    Pair
	balance *big.Int
}

type pairRuntimeState struct {
	reserves             reservesOutput
	totalSupply          *big.Int
	kLast                *big.Int
	token0Balance        *big.Int
	token1Balance        *big.Int
	feeTo                common.Address
	feeOn                bool
	protocolLiquidity    *big.Int
	effectiveTotalSupply *big.Int
}

type activeFarmingPool struct {
	pool   FarmingPool
	staked *big.Int
	earned *big.Int
}

func (f Fetcher) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	factory, err := requireMulticallFactory(f.multicallFactory)
	if err != nil {
		return nil, err
	}
	owner := strings.ToLower(strings.TrimSpace(req.Owner))
	if !common.IsHexAddress(owner) {
		return nil, fmt.Errorf("uniswap v2 fetch: owner %q is not a valid EVM address", req.Owner)
	}

	markets, pairs, farmingPools, err := loadMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 || len(pairs) == 0 {
		return nil, nil
	}

	runner, err := factory(req.Chain)
	if err != nil {
		return nil, fmt.Errorf("create multicall runner: %w", err)
	}
	ownerAddress := common.HexToAddress(owner)

	positions := make([]core.Position, 0)
	direct, err := fetchDirectLiquidityPositions(ctx, runner, req.Chain.ID, owner, ownerAddress, markets, pairs)
	if err != nil {
		return nil, err
	}
	positions = append(positions, direct...)

	farming, err := fetchFarmingPositions(ctx, runner, req.Chain.ID, owner, ownerAddress, markets, farmingPools)
	if err != nil {
		return nil, err
	}
	positions = append(positions, farming...)
	return positions, nil
}

func loadMetadata(ctx context.Context, req adapter.FetchRequest) ([]Market, []Pair, []FarmingPool, error) {
	if req.Store == nil {
		return nil, nil, nil, fmt.Errorf("uniswap v2 fetch: cache store is nil")
	}

	var markets []Market
	marketsInfo, marketsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets)

	var pairs []Pair
	pairsInfo, pairsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: pairsNamespace,
	}, &pairs)

	var farmingPools []FarmingPool
	farmingInfo, farmingErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: farmingPoolsNamespace,
	}, &farmingPools)

	if errors.Is(marketsErr, cache.ErrNotFound) || errors.Is(pairsErr, cache.ErrNotFound) || errors.Is(farmingErr, cache.ErrNotFound) {
		return nil, nil, nil, syncRequiredError(req.Chain, "metadata is missing")
	}
	if marketsErr != nil {
		return nil, nil, nil, marketsErr
	}
	if pairsErr != nil {
		return nil, nil, nil, pairsErr
	}
	if farmingErr != nil {
		return nil, nil, nil, farmingErr
	}

	maxAge := req.MetadataMaxAge
	if maxAge == 0 {
		maxAge = defaultMetadataMaxAge
	}
	if maxAge > 0 {
		for _, info := range []core.MetadataInfo{marketsInfo, pairsInfo, farmingInfo} {
			if metadataExpired(info, maxAge) {
				return nil, nil, nil, syncRequiredError(req.Chain, fmt.Sprintf("metadata namespace %q is older than %s", info.Namespace, maxAge))
			}
		}
	}
	return markets, pairs, farmingPools, nil
}

func metadataExpired(info core.MetadataInfo, maxAge time.Duration) bool {
	if info.UpdatedAt.IsZero() {
		return true
	}
	return time.Since(info.UpdatedAt) > maxAge
}

func syncRequiredError(target core.Chain, reason string) error {
	chainName := target.Name
	if chainName == "" {
		chainName = fmt.Sprintf("%d", target.ID)
	}
	return fmt.Errorf("uniswap v2 fetch: %s; run `dpr sync-metadata -chain %s -protocol %s` before fetching positions", reason, chainName, ProtocolID)
}

func fetchDirectLiquidityPositions(
	ctx context.Context,
	runner MulticallRunner,
	chainID int64,
	owner string,
	ownerAddress common.Address,
	markets []Market,
	pairs []Pair,
) ([]core.Position, error) {
	active, err := fetchPairBalances(ctx, runner, ownerAddress, pairs)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, nil
	}
	activePairs := make([]Pair, 0, len(active))
	for _, item := range active {
		activePairs = append(activePairs, item.pair)
	}
	states, err := fetchPairRuntimeStates(ctx, runner, markets, activePairs)
	if err != nil {
		return nil, err
	}

	out := make([]core.Position, 0, len(active))
	for _, item := range active {
		state, ok := states[strings.ToLower(item.pair.Address)]
		if !ok {
			return nil, fmt.Errorf("uniswap v2 pair %s: runtime state missing", item.pair.Address)
		}
		position, ok := buildLiquidityPosition(chainID, owner, item.pair, item.balance, state, nil)
		if ok {
			out = append(out, position)
		}
	}
	return out, nil
}

func fetchPairBalances(ctx context.Context, runner MulticallRunner, owner common.Address, pairs []Pair) ([]activePairBalance, error) {
	data, err := packPair("balanceOf", owner)
	if err != nil {
		return nil, err
	}
	calls := make([]evm.Call, 0, len(pairs))
	for _, pair := range pairs {
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(pair.Address),
			AllowFailure: false,
			CallData:     data,
		})
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 direct balances: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]activePairBalance, 0)
	for i, result := range results {
		pair := pairs[i]
		if !result.Success {
			return nil, fmt.Errorf("uniswap v2 pair %s: balanceOf failed", pair.Address)
		}
		balance, err := unpackPairBalance(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s: %w", pair.Address, err)
		}
		balance = zeroIfNil(balance)
		if balance.Sign() == 0 {
			continue
		}
		out = append(out, activePairBalance{pair: pair, balance: balance})
	}
	return out, nil
}

func fetchFarmingPositions(
	ctx context.Context,
	runner MulticallRunner,
	chainID int64,
	owner string,
	ownerAddress common.Address,
	markets []Market,
	pools []FarmingPool,
) ([]core.Position, error) {
	active, err := fetchActiveFarmingPools(ctx, runner, ownerAddress, pools)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, nil
	}

	stakedPairs := make([]Pair, 0)
	for _, item := range active {
		if item.staked.Sign() == 0 {
			continue
		}
		stakedPairs = append(stakedPairs, pairFromFarmingPool(item.pool))
	}
	states, err := fetchPairRuntimeStates(ctx, runner, markets, stakedPairs)
	if err != nil {
		return nil, err
	}

	out := make([]core.Position, 0, len(active)*2)
	for _, item := range active {
		if item.staked.Sign() > 0 {
			pair := pairFromFarmingPool(item.pool)
			state, ok := states[strings.ToLower(pair.Address)]
			if !ok {
				return nil, fmt.Errorf("uniswap v2 farming %s: runtime state missing for pair %s", item.pool.ID, pair.Address)
			}
			position, ok := buildLiquidityPosition(chainID, owner, pair, item.staked, state, &item.pool)
			if ok {
				out = append(out, position)
			}
		}
		if item.earned.Sign() > 0 {
			out = append(out, buildFarmingRewardPosition(chainID, owner, item.pool, item.earned))
		}
	}
	return out, nil
}

func fetchActiveFarmingPools(ctx context.Context, runner MulticallRunner, owner common.Address, pools []FarmingPool) ([]activeFarmingPool, error) {
	if len(pools) == 0 {
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
	calls := make([]evm.Call, 0, len(pools)*2)
	for _, pool := range pools {
		target := common.HexToAddress(pool.StakingContract)
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
		return nil, fmt.Errorf("uniswap v2 farming balances: got %d multicall results, want %d", len(results), len(calls))
	}

	out := make([]activeFarmingPool, 0)
	for i, pool := range pools {
		base := i * 2
		if !results[base].Success {
			return nil, fmt.Errorf("uniswap v2 farming %s: balanceOf failed", pool.ID)
		}
		staked, err := unpackStakedBalance(results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 farming %s balance: %w", pool.ID, err)
		}
		earned := big.NewInt(0)
		if results[base+1].Success {
			earned, err = unpackEarned(results[base+1].ReturnData)
			if err != nil {
				return nil, fmt.Errorf("uniswap v2 farming %s earned: %w", pool.ID, err)
			}
		}
		staked = zeroIfNil(staked)
		earned = zeroIfNil(earned)
		if staked.Sign() == 0 && earned.Sign() == 0 {
			continue
		}
		out = append(out, activeFarmingPool{
			pool:   pool,
			staked: staked,
			earned: earned,
		})
	}
	return out, nil
}

func fetchPairRuntimeStates(ctx context.Context, runner MulticallRunner, markets []Market, pairs []Pair) (map[string]pairRuntimeState, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	pairs = uniquePairs(pairs)
	marketsByID := make(map[string]Market, len(markets))
	for _, market := range markets {
		marketsByID[market.ID] = market
	}

	activeMarkets := make([]Market, 0)
	seenMarkets := make(map[string]struct{})
	for _, pair := range pairs {
		if _, ok := seenMarkets[pair.MarketID]; ok {
			continue
		}
		market, ok := marketsByID[pair.MarketID]
		if !ok {
			return nil, fmt.Errorf("uniswap v2 pair %s: market %s metadata missing", pair.Address, pair.MarketID)
		}
		activeMarkets = append(activeMarkets, market)
		seenMarkets[pair.MarketID] = struct{}{}
	}

	calls := make([]evm.Call, 0, len(activeMarkets)+len(pairs)*5)
	for _, market := range activeMarkets {
		data, err := packFactory("feeTo")
		if err != nil {
			return nil, err
		}
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.Factory),
			AllowFailure: true,
			CallData:     data,
		})
	}
	for _, pair := range pairs {
		pairAddress := common.HexToAddress(pair.Address)
		getReserves, err := packPair("getReserves")
		if err != nil {
			return nil, err
		}
		totalSupply, err := packPair("totalSupply")
		if err != nil {
			return nil, err
		}
		kLast, err := packPair("kLast")
		if err != nil {
			return nil, err
		}
		token0Balance, err := packERC20("balanceOf", pairAddress)
		if err != nil {
			return nil, err
		}
		token1Balance, err := packERC20("balanceOf", pairAddress)
		if err != nil {
			return nil, err
		}
		calls = append(calls,
			evm.Call{Target: pairAddress, AllowFailure: false, CallData: getReserves},
			evm.Call{Target: pairAddress, AllowFailure: false, CallData: totalSupply},
			evm.Call{Target: pairAddress, AllowFailure: true, CallData: kLast},
			evm.Call{Target: common.HexToAddress(pair.Token0.Address), AllowFailure: false, CallData: token0Balance},
			evm.Call{Target: common.HexToAddress(pair.Token1.Address), AllowFailure: false, CallData: token1Balance},
		)
	}

	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v2 pair runtime: got %d multicall results, want %d", len(results), len(calls))
	}

	feeToByMarket := make(map[string]common.Address, len(activeMarkets))
	for i, market := range activeMarkets {
		if results[i].Success {
			feeTo, err := unpackFeeTo(results[i].ReturnData)
			if err != nil {
				return nil, fmt.Errorf("uniswap v2 market %s feeTo: %w", market.ID, err)
			}
			feeToByMarket[market.ID] = feeTo
		}
	}

	out := make(map[string]pairRuntimeState, len(pairs))
	index := len(activeMarkets)
	for _, pair := range pairs {
		for offset, label := range []string{"getReserves", "totalSupply", "token0.balanceOf", "token1.balanceOf"} {
			resultIndex := index + offset
			if offset >= 2 {
				resultIndex = index + offset + 1
			}
			if !results[resultIndex].Success {
				return nil, fmt.Errorf("uniswap v2 pair %s: %s failed", pair.Address, label)
			}
		}
		reserves, err := unpackReserves(results[index].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s: %w", pair.Address, err)
		}
		totalSupply, err := unpackTotalSupply(results[index+1].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s totalSupply: %w", pair.Address, err)
		}
		kLast := big.NewInt(0)
		if results[index+2].Success {
			kLast, err = unpackKLast(results[index+2].ReturnData)
			if err != nil {
				return nil, fmt.Errorf("uniswap v2 pair %s kLast: %w", pair.Address, err)
			}
		}
		token0Balance, err := unpackTokenBalance(results[index+3].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s token0 balance: %w", pair.Address, err)
		}
		token1Balance, err := unpackTokenBalance(results[index+4].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v2 pair %s token1 balance: %w", pair.Address, err)
		}
		index += 5

		totalSupply = zeroIfNil(totalSupply)
		kLast = zeroIfNil(kLast)
		token0Balance = zeroIfNil(token0Balance)
		token1Balance = zeroIfNil(token1Balance)
		feeTo := feeToByMarket[pair.MarketID]
		feeOn := feeTo != (common.Address{}) && kLast.Sign() > 0
		protocolLiquidity := pendingProtocolLiquidity(totalSupply, zeroIfNil(reserves.Reserve0), zeroIfNil(reserves.Reserve1), kLast, feeOn)
		effectiveTotalSupply := new(big.Int).Add(totalSupply, protocolLiquidity)
		out[strings.ToLower(pair.Address)] = pairRuntimeState{
			reserves:             reserves,
			totalSupply:          totalSupply,
			kLast:                kLast,
			token0Balance:        token0Balance,
			token1Balance:        token1Balance,
			feeTo:                feeTo,
			feeOn:                feeOn,
			protocolLiquidity:    protocolLiquidity,
			effectiveTotalSupply: effectiveTotalSupply,
		}
	}
	return out, nil
}

func buildLiquidityPosition(chainID int64, owner string, pair Pair, lpBalance *big.Int, state pairRuntimeState, farmingPool *FarmingPool) (core.Position, bool) {
	lpBalance = zeroIfNil(lpBalance)
	if lpBalance.Sign() == 0 || zeroIfNil(state.effectiveTotalSupply).Sign() == 0 {
		return core.Position{}, false
	}

	amount0 := mulDiv(lpBalance, state.token0Balance, state.effectiveTotalSupply)
	amount1 := mulDiv(lpBalance, state.token1Balance, state.effectiveTotalSupply)
	shareRaw := mulDiv(lpBalance, shareScale, state.effectiveTotalSupply)
	strategy := "liquidity"
	idSuffix := "liquidity"
	displayName := fmt.Sprintf("Uniswap V2 %s / %s LP", pair.Token0.Symbol, pair.Token1.Symbol)
	extra := map[string]any{
		"strategy":                  strategy,
		"marketId":                  pair.MarketID,
		"pair":                      pair.Address,
		"debankPoolId":              pair.DebankPoolID,
		"token0":                    pair.Token0,
		"token1":                    pair.Token1,
		"reserve0Raw":               bigString(state.reserves.Reserve0),
		"reserve1Raw":               bigString(state.reserves.Reserve1),
		"blockTimestampLast":        state.reserves.BlockTimestampLast,
		"token0BalanceRaw":          bigString(state.token0Balance),
		"token1BalanceRaw":          bigString(state.token1Balance),
		"totalSupplyRaw":            bigString(state.totalSupply),
		"effectiveTotalSupplyRaw":   bigString(state.effectiveTotalSupply),
		"protocolLiquidityRaw":      bigString(state.protocolLiquidity),
		"kLastRaw":                  bigString(state.kLast),
		"feeOn":                     state.feeOn,
		"feeTo":                     state.feeTo.Hex(),
		"userShareRaw":              bigString(shareRaw),
		"userShareFormatted":        evm.FormatUnits(shareRaw, 18),
		"underlyingFormula":         "tokenBalanceOfPair * userLpBalance / effectiveTotalSupply",
		"protocolFeeFormulaApplied": state.protocolLiquidity.Sign() > 0,
		"readMethods":               "balanceOf,getReserves,totalSupply,kLast,feeTo,token.balanceOf",
	}
	if farmingPool != nil {
		strategy = "farming"
		idSuffix = "farming:" + strings.ToLower(farmingPool.StakingContract)
		displayName = farmingPool.DisplayName
		extra["strategy"] = strategy
		extra["stakingContract"] = farmingPool.StakingContract
		extra["farmingPoolId"] = farmingPool.ID
		extra["farmingPoolName"] = farmingPool.Name
		extra["debankPoolId"] = farmingPool.DebankPoolID
	}

	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:%s", ProtocolID, chainID, owner, strings.ToLower(pair.Address), idSuffix),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeLiquidity,
		DisplayName: displayName,
		Shares: []core.TokenAmount{
			tokenAmount(pair.PairToken, lpBalance),
		},
		Underlying: []core.TokenAmount{
			tokenAmount(pair.Token0, amount0),
			tokenAmount(pair.Token1, amount1),
		},
		Extra: extra,
	}, true
}

func buildFarmingRewardPosition(chainID int64, owner string, pool FarmingPool, earned *big.Int) core.Position {
	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:reward", ProtocolID, chainID, owner, pool.ID),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeReward,
		DisplayName: pool.DisplayName + " Rewards",
		Underlying: []core.TokenAmount{
			tokenAmount(pool.RewardToken, earned),
		},
		Extra: map[string]any{
			"strategy":        "farming-reward",
			"farmingPoolId":   pool.ID,
			"stakingContract": pool.StakingContract,
			"pair":            pool.Pair,
			"rewardToken":     pool.RewardToken,
			"debankPoolId":    pool.DebankPoolID,
			"earnedRaw":       bigString(earned),
			"readMethods":     "earned",
		},
	}
}

func pairFromFarmingPool(pool FarmingPool) Pair {
	return Pair{
		MarketID:     pool.MarketID,
		Address:      pool.Pair,
		PairToken:    pool.PairToken,
		Token0:       pool.Token0,
		Token1:       pool.Token1,
		DebankPoolID: strings.ToLower(pool.Pair),
	}
}

func uniquePairs(pairs []Pair) []Pair {
	out := make([]Pair, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		key := strings.ToLower(pair.Address)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pair)
	}
	return out
}

func pendingProtocolLiquidity(totalSupply *big.Int, reserve0 *big.Int, reserve1 *big.Int, kLast *big.Int, feeOn bool) *big.Int {
	totalSupply = zeroIfNil(totalSupply)
	reserve0 = zeroIfNil(reserve0)
	reserve1 = zeroIfNil(reserve1)
	kLast = zeroIfNil(kLast)
	if !feeOn || totalSupply.Sign() == 0 || reserve0.Sign() == 0 || reserve1.Sign() == 0 || kLast.Sign() == 0 {
		return big.NewInt(0)
	}

	rootK := new(big.Int).Sqrt(new(big.Int).Mul(reserve0, reserve1))
	rootKLast := new(big.Int).Sqrt(kLast)
	if rootK.Cmp(rootKLast) <= 0 {
		return big.NewInt(0)
	}

	numerator := new(big.Int).Mul(totalSupply, new(big.Int).Sub(rootK, rootKLast))
	denominator := new(big.Int).Add(new(big.Int).Mul(rootK, big.NewInt(5)), rootKLast)
	if denominator.Sign() == 0 {
		return big.NewInt(0)
	}
	return numerator.Div(numerator, denominator)
}

func mulDiv(a *big.Int, b *big.Int, denominator *big.Int) *big.Int {
	a = zeroIfNil(a)
	b = zeroIfNil(b)
	denominator = zeroIfNil(denominator)
	if a.Sign() == 0 || b.Sign() == 0 || denominator.Sign() == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Div(new(big.Int).Mul(a, b), denominator)
}

func tokenAmount(token core.Token, value *big.Int) core.TokenAmount {
	value = zeroIfNil(value)
	return core.TokenAmount{
		Token:     token,
		Raw:       value.String(),
		Formatted: evm.FormatUnits(value, token.Decimals),
	}
}

func zeroIfNil(value *big.Int) *big.Int {
	if value == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(value)
}

func bigString(value *big.Int) string {
	return zeroIfNil(value).String()
}
