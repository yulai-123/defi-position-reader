package aavev3

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

type userReserveCall struct {
	market  MarketConfig
	reserve LendingReserve
}

type activeYieldVault struct {
	vault   YieldVault
	balance *big.Int
}

func (f Fetcher) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	factory, err := requireMulticallFactory(f.multicallFactory)
	if err != nil {
		return nil, err
	}
	owner := strings.ToLower(strings.TrimSpace(req.Owner))
	if !common.IsHexAddress(owner) {
		return nil, fmt.Errorf("aave v3 fetch: owner %q is not a valid EVM address", req.Owner)
	}

	markets, reserves, yieldVaults, err := loadAaveMetadata(ctx, req)
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
	positions := make([]core.Position, 0, len(markets)+len(yieldVaults))
	if len(reserves) > 0 {
		reservesByMarket := groupReservesByMarket(reserves)
		for _, market := range markets {
			marketReserves := reservesByMarket[market.ID]
			if len(marketReserves) == 0 {
				continue
			}
			position, ok, err := fetchMarketPosition(ctx, runner, req.Chain.ID, owner, ownerAddress, market, marketReserves)
			if err != nil {
				return nil, err
			}
			if ok {
				positions = append(positions, position)
			}
		}
	}

	yieldPositions, err := fetchYieldPositions(ctx, runner, req.Chain.ID, owner, ownerAddress, markets, yieldVaults)
	if err != nil {
		return nil, err
	}
	positions = append(positions, yieldPositions...)

	return positions, nil
}

func loadAaveMetadata(ctx context.Context, req adapter.FetchRequest) ([]MarketConfig, []LendingReserve, []YieldVault, error) {
	if req.Store == nil {
		return nil, nil, nil, fmt.Errorf("aave v3 fetch: cache store is nil")
	}

	var markets []MarketConfig
	marketsInfo, marketsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets)

	var reserves []LendingReserve
	reservesInfo, reservesErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: lendingReservesNamespace,
	}, &reserves)

	var yieldVaults []YieldVault
	yieldVaultsInfo, yieldVaultsErr := req.Store.Get(ctx, cache.Key{
		ChainID:   req.Chain.ID,
		Protocol:  ProtocolID,
		Namespace: yieldVaultsNamespace,
	}, &yieldVaults)

	if errors.Is(marketsErr, cache.ErrNotFound) || errors.Is(reservesErr, cache.ErrNotFound) || errors.Is(yieldVaultsErr, cache.ErrNotFound) {
		return nil, nil, nil, metadataSyncRequiredError(req.Chain, "metadata is missing")
	}
	if marketsErr != nil {
		return nil, nil, nil, marketsErr
	}
	if reservesErr != nil {
		return nil, nil, nil, reservesErr
	}
	if yieldVaultsErr != nil {
		return nil, nil, nil, yieldVaultsErr
	}

	maxAge := req.MetadataMaxAge
	if maxAge == 0 {
		maxAge = defaultMetadataMaxAge
	}
	if maxAge > 0 {
		for _, info := range []core.MetadataInfo{marketsInfo, reservesInfo, yieldVaultsInfo} {
			if metadataExpired(info, maxAge) {
				return nil, nil, nil, metadataSyncRequiredError(req.Chain, fmt.Sprintf("metadata namespace %q is older than %s", info.Namespace, maxAge))
			}
		}
	}
	return markets, reserves, yieldVaults, nil
}

func metadataExpired(info core.MetadataInfo, maxAge time.Duration) bool {
	if info.UpdatedAt.IsZero() {
		return true
	}
	return time.Since(info.UpdatedAt) > maxAge
}

func metadataSyncRequiredError(target core.Chain, reason string) error {
	chainName := target.Name
	if chainName == "" {
		chainName = fmt.Sprintf("%d", target.ID)
	}
	return fmt.Errorf("aave v3 fetch: %s; run `dpr sync-metadata -chain %s -protocol %s` before fetching positions", reason, chainName, ProtocolID)
}

func fetchMarketPosition(
	ctx context.Context,
	runner MulticallRunner,
	chainID int64,
	owner string,
	ownerAddress common.Address,
	market MarketConfig,
	reserves []LendingReserve,
) (core.Position, bool, error) {
	accountDataCall, err := packPool("getUserAccountData", ownerAddress)
	if err != nil {
		return core.Position{}, false, err
	}
	calls := make([]evm.Call, 0, len(reserves)+1)
	calls = append(calls, evm.Call{
		Target:       common.HexToAddress(market.Pool),
		AllowFailure: false,
		CallData:     accountDataCall,
	})

	userReserveCalls := make([]userReserveCall, 0, len(reserves))
	for _, reserve := range reserves {
		data, err := packDataProvider("getUserReserveData", common.HexToAddress(reserve.Asset.Address), ownerAddress)
		if err != nil {
			return core.Position{}, false, err
		}
		userReserveCalls = append(userReserveCalls, userReserveCall{market: market, reserve: reserve})
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(market.DataProvider),
			AllowFailure: false,
			CallData:     data,
		})
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return core.Position{}, false, err
	}
	if len(results) != len(calls) {
		return core.Position{}, false, fmt.Errorf("aave v3 market %s: got %d multicall results, want %d", market.ID, len(results), len(calls))
	}
	if !results[0].Success {
		return core.Position{}, false, fmt.Errorf("aave v3 market %s: getUserAccountData failed", market.ID)
	}
	accountData, err := unpackUserAccountData(results[0].ReturnData)
	if err != nil {
		return core.Position{}, false, fmt.Errorf("aave v3 market %s: %w", market.ID, err)
	}

	shares := make([]core.TokenAmount, 0)
	underlying := make([]core.TokenAmount, 0)
	debt := make([]core.TokenAmount, 0)
	reserveExtras := make([]map[string]any, 0)
	for i, call := range userReserveCalls {
		result := results[i+1]
		reserve := call.reserve
		if !result.Success {
			return core.Position{}, false, fmt.Errorf("aave v3 market %s reserve %s: getUserReserveData failed", market.ID, reserve.Asset.Address)
		}
		userReserve, err := unpackUserReserveData(result.ReturnData)
		if err != nil {
			return core.Position{}, false, fmt.Errorf("aave v3 market %s reserve %s: %w", market.ID, reserve.Asset.Address, err)
		}

		supply := zeroIfNil(userReserve.CurrentATokenBalance)
		stableDebt := zeroIfNil(userReserve.CurrentStableDebt)
		variableDebt := zeroIfNil(userReserve.CurrentVariableDebt)
		totalDebt := new(big.Int).Add(stableDebt, variableDebt)
		if supply.Sign() == 0 && totalDebt.Sign() == 0 {
			continue
		}

		if supply.Sign() > 0 {
			shares = append(shares, tokenAmount(reserve.AToken, supply))
			underlying = append(underlying, tokenAmount(reserve.Asset, supply))
		}
		if totalDebt.Sign() > 0 {
			debt = append(debt, tokenAmount(reserve.Asset, totalDebt))
		}

		reserveExtras = append(reserveExtras, map[string]any{
			"asset":                    reserve.Asset.Symbol,
			"assetAddress":             reserve.Asset.Address,
			"aToken":                   reserve.AToken.Address,
			"stableDebtToken":          reserve.StableDebtToken.Address,
			"variableDebtToken":        reserve.VariableDebtToken.Address,
			"supplyRaw":                supply.String(),
			"stableDebtRaw":            stableDebt.String(),
			"variableDebtRaw":          variableDebt.String(),
			"scaledVariableDebtRaw":    bigString(userReserve.ScaledVariableDebt),
			"liquidityRateRaw":         bigString(userReserve.LiquidityRate),
			"stableBorrowRateRaw":      bigString(userReserve.StableBorrowRate),
			"usageAsCollateralEnabled": userReserve.UsageAsCollateralEnabled,
			"reserveCollateralEnabled": reserve.UsageAsCollateralEnabled,
			"borrowingEnabled":         reserve.BorrowingEnabled,
			"stableBorrowRateEnabled":  reserve.StableBorrowRateEnabled,
		})
	}

	if len(shares) == 0 && len(debt) == 0 {
		return core.Position{}, false, nil
	}

	sortTokenAmounts(shares)
	sortTokenAmounts(underlying)
	sortTokenAmounts(debt)
	sort.Slice(reserveExtras, func(i, j int) bool {
		left, _ := reserveExtras[i]["asset"].(string)
		right, _ := reserveExtras[j]["asset"].(string)
		return left < right
	})

	position := core.Position{
		ID:          fmt.Sprintf("%s:%d:%s:%s:lending", ProtocolID, chainID, owner, market.ID),
		ChainID:     chainID,
		Protocol:    ProtocolID,
		Owner:       owner,
		Type:        core.PositionTypeLending,
		DisplayName: market.DisplayName,
		Shares:      shares,
		Underlying:  underlying,
		Debt:        debt,
		Extra: map[string]any{
			"strategy":                       "lending",
			"marketId":                       market.ID,
			"marketName":                     market.Name,
			"pool":                           market.Pool,
			"poolAddressesProvider":          market.PoolAddressesProvider,
			"dataProvider":                   market.DataProvider,
			"totalCollateralBaseRaw":         bigString(accountData.TotalCollateralBase),
			"totalDebtBaseRaw":               bigString(accountData.TotalDebtBase),
			"availableBorrowsBaseRaw":        bigString(accountData.AvailableBorrowsBase),
			"currentLiquidationThresholdRaw": bigString(accountData.CurrentLiquidationThreshold),
			"ltvRaw":                         bigString(accountData.LTV),
			"healthFactorRaw":                bigString(accountData.HealthFactor),
			"healthFactorFormatted":          evm.FormatUnits(accountData.HealthFactor, 18),
			"reserves":                       reserveExtras,
		},
	}
	return position, true, nil
}

func fetchYieldPositions(
	ctx context.Context,
	runner MulticallRunner,
	chainID int64,
	owner string,
	ownerAddress common.Address,
	markets []MarketConfig,
	vaults []YieldVault,
) ([]core.Position, error) {
	if len(vaults) == 0 {
		return nil, nil
	}

	balanceCall, err := packYieldVault("balanceOf", ownerAddress)
	if err != nil {
		return nil, err
	}
	calls := make([]evm.Call, 0, len(vaults))
	for _, vault := range vaults {
		calls = append(calls, evm.Call{
			Target:       common.HexToAddress(vault.VaultToken.Address),
			AllowFailure: false,
			CallData:     balanceCall,
		})
	}

	results, err := runner.Aggregate3(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("aave v3 yield: got %d balance results, want %d", len(results), len(calls))
	}

	active := make([]activeYieldVault, 0)
	for i, result := range results {
		vault := vaults[i]
		if !result.Success {
			return nil, fmt.Errorf("aave v3 yield vault %s: balanceOf failed", vault.VaultToken.Address)
		}
		balance, err := unpackVaultBalance(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("aave v3 yield vault %s: %w", vault.VaultToken.Address, err)
		}
		balance = zeroIfNil(balance)
		if balance.Sign() == 0 {
			continue
		}
		active = append(active, activeYieldVault{
			vault:   vault,
			balance: balance,
		})
	}
	if len(active) == 0 {
		return nil, nil
	}

	valueCalls := make([]evm.Call, 0)
	for _, item := range active {
		target := common.HexToAddress(item.vault.VaultToken.Address)
		previewRedeemCall, err := packYieldVault("previewRedeem", item.balance)
		if err != nil {
			return nil, err
		}
		maxWithdrawCall, err := packYieldVault("maxWithdraw", ownerAddress)
		if err != nil {
			return nil, err
		}
		maxRedeemCall, err := packYieldVault("maxRedeem", ownerAddress)
		if err != nil {
			return nil, err
		}
		valueCalls = append(valueCalls,
			evm.Call{Target: target, AllowFailure: false, CallData: previewRedeemCall},
			evm.Call{Target: target, AllowFailure: false, CallData: maxWithdrawCall},
			evm.Call{Target: target, AllowFailure: false, CallData: maxRedeemCall},
		)
		for _, rewardToken := range item.vault.RewardTokens {
			rewardCall, err := packYieldVault("getClaimableRewards", ownerAddress, common.HexToAddress(rewardToken.Address))
			if err != nil {
				return nil, err
			}
			valueCalls = append(valueCalls, evm.Call{
				Target:       target,
				AllowFailure: false,
				CallData:     rewardCall,
			})
		}
	}

	valueResults, err := runner.Aggregate3(ctx, valueCalls)
	if err != nil {
		return nil, err
	}
	if len(valueResults) != len(valueCalls) {
		return nil, fmt.Errorf("aave v3 yield: got %d value results, want %d", len(valueResults), len(valueCalls))
	}

	marketsByID := groupMarketsByID(markets)
	positions := make([]core.Position, 0, len(active))
	resultIndex := 0
	for _, item := range active {
		vault := item.vault
		if !valueResults[resultIndex].Success {
			return nil, fmt.Errorf("aave v3 yield vault %s: previewRedeem failed", vault.VaultToken.Address)
		}
		underlyingAmount, err := unpackPreviewRedeem(valueResults[resultIndex].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("aave v3 yield vault %s: %w", vault.VaultToken.Address, err)
		}
		resultIndex++

		if !valueResults[resultIndex].Success {
			return nil, fmt.Errorf("aave v3 yield vault %s: maxWithdraw failed", vault.VaultToken.Address)
		}
		maxWithdraw, err := unpackMaxWithdraw(valueResults[resultIndex].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("aave v3 yield vault %s: %w", vault.VaultToken.Address, err)
		}
		resultIndex++

		if !valueResults[resultIndex].Success {
			return nil, fmt.Errorf("aave v3 yield vault %s: maxRedeem failed", vault.VaultToken.Address)
		}
		maxRedeem, err := unpackMaxRedeem(valueResults[resultIndex].ReturnData)
		if err != nil {
			return nil, fmt.Errorf("aave v3 yield vault %s: %w", vault.VaultToken.Address, err)
		}
		resultIndex++

		rewards := make([]map[string]any, 0, len(vault.RewardTokens))
		for _, rewardToken := range vault.RewardTokens {
			if !valueResults[resultIndex].Success {
				return nil, fmt.Errorf("aave v3 yield vault %s reward %s: getClaimableRewards failed", vault.VaultToken.Address, rewardToken.Address)
			}
			rewardAmount, err := unpackClaimableRewards(valueResults[resultIndex].ReturnData)
			if err != nil {
				return nil, fmt.Errorf("aave v3 yield vault %s reward %s: %w", vault.VaultToken.Address, rewardToken.Address, err)
			}
			resultIndex++
			rewardAmount = zeroIfNil(rewardAmount)
			if rewardAmount.Sign() == 0 {
				continue
			}
			rewards = append(rewards, map[string]any{
				"token":     rewardToken,
				"raw":       rewardAmount.String(),
				"formatted": evm.FormatUnits(rewardAmount, rewardToken.Decimals),
			})
		}

		market := marketsByID[vault.MarketID]
		displayName := yieldDisplayName(market, vault)
		position := core.Position{
			ID:          fmt.Sprintf("%s:%d:%s:%s:yield:%s", ProtocolID, chainID, owner, vault.MarketID, strings.ToLower(vault.VaultToken.Address)),
			ChainID:     chainID,
			Protocol:    ProtocolID,
			Owner:       owner,
			Type:        core.PositionTypeYield,
			DisplayName: displayName,
			Shares: []core.TokenAmount{
				tokenAmount(vault.VaultToken, item.balance),
			},
			Underlying: []core.TokenAmount{
				tokenAmount(vault.Asset, underlyingAmount),
			},
			Extra: map[string]any{
				"strategy":              "yield",
				"marketId":              vault.MarketID,
				"marketName":            market.Name,
				"vault":                 vault.VaultToken.Address,
				"vaultKind":             vault.Kind,
				"factory":               vault.Factory,
				"asset":                 vault.Asset,
				"aToken":                vault.AToken,
				"previewRedeemRaw":      bigString(underlyingAmount),
				"maxWithdrawRaw":        bigString(maxWithdraw),
				"maxWithdrawFormatted":  evm.FormatUnits(maxWithdraw, vault.Asset.Decimals),
				"maxRedeemRaw":          bigString(maxRedeem),
				"maxRedeemFormatted":    evm.FormatUnits(maxRedeem, vault.VaultToken.Decimals),
				"claimableRewards":      rewards,
				"claimableRewardsCount": len(rewards),
			},
		}
		positions = append(positions, position)
	}
	return positions, nil
}

func groupMarketsByID(markets []MarketConfig) map[string]MarketConfig {
	grouped := make(map[string]MarketConfig, len(markets))
	for _, market := range markets {
		grouped[market.ID] = market
	}
	return grouped
}

func yieldDisplayName(market MarketConfig, vault YieldVault) string {
	base := strings.TrimSuffix(market.DisplayName, " Lending")
	if base == "" {
		base = "Aave V3"
	}
	if vault.Asset.Symbol == "" {
		return base + " Yield"
	}
	return base + " Yield " + vault.Asset.Symbol
}

func groupReservesByMarket(reserves []LendingReserve) map[string][]LendingReserve {
	grouped := make(map[string][]LendingReserve)
	for _, reserve := range reserves {
		grouped[reserve.MarketID] = append(grouped[reserve.MarketID], reserve)
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

func tokenAmount(token core.Token, value *big.Int) core.TokenAmount {
	if value == nil {
		value = big.NewInt(0)
	}
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

func zeroIfNil(value *big.Int) *big.Int {
	if value == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(value)
}
