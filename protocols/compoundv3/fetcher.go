package compoundv3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
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

var factorScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

type marketUserState struct {
	baseSupply             *big.Int
	baseDebt               *big.Int
	basePrice              *big.Int
	isBorrowCollateralized bool
	isLiquidatable         bool
	collaterals            []collateralUserState
	reward                 *rewardOwedOutput
	rewardConfig           RewardConfig
}

type collateralUserState struct {
	asset   CollateralAsset
	balance *big.Int
	price   *big.Int
	value   *big.Int
}

func (f Fetcher) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	factory, err := requireMulticallFactory(f.multicallFactory)
	if err != nil {
		return nil, err
	}
	owner := strings.ToLower(strings.TrimSpace(req.Owner))
	if !common.IsHexAddress(owner) {
		return nil, fmt.Errorf("compound v3 fetch: owner %q is not a valid EVM address", req.Owner)
	}

	markets, collaterals, rewards, err := loadMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return nil, nil
	}
	runner, err := factory(req.Chain)
	if err != nil {
		return nil, fmt.Errorf("create multicall runner: %w", err)
	}

	ownerAddress := common.HexToAddress(owner)
	collateralsByMarket := groupCollateralsByMarket(collaterals)
	rewardsByMarket := groupRewardsByMarket(rewards)
	positions := make([]core.Position, 0, len(markets)*3)
	for _, market := range markets {
		state, err := fetchMarketState(ctx, runner, ownerAddress, market, collateralsByMarket[market.ID], rewardsByMarket[market.ID])
		if err != nil {
			return nil, err
		}
		if position, ok := buildYieldPosition(req.Chain.ID, owner, market, state); ok {
			positions = append(positions, position)
		}
		if position, ok := buildLendingPosition(req.Chain.ID, owner, market, state); ok {
			positions = append(positions, position)
		}
		if position, ok := buildRewardPosition(req.Chain.ID, owner, market, state); ok {
			positions = append(positions, position)
		}
	}
	return positions, nil
}

func loadMetadata(ctx context.Context, req adapter.FetchRequest) ([]Market, []CollateralAsset, []RewardConfig, error) {
	if req.Store == nil {
		return nil, nil, nil, fmt.Errorf("compound v3 fetch: cache store is nil")
	}

	var markets []Market
	marketsInfo, marketsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets)

	var collaterals []CollateralAsset
	collateralsInfo, collateralsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: collateralAssetsNamespace,
	}, &collaterals)

	var rewards []RewardConfig
	rewardsInfo, rewardsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: rewardConfigsNamespace,
	}, &rewards)

	if errors.Is(marketsErr, cache.ErrNotFound) || errors.Is(collateralsErr, cache.ErrNotFound) || errors.Is(rewardsErr, cache.ErrNotFound) {
		return nil, nil, nil, syncRequiredError(req.Chain, "metadata is missing")
	}
	if marketsErr != nil {
		return nil, nil, nil, marketsErr
	}
	if collateralsErr != nil {
		return nil, nil, nil, collateralsErr
	}
	if rewardsErr != nil {
		return nil, nil, nil, rewardsErr
	}

	maxAge := req.MetadataMaxAge
	if maxAge == 0 {
		maxAge = defaultMetadataMaxAge
	}
	if maxAge > 0 {
		for _, info := range []core.MetadataInfo{marketsInfo, collateralsInfo, rewardsInfo} {
			if metadataExpired(info, maxAge) {
				return nil, nil, nil, syncRequiredError(req.Chain, fmt.Sprintf("metadata namespace %q is older than %s", info.Namespace, maxAge))
			}
		}
	}
	return markets, collaterals, rewards, nil
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
	return fmt.Errorf("compound v3 fetch: %s; run `dpr sync-metadata -chain %s -protocol %s` before fetching positions", reason, chainName, ProtocolID)
}

func fetchMarketState(
	ctx context.Context,
	runner MulticallRunner,
	owner common.Address,
	market Market,
	collaterals []CollateralAsset,
	reward RewardConfig,
) (marketUserState, error) {
	calls := make([]evm.Call, 0, 5+len(collaterals)*2+1)
	target := common.HexToAddress(market.Comet)
	for _, method := range []string{"balanceOf", "borrowBalanceOf", "isBorrowCollateralized", "isLiquidatable"} {
		data, err := packComet(method, owner)
		if err != nil {
			return marketUserState{}, err
		}
		calls = append(calls, evm.Call{Target: target, AllowFailure: false, CallData: data})
	}
	basePriceCall, err := packComet("getPrice", common.HexToAddress(market.BasePriceFeed))
	if err != nil {
		return marketUserState{}, err
	}
	calls = append(calls, evm.Call{Target: target, AllowFailure: true, CallData: basePriceCall})

	for _, collateral := range collaterals {
		balanceCall, err := packComet("collateralBalanceOf", owner, common.HexToAddress(collateral.Asset.Address))
		if err != nil {
			return marketUserState{}, err
		}
		priceCall, err := packComet("getPrice", common.HexToAddress(collateral.PriceFeed))
		if err != nil {
			return marketUserState{}, err
		}
		calls = append(calls,
			evm.Call{Target: target, AllowFailure: false, CallData: balanceCall},
			evm.Call{Target: target, AllowFailure: true, CallData: priceCall},
		)
	}

	hasRewardCall := reward.Supported && reward.Rewards != "" && reward.Rewards != (common.Address{}).Hex()
	if hasRewardCall {
		rewardCall, err := packRewards("getRewardOwed", common.HexToAddress(market.Comet), owner)
		if err != nil {
			return marketUserState{}, err
		}
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(reward.Rewards),
			AllowFailure: true,
			CallData:     rewardCall,
		})
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return marketUserState{}, err
	}
	if len(results) != len(calls) {
		return marketUserState{}, fmt.Errorf("compound v3 market %s: got %d multicall results, want %d", market.ID, len(results), len(calls))
	}
	for i, method := range []string{"balanceOf", "borrowBalanceOf", "isBorrowCollateralized", "isLiquidatable"} {
		if !results[i].Success {
			return marketUserState{}, fmt.Errorf("compound v3 market %s: %s failed", market.ID, method)
		}
	}

	baseSupply, err := unpackBalance(results[0].ReturnData)
	if err != nil {
		return marketUserState{}, fmt.Errorf("compound v3 market %s: %w", market.ID, err)
	}
	baseDebt, err := unpackBorrowBalance(results[1].ReturnData)
	if err != nil {
		return marketUserState{}, fmt.Errorf("compound v3 market %s: %w", market.ID, err)
	}
	isBorrowCollateralized, err := unpackIsBorrowCollateralized(results[2].ReturnData)
	if err != nil {
		return marketUserState{}, fmt.Errorf("compound v3 market %s: %w", market.ID, err)
	}
	isLiquidatable, err := unpackIsLiquidatable(results[3].ReturnData)
	if err != nil {
		return marketUserState{}, fmt.Errorf("compound v3 market %s: %w", market.ID, err)
	}

	var basePrice *big.Int
	if results[4].Success {
		basePrice, err = unpackPrice(results[4].ReturnData)
		if err != nil {
			return marketUserState{}, fmt.Errorf("compound v3 market %s base price: %w", market.ID, err)
		}
	}

	state := marketUserState{
		baseSupply:             zeroIfNil(baseSupply),
		baseDebt:               zeroIfNil(baseDebt),
		basePrice:              zeroIfNil(basePrice),
		isBorrowCollateralized: isBorrowCollateralized,
		isLiquidatable:         isLiquidatable,
		collaterals:            make([]collateralUserState, 0, len(collaterals)),
		rewardConfig:           reward,
	}
	index := 5
	for _, collateral := range collaterals {
		if !results[index].Success {
			return marketUserState{}, fmt.Errorf("compound v3 market %s collateral %s balance failed", market.ID, collateral.Asset.Address)
		}
		balance, err := unpackCollateralBalance(results[index].ReturnData)
		if err != nil {
			return marketUserState{}, fmt.Errorf("compound v3 market %s collateral %s: %w", market.ID, collateral.Asset.Address, err)
		}
		index++

		var price *big.Int
		if results[index].Success {
			price, err = unpackPrice(results[index].ReturnData)
			if err != nil {
				return marketUserState{}, fmt.Errorf("compound v3 market %s collateral %s price: %w", market.ID, collateral.Asset.Address, err)
			}
		}
		index++

		value, _ := valueFromPrice(zeroIfNil(balance), zeroIfNil(price), collateral.Scale)
		state.collaterals = append(state.collaterals, collateralUserState{
			asset:   collateral,
			balance: zeroIfNil(balance),
			price:   zeroIfNil(price),
			value:   value,
		})
	}

	if hasRewardCall {
		result := results[index]
		if result.Success {
			owed, err := unpackRewardOwed(result.ReturnData)
			if err != nil {
				return marketUserState{}, fmt.Errorf("compound v3 market %s reward owed: %w", market.ID, err)
			}
			state.reward = &owed
		}
	}
	return state, nil
}

func buildYieldPosition(chainID int64, owner string, market Market, state marketUserState) (core.Position, bool) {
	if state.baseSupply.Sign() == 0 {
		return core.Position{}, false
	}
	extra := map[string]any{
		"strategy":           "yield",
		"marketId":           market.ID,
		"marketName":         market.Name,
		"comet":              market.Comet,
		"debankProtocolId":   market.DebankProtocolID,
		"debankPoolId":       market.YieldPoolID,
		"baseToken":          market.BaseToken,
		"basePriceFeed":      market.BasePriceFeed,
		"baseSupplyRaw":      state.baseSupply.String(),
		"basePriceRaw":       bigString(state.basePrice),
		"readMethods":        "balanceOf,getPrice",
		"usesCometBaseIndex": true,
	}
	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:yield", ProtocolID, chainID, owner, market.ID),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeYield,
		DisplayName: market.DisplayName + " Yield",
		Shares:      []core.TokenAmount{tokenAmount(market.CometToken, state.baseSupply)},
		Underlying:  []core.TokenAmount{tokenAmount(market.BaseToken, state.baseSupply)},
		Extra:       extra,
	}, true
}

func buildLendingPosition(chainID int64, owner string, market Market, state marketUserState) (core.Position, bool) {
	underlying := make([]core.TokenAmount, 0)
	collateralExtras := make([]map[string]any, 0)
	borrowCapacity := big.NewInt(0)
	liquidationCapacity := big.NewInt(0)

	for _, item := range state.collaterals {
		if item.balance.Sign() == 0 {
			continue
		}
		underlying = append(underlying, tokenAmount(item.asset.Asset, item.balance))

		borrowValue, _ := factorValue(item.value, item.asset.BorrowCollateralFactor)
		liquidationValue, _ := factorValue(item.value, item.asset.LiquidateCollateralFactor)
		borrowCapacity.Add(borrowCapacity, borrowValue)
		liquidationCapacity.Add(liquidationCapacity, liquidationValue)
		collateralExtras = append(collateralExtras, map[string]any{
			"asset":                        item.asset.Asset,
			"priceFeed":                    item.asset.PriceFeed,
			"balanceRaw":                   item.balance.String(),
			"priceRaw":                     bigString(item.price),
			"valueRaw":                     bigString(item.value),
			"borrowCollateralFactorRaw":    item.asset.BorrowCollateralFactor,
			"liquidateCollateralFactorRaw": item.asset.LiquidateCollateralFactor,
			"liquidationFactorRaw":         item.asset.LiquidationFactor,
			"borrowCapacityRaw":            bigString(borrowValue),
			"liquidationCapacityRaw":       bigString(liquidationValue),
			"supplyCapRaw":                 item.asset.SupplyCap,
		})
	}
	if len(underlying) == 0 && state.baseDebt.Sign() == 0 {
		return core.Position{}, false
	}

	debt := make([]core.TokenAmount, 0, 1)
	if state.baseDebt.Sign() > 0 {
		debt = append(debt, tokenAmount(market.BaseToken, state.baseDebt))
	}
	sortTokenAmounts(underlying)
	sortTokenAmounts(debt)
	sort.Slice(collateralExtras, func(i, j int) bool {
		left, _ := collateralExtras[i]["asset"].(core.Token)
		right, _ := collateralExtras[j]["asset"].(core.Token)
		return left.Symbol < right.Symbol
	})

	baseDebtValue, _ := valueFromPrice(state.baseDebt, state.basePrice, market.BaseScale)
	availableBorrow := new(big.Int).Sub(new(big.Int).Set(borrowCapacity), baseDebtValue)
	healthRaw := ratioRaw(liquidationCapacity, baseDebtValue)
	extra := map[string]any{
		"strategy":                   "lending",
		"marketId":                   market.ID,
		"marketName":                 market.Name,
		"comet":                      market.Comet,
		"debankProtocolId":           market.DebankProtocolID,
		"debankPoolId":               market.LendingPoolID,
		"baseToken":                  market.BaseToken,
		"basePriceFeed":              market.BasePriceFeed,
		"baseDebtRaw":                state.baseDebt.String(),
		"basePriceRaw":               bigString(state.basePrice),
		"baseDebtValueRaw":           bigString(baseDebtValue),
		"borrowCapacityRaw":          bigString(borrowCapacity),
		"liquidationCapacityRaw":     bigString(liquidationCapacity),
		"availableBorrowRaw":         availableBorrow.String(),
		"liquidationHealthRaw":       bigString(healthRaw),
		"liquidationHealthFormatted": formatRatio(healthRaw),
		"isBorrowCollateralized":     state.isBorrowCollateralized,
		"isLiquidatable":             state.isLiquidatable,
		"collaterals":                collateralExtras,
		"collateralCount":            len(collateralExtras),
		"readMethods":                "borrowBalanceOf,collateralBalanceOf,isBorrowCollateralized,isLiquidatable,getPrice",
	}
	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:lending", ProtocolID, chainID, owner, market.ID),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeLending,
		DisplayName: market.DisplayName + " Lending",
		Underlying:  underlying,
		Debt:        debt,
		Extra:       extra,
	}, true
}

func buildRewardPosition(chainID int64, owner string, market Market, state marketUserState) (core.Position, bool) {
	reward, ok := rewardTokenAmount(state)
	if !ok {
		return core.Position{}, false
	}
	extra := map[string]any{
		"strategy":                "reward",
		"marketId":                market.ID,
		"marketName":              market.Name,
		"comet":                   market.Comet,
		"debankProtocolId":        market.DebankProtocolID,
		"baseToken":               market.BaseToken,
		"rewardToken":             reward.Token,
		"rewardRaw":               reward.Raw,
		"readMethods":             "getRewardOwed",
		"claimableRewards":        claimableRewards(state),
		"claimableRewardsCount":   1,
		"rewards":                 market.Rewards,
		"usesCometRewardContract": true,
	}
	return core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:reward", ProtocolID, chainID, owner, market.ID),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeReward,
		DisplayName: market.DisplayName + " Rewards",
		Underlying:  []core.TokenAmount{reward},
		Extra:       extra,
	}, true
}

func claimableRewards(state marketUserState) []map[string]any {
	return []map[string]any{
		{
			"tokenAddress": state.reward.Token.Hex(),
			"token":        rewardToken(state),
			"raw":          state.reward.Owed.String(),
			"formatted":    evm.FormatUnits(state.reward.Owed, rewardToken(state).Decimals),
		},
	}
}

func rewardTokenAmount(state marketUserState) (core.TokenAmount, bool) {
	if state.reward == nil || state.reward.Owed == nil || state.reward.Owed.Sign() == 0 {
		return core.TokenAmount{}, false
	}
	token := rewardToken(state)
	return core.TokenAmount{
		Token:     token,
		Raw:       state.reward.Owed.String(),
		Formatted: evm.FormatUnits(state.reward.Owed, token.Decimals),
	}, true
}

func rewardToken(state marketUserState) core.Token {
	token := state.rewardConfig.RewardToken
	if token.Address == "" && state.reward != nil {
		token.Address = state.reward.Token.Hex()
	}
	return token
}

func groupCollateralsByMarket(items []CollateralAsset) map[string][]CollateralAsset {
	grouped := make(map[string][]CollateralAsset)
	for _, item := range items {
		grouped[item.MarketID] = append(grouped[item.MarketID], item)
	}
	for marketID := range grouped {
		sort.Slice(grouped[marketID], func(i, j int) bool {
			if grouped[marketID][i].Asset.Symbol == grouped[marketID][j].Asset.Symbol {
				return grouped[marketID][i].Asset.Address < grouped[marketID][j].Asset.Address
			}
			return grouped[marketID][i].Asset.Symbol < grouped[marketID][j].Asset.Symbol
		})
	}
	return grouped
}

func groupRewardsByMarket(items []RewardConfig) map[string]RewardConfig {
	grouped := make(map[string]RewardConfig, len(items))
	for _, item := range items {
		grouped[item.MarketID] = item
	}
	return grouped
}

func tokenAmount(token core.Token, value *big.Int) core.TokenAmount {
	value = zeroIfNil(value)
	return core.TokenAmount{
		Token:     token,
		Raw:       value.String(),
		Formatted: evm.FormatUnits(value, token.Decimals),
	}
}

func sortTokenAmounts(items []core.TokenAmount) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Token.Symbol == items[j].Token.Symbol {
			return items[i].Token.Address < items[j].Token.Address
		}
		return items[i].Token.Symbol < items[j].Token.Symbol
	})
}

func valueFromPrice(amount *big.Int, price *big.Int, scale string) (*big.Int, error) {
	amount = zeroIfNil(amount)
	price = zeroIfNil(price)
	if amount.Sign() == 0 || price.Sign() == 0 {
		return big.NewInt(0), nil
	}
	scaleValue, ok := new(big.Int).SetString(scale, 10)
	if !ok || scaleValue.Sign() == 0 {
		return nil, fmt.Errorf("invalid scale %q", scale)
	}
	value := new(big.Int).Mul(amount, price)
	value.Div(value, scaleValue)
	return value, nil
}

func factorValue(value *big.Int, factor string) (*big.Int, error) {
	value = zeroIfNil(value)
	if value.Sign() == 0 {
		return big.NewInt(0), nil
	}
	factorValue, ok := new(big.Int).SetString(factor, 10)
	if !ok {
		return nil, fmt.Errorf("invalid factor %q", factor)
	}
	out := new(big.Int).Mul(value, factorValue)
	out.Div(out, factorScale)
	return out, nil
}

func ratioRaw(numerator *big.Int, denominator *big.Int) *big.Int {
	numerator = zeroIfNil(numerator)
	denominator = zeroIfNil(denominator)
	if numerator.Sign() == 0 || denominator.Sign() == 0 {
		return big.NewInt(0)
	}
	out := new(big.Int).Mul(numerator, factorScale)
	out.Div(out, denominator)
	return out
}

func formatRatio(value *big.Int) string {
	value = zeroIfNil(value)
	if value.Sign() == 0 {
		return ""
	}
	return evm.FormatUnits(value, 18)
}

func zeroIfNil(value *big.Int) *big.Int {
	if value == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(value)
}

func bigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}
