package uniswapv3

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

type ownerPosition struct {
	market  Market
	tokenID *big.Int
	data    positionOutput
	pool    Pool
}

type poolRuntimeState struct {
	slot0            slot0Output
	poolLiquidity    *big.Int
	feeGrowthGlobal0 *big.Int
	feeGrowthGlobal1 *big.Int
	lowerTick        tickOutput
	upperTick        tickOutput
}

func (f Fetcher) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	factory, err := requireMulticallFactory(f.multicallFactory)
	if err != nil {
		return nil, err
	}
	owner := strings.ToLower(strings.TrimSpace(req.Owner))
	if !common.IsHexAddress(owner) {
		return nil, fmt.Errorf("uniswap v3 fetch: owner %q is not a valid EVM address", req.Owner)
	}

	markets, pools, err := loadMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 || len(pools) == 0 {
		return nil, nil
	}

	runner, err := factory(req.Chain)
	if err != nil {
		return nil, fmt.Errorf("create multicall runner: %w", err)
	}
	ownerAddress := common.HexToAddress(owner)

	userPositions, err := fetchOwnerPositions(ctx, runner, markets, pools, ownerAddress)
	if err != nil {
		return nil, err
	}
	if len(userPositions) == 0 {
		return nil, nil
	}
	states, err := fetchPoolRuntimeStates(ctx, runner, userPositions)
	if err != nil {
		return nil, err
	}

	out := make([]core.Position, 0, len(userPositions))
	for _, item := range userPositions {
		state, ok := states[positionStateKey(item)]
		if !ok {
			return nil, fmt.Errorf("uniswap v3 position %s: runtime state missing", item.tokenID.String())
		}
		position, ok, err := buildLiquidityPosition(req.Chain.ID, owner, item, state)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, position)
		}
	}
	return out, nil
}

func loadMetadata(ctx context.Context, req adapter.FetchRequest) ([]Market, []Pool, error) {
	if req.Store == nil {
		return nil, nil, fmt.Errorf("uniswap v3 fetch: cache store is nil")
	}

	var markets []Market
	marketsInfo, marketsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets)

	var pools []Pool
	poolsInfo, poolsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: poolsNamespace,
	}, &pools)

	if errors.Is(marketsErr, cache.ErrNotFound) || errors.Is(poolsErr, cache.ErrNotFound) {
		return nil, nil, syncRequiredError(req.Chain, "metadata is missing")
	}
	if marketsErr != nil {
		return nil, nil, marketsErr
	}
	if poolsErr != nil {
		return nil, nil, poolsErr
	}

	maxAge := req.MetadataMaxAge
	if maxAge == 0 {
		maxAge = defaultMetadataMaxAge
	}
	if maxAge > 0 {
		for _, info := range []core.MetadataInfo{marketsInfo, poolsInfo} {
			if metadataExpired(info, maxAge) {
				return nil, nil, syncRequiredError(req.Chain, fmt.Sprintf("metadata namespace %q is older than %s", info.Namespace, maxAge))
			}
		}
	}
	return markets, pools, nil
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
	return fmt.Errorf("uniswap v3 fetch: %s; run `dpr sync-metadata -chain %s -protocol %s` before fetching positions", reason, chainName, ProtocolID)
}

func fetchOwnerPositions(ctx context.Context, runner MulticallRunner, markets []Market, pools []Pool, owner common.Address) ([]ownerPosition, error) {
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
		return nil, fmt.Errorf("uniswap v3 balances: got %d results, want %d", len(results), len(calls))
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
		return nil, fmt.Errorf("uniswap v3 tokenOfOwnerByIndex: got %d results, want %d", len(tokenResults), len(tokenCalls))
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
		return nil, fmt.Errorf("uniswap v3 positions: got %d results, want %d", len(positionResults), len(positionCalls))
	}

	poolsByKey := poolsByPositionKey(pools)
	out := make([]ownerPosition, 0, len(positionResults))
	for i, result := range positionResults {
		if !result.Success {
			return nil, fmt.Errorf("uniswap v3 market %s position %s failed", positionPending[i].market.ID, positionPending[i].tokenID.String())
		}
		position, err := unpackPosition(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 market %s position %s: %w", positionPending[i].market.ID, positionPending[i].tokenID.String(), err)
		}
		pool, ok := poolsByKey[poolPositionKey(position.Token0, position.Token1, position.Fee)]
		if !ok {
			return nil, fmt.Errorf("uniswap v3 position %s pool metadata missing for %s/%s fee %d; run full sync-metadata or sync this owner address", positionPending[i].tokenID.String(), position.Token0.Hex(), position.Token1.Hex(), position.Fee)
		}
		out = append(out, ownerPosition{
			market:  positionPending[i].market,
			tokenID: positionPending[i].tokenID,
			data:    position,
			pool:    pool,
		})
	}
	return out, nil
}

func fetchPoolRuntimeStates(ctx context.Context, runner MulticallRunner, positions []ownerPosition) (map[string]poolRuntimeState, error) {
	if len(positions) == 0 {
		return nil, nil
	}
	type pendingState struct {
		position ownerPosition
	}
	pending := make([]pendingState, 0, len(positions))
	calls := make([]evm.Call, 0, len(positions)*6)
	for _, position := range positions {
		poolAddress := common.HexToAddress(position.pool.Address)
		slot0Call, err := packPool("slot0")
		if err != nil {
			return nil, err
		}
		liquidityCall, err := packPool("liquidity")
		if err != nil {
			return nil, err
		}
		feeGrowth0Call, err := packPool("feeGrowthGlobal0X128")
		if err != nil {
			return nil, err
		}
		feeGrowth1Call, err := packPool("feeGrowthGlobal1X128")
		if err != nil {
			return nil, err
		}
		lowerTickCall, err := packPool("ticks", big.NewInt(int64(position.data.TickLower)))
		if err != nil {
			return nil, err
		}
		upperTickCall, err := packPool("ticks", big.NewInt(int64(position.data.TickUpper)))
		if err != nil {
			return nil, err
		}
		pending = append(pending, pendingState{position: position})
		calls = append(calls,
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: slot0Call},
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: liquidityCall},
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: feeGrowth0Call},
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: feeGrowth1Call},
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: lowerTickCall},
			evm.Call{Target: poolAddress, AllowFailure: false, CallData: upperTickCall},
		)
	}
	results, err := aggregateInBatches(ctx, runner, calls, defaultMulticallBatchSz)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("uniswap v3 runtime states: got %d results, want %d", len(results), len(calls))
	}

	out := make(map[string]poolRuntimeState, len(pending))
	for i, item := range pending {
		base := i * 6
		for offset, label := range []string{"slot0", "liquidity", "feeGrowthGlobal0X128", "feeGrowthGlobal1X128", "ticks(lower)", "ticks(upper)"} {
			if !results[base+offset].Success {
				return nil, fmt.Errorf("uniswap v3 position %s pool %s: %s failed", item.position.tokenID.String(), item.position.pool.Address, label)
			}
		}
		slot0, err := unpackSlot0(results[base].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s slot0: %w", item.position.pool.Address, err)
		}
		poolLiquidity, err := unpackLiquidity(results[base+1].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s liquidity: %w", item.position.pool.Address, err)
		}
		feeGrowth0, err := unpackFeeGrowthGlobal0(results[base+2].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s feeGrowthGlobal0: %w", item.position.pool.Address, err)
		}
		feeGrowth1, err := unpackFeeGrowthGlobal1(results[base+3].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s feeGrowthGlobal1: %w", item.position.pool.Address, err)
		}
		lowerTick, err := unpackTick(results[base+4].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s lower tick: %w", item.position.pool.Address, err)
		}
		upperTick, err := unpackTick(results[base+5].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("uniswap v3 pool %s upper tick: %w", item.position.pool.Address, err)
		}
		out[positionStateKey(item.position)] = poolRuntimeState{
			slot0:            slot0,
			poolLiquidity:    poolLiquidity,
			feeGrowthGlobal0: feeGrowth0,
			feeGrowthGlobal1: feeGrowth1,
			lowerTick:        lowerTick,
			upperTick:        upperTick,
		}
	}
	return out, nil
}

func buildLiquidityPosition(chainID int64, owner string, item ownerPosition, state poolRuntimeState) (core.Position, bool, error) {
	liquidity := zeroIfNil(item.data.Liquidity)
	if liquidity.Sign() == 0 && zeroIfNil(item.data.TokensOwed0).Sign() == 0 && zeroIfNil(item.data.TokensOwed1).Sign() == 0 {
		return core.Position{}, false, nil
	}
	sqrtLower, err := sqrtRatioAtTick(item.data.TickLower)
	if err != nil {
		return core.Position{}, false, fmt.Errorf("uniswap v3 position %s lower tick: %w", item.tokenID.String(), err)
	}
	sqrtUpper, err := sqrtRatioAtTick(item.data.TickUpper)
	if err != nil {
		return core.Position{}, false, fmt.Errorf("uniswap v3 position %s upper tick: %w", item.tokenID.String(), err)
	}
	amount0, amount1 := amountsForLiquidity(state.slot0.SqrtPriceX96, sqrtLower, sqrtUpper, liquidity)
	inside0, inside1 := feeGrowthInside(
		state.slot0.Tick,
		item.data.TickLower,
		item.data.TickUpper,
		state.feeGrowthGlobal0,
		state.feeGrowthGlobal1,
		state.lowerTick,
		state.upperTick,
	)
	fee0, fee1 := uncollectedFees(
		liquidity,
		inside0,
		inside1,
		item.data.FeeGrowthInside0LastX128,
		item.data.FeeGrowthInside1LastX128,
		item.data.TokensOwed0,
		item.data.TokensOwed1,
	)

	rewards := make([]core.TokenAmount, 0, 2)
	if fee0.Sign() > 0 {
		rewards = append(rewards, tokenAmount(item.pool.Token0, fee0))
	}
	if fee1.Sign() > 0 {
		rewards = append(rewards, tokenAmount(item.pool.Token1, fee1))
	}
	positionManager := item.market.PositionManager
	displayName := fmt.Sprintf("Uniswap V3 %s / %s %s LP", item.pool.Token0.Symbol, item.pool.Token1.Symbol, item.pool.FeeFormatted)
	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:liquidity", ProtocolID, chainID, owner, item.tokenID.String()),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeLiquidity,
		DisplayName: displayName,
		Shares: []core.TokenAmount{
			{
				Token: core.Token{
					ChainID:  chainID,
					Address:  common.HexToAddress(positionManager).Hex(),
					Symbol:   "UNI-V3-POS",
					Decimals: 0,
				},
				Raw:       "1",
				Formatted: "1",
			},
		},
		Underlying: []core.TokenAmount{
			tokenAmount(item.pool.Token0, amount0),
			tokenAmount(item.pool.Token1, amount1),
		},
		Rewards: rewards,
		Extra: map[string]any{
			"strategy":                 "liquidity-pool",
			"marketId":                 item.market.ID,
			"positionManager":          positionManager,
			"tokenId":                  item.tokenID.String(),
			"pool":                     item.pool.Address,
			"debankPoolId":             item.pool.DebankPoolID,
			"token0":                   item.pool.Token0,
			"token1":                   item.pool.Token1,
			"feeTier":                  item.pool.Fee,
			"feeTierFormatted":         item.pool.FeeFormatted,
			"tickSpacing":              item.pool.TickSpacing,
			"tickLower":                item.data.TickLower,
			"tickUpper":                item.data.TickUpper,
			"currentTick":              state.slot0.Tick,
			"rangeStatus":              rangeStatus(state.slot0.Tick, item.data.TickLower, item.data.TickUpper),
			"liquidityRaw":             bigString(liquidity),
			"poolLiquidityRaw":         bigString(state.poolLiquidity),
			"sqrtPriceX96":             bigString(state.slot0.SqrtPriceX96),
			"sqrtLowerX96":             bigString(sqrtLower),
			"sqrtUpperX96":             bigString(sqrtUpper),
			"feeProtocol":              state.slot0.FeeProtocol,
			"poolUnlocked":             state.slot0.Unlocked,
			"tokensOwed0Raw":           bigString(item.data.TokensOwed0),
			"tokensOwed1Raw":           bigString(item.data.TokensOwed1),
			"feeGrowthGlobal0X128":     bigString(state.feeGrowthGlobal0),
			"feeGrowthGlobal1X128":     bigString(state.feeGrowthGlobal1),
			"feeGrowthInside0X128":     bigString(inside0),
			"feeGrowthInside1X128":     bigString(inside1),
			"feeGrowthInside0LastX128": bigString(item.data.FeeGrowthInside0LastX128),
			"feeGrowthInside1LastX128": bigString(item.data.FeeGrowthInside1LastX128),
			"principalFormula":         "LiquidityAmounts.getAmountsForLiquidity",
			"feeFormula":               "tokensOwed + liquidity * feeGrowthInsideDelta / 2^128",
			"readMethods":              "balanceOf,tokenOfOwnerByIndex,positions,slot0,liquidity,feeGrowthGlobal,ticks",
		},
	}, true, nil
}

func poolsByPositionKey(pools []Pool) map[string]Pool {
	out := make(map[string]Pool, len(pools))
	for _, pool := range pools {
		out[poolPositionKey(common.HexToAddress(pool.Token0.Address), common.HexToAddress(pool.Token1.Address), pool.Fee)] = pool
	}
	return out
}

func poolPositionKey(token0 common.Address, token1 common.Address, fee uint32) string {
	return fmt.Sprintf("%s:%s:%d", strings.ToLower(token0.Hex()), strings.ToLower(token1.Hex()), fee)
}

func positionStateKey(position ownerPosition) string {
	return strings.ToLower(position.pool.Address) + ":" + position.tokenID.String()
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
