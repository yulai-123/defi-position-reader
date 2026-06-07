package aavev3

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
	"github.com/yulai-123/defi-position-reader/pkg/evm"
)

const (
	testOwner        = "0x0000000000000000000000000000000000000001"
	testUSDC         = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	testAUSDC        = "0x4e65fE4DbA92790696d040ac24Aa414708F5c0AB"
	testStableUSDC   = "0xeb284A70557EFe3591b9e6D9D720040E02c54a4d"
	testVariableUSDC = "0x59dca05b6c26dbd64b5381374aAaC5CD05644C28"
	testVaultUSDC    = "0xe298b938631f750DD409fB18227C4a23dCdaab9b"
	testRewardAAVE   = "0x63706e401c06ac8513145b7687A14804d17f814b"
)

type fakeMulticallRunner struct {
	t     *testing.T
	calls [][]evm.Call
}

func (r *fakeMulticallRunner) Aggregate3(_ context.Context, calls []evm.Call) ([]evm.Result, error) {
	r.calls = append(r.calls, append([]evm.Call(nil), calls...))
	results := make([]evm.Result, len(calls))
	for i, call := range calls {
		result, err := r.resultFor(call)
		if err != nil {
			r.t.Fatalf("fake multicall result: %v", err)
		}
		results[i] = result
	}
	return results, nil
}

func (r *fakeMulticallRunner) resultFor(call evm.Call) (evm.Result, error) {
	method := call.CallData[:4]
	switch {
	case bytes.Equal(method, dataProviderABI.Methods["getAllReservesTokens"].ID):
		return packResult(dataProviderABI.Methods["getAllReservesTokens"].Outputs.Pack([]tokenDataOutput{
			{
				Symbol:       "USDC",
				TokenAddress: common.HexToAddress(testUSDC),
			},
		}))
	case bytes.Equal(method, dataProviderABI.Methods["getReserveTokensAddresses"].ID):
		return packResult(dataProviderABI.Methods["getReserveTokensAddresses"].Outputs.Pack(
			common.HexToAddress(testAUSDC),
			common.HexToAddress(testStableUSDC),
			common.HexToAddress(testVariableUSDC),
		))
	case bytes.Equal(method, dataProviderABI.Methods["getReserveConfigurationData"].ID):
		return packResult(dataProviderABI.Methods["getReserveConfigurationData"].Outputs.Pack(
			big.NewInt(6),
			big.NewInt(8000),
			big.NewInt(8250),
			big.NewInt(10500),
			big.NewInt(1000),
			true,
			true,
			false,
			true,
			false,
		))
	case bytes.Equal(method, erc20ABI.Methods["symbol"].ID):
		symbol := map[string]string{
			common.HexToAddress(testAUSDC).Hex():        "aBasUSDC",
			common.HexToAddress(testStableUSDC).Hex():   "stableDebtBasUSDC",
			common.HexToAddress(testVariableUSDC).Hex(): "variableDebtBasUSDC",
			common.HexToAddress(testUSDC).Hex():         "USDC",
			common.HexToAddress(testVaultUSDC).Hex():    "waBasUSDC",
			common.HexToAddress(testRewardAAVE).Hex():   "AAVE",
		}[call.Target.Hex()]
		return packResult(erc20ABI.Methods["symbol"].Outputs.Pack(symbol))
	case bytes.Equal(method, erc20ABI.Methods["decimals"].ID):
		decimals := map[string]uint8{
			common.HexToAddress(testAUSDC).Hex():      6,
			common.HexToAddress(testUSDC).Hex():       6,
			common.HexToAddress(testVaultUSDC).Hex():  6,
			common.HexToAddress(testRewardAAVE).Hex(): 18,
		}[call.Target.Hex()]
		return packResult(erc20ABI.Methods["decimals"].Outputs.Pack(decimals))
	case bytes.Equal(method, poolABI.Methods["getUserAccountData"].ID):
		return packResult(poolABI.Methods["getUserAccountData"].Outputs.Pack(
			big.NewInt(1500000),
			big.NewInt(250000),
			big.NewInt(1000000),
			big.NewInt(8250),
			big.NewInt(8000),
			big.NewInt(6_000_000_000_000_000_000),
		))
	case bytes.Equal(method, dataProviderABI.Methods["getUserReserveData"].ID):
		return packResult(dataProviderABI.Methods["getUserReserveData"].Outputs.Pack(
			big.NewInt(1500000),
			big.NewInt(0),
			big.NewInt(250000),
			big.NewInt(0),
			big.NewInt(240000),
			big.NewInt(0),
			big.NewInt(30000000000000000),
			big.NewInt(0),
			true,
		))
	case bytes.Equal(method, stataFactoryABI.Methods["getStataTokens"].ID):
		return packResult(stataFactoryABI.Methods["getStataTokens"].Outputs.Pack([]common.Address{
			common.HexToAddress(testVaultUSDC),
		}))
	case bytes.Equal(method, stataFactoryABI.Methods["getStaticATokens"].ID):
		return packResult(stataFactoryABI.Methods["getStaticATokens"].Outputs.Pack([]common.Address{}))
	case bytes.Equal(method, yieldVaultABI.Methods["asset"].ID):
		return packResult(yieldVaultABI.Methods["asset"].Outputs.Pack(common.HexToAddress(testUSDC)))
	case bytes.Equal(method, yieldVaultABI.Methods["aToken"].ID):
		return packResult(yieldVaultABI.Methods["aToken"].Outputs.Pack(common.HexToAddress(testAUSDC)))
	case bytes.Equal(method, yieldVaultABI.Methods["rewardTokens"].ID):
		return packResult(yieldVaultABI.Methods["rewardTokens"].Outputs.Pack([]common.Address{
			common.HexToAddress(testRewardAAVE),
		}))
	case bytes.Equal(method, yieldVaultABI.Methods["balanceOf"].ID):
		return packResult(yieldVaultABI.Methods["balanceOf"].Outputs.Pack(big.NewInt(1400000)))
	case bytes.Equal(method, yieldVaultABI.Methods["previewRedeem"].ID):
		return packResult(yieldVaultABI.Methods["previewRedeem"].Outputs.Pack(big.NewInt(1600000)))
	case bytes.Equal(method, yieldVaultABI.Methods["maxWithdraw"].ID):
		return packResult(yieldVaultABI.Methods["maxWithdraw"].Outputs.Pack(big.NewInt(1600000)))
	case bytes.Equal(method, yieldVaultABI.Methods["maxRedeem"].ID):
		return packResult(yieldVaultABI.Methods["maxRedeem"].Outputs.Pack(big.NewInt(1400000)))
	case bytes.Equal(method, yieldVaultABI.Methods["getClaimableRewards"].ID):
		return packResult(yieldVaultABI.Methods["getClaimableRewards"].Outputs.Pack(big.NewInt(20_000_000_000_000_000)))
	default:
		r.t.Fatalf("unexpected call target=%s data=%x", call.Target.Hex(), call.CallData)
		return evm.Result{}, nil
	}
}

func packResult(data []byte, err error) (evm.Result, error) {
	if err != nil {
		return evm.Result{}, err
	}
	return evm.Result{Success: true, ReturnData: data}, nil
}

func TestDescriptor(t *testing.T) {
	t.Parallel()

	descriptor := New().Descriptor()
	if descriptor.ID != ProtocolID || !descriptor.SupportsChain(chain.BaseChainID) {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
}

func TestSyncerSyncsLendingReserves(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newAaveTestStore(t)
	result, err := (Syncer{multicallFactory: fakeFactory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Store: store,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Items != 2 {
		t.Fatalf("expected 2 metadata items, got %d", result.Items)
	}
	steps, ok := result.Details["discovery"].([]syncDiscoveryStep)
	if !ok || len(steps) == 0 {
		t.Fatalf("expected discovery details, got %#v", result.Details)
	}
	if steps[1].Step != "discover lending reserves" || steps[1].Calls != 1 || steps[1].Items != 1 {
		t.Fatalf("unexpected reserve discovery step: %#v", steps[1])
	}
	if steps[5].Step != "load vault basics" || steps[5].Calls != 3 || steps[5].Items != 1 {
		t.Fatalf("unexpected vault basics step: %#v", steps[5])
	}

	var reserves []LendingReserve
	if _, err := store.Get(context.Background(), cache.Key{
		ChainID:   chain.BaseChainID,
		Protocol:  ProtocolID,
		Namespace: lendingReservesNamespace,
	}, &reserves); err != nil {
		t.Fatalf("read reserves: %v", err)
	}
	if len(reserves) != 1 {
		t.Fatalf("expected 1 reserve, got %d", len(reserves))
	}
	reserve := reserves[0]
	if reserve.MarketID != "base-core" || reserve.Asset.Symbol != "USDC" || reserve.AToken.Symbol != "aBasUSDC" {
		t.Fatalf("unexpected reserve: %#v", reserve)
	}
	if !reserve.BorrowingEnabled || !reserve.UsageAsCollateralEnabled {
		t.Fatalf("expected reserve flags to be synced: %#v", reserve)
	}

	var vaults []YieldVault
	if _, err := store.Get(context.Background(), cache.Key{
		ChainID:   chain.BaseChainID,
		Protocol:  ProtocolID,
		Namespace: yieldVaultsNamespace,
	}, &vaults); err != nil {
		t.Fatalf("read yield vaults: %v", err)
	}
	if len(vaults) != 1 {
		t.Fatalf("expected 1 yield vault, got %d", len(vaults))
	}
	vault := vaults[0]
	if vault.MarketID != "base-core" || vault.VaultToken.Symbol != "waBasUSDC" || vault.Asset.Symbol != "USDC" || vault.AToken.Symbol != "aBasUSDC" {
		t.Fatalf("unexpected yield vault: %#v", vault)
	}
	if len(vault.RewardTokens) != 1 || vault.RewardTokens[0].Symbol != "AAVE" {
		t.Fatalf("unexpected reward tokens: %#v", vault.RewardTokens)
	}
}

func TestFetcherRequiresSyncedMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newAaveTestStore(t)
	_, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err == nil {
		t.Fatal("expected missing metadata error")
	}
	if !strings.Contains(err.Error(), "sync-metadata") {
		t.Fatalf("expected sync hint, got %v", err)
	}
}

func TestFetcherRejectsExpiredMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newAaveTestStore(t)
	syncAaveTestMetadata(t, runner, store)

	var markets []MarketConfig
	if _, err := store.Get(context.Background(), cache.Key{
		ChainID:   chain.BaseChainID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, &markets); err != nil {
		t.Fatalf("read markets: %v", err)
	}
	if err := store.Set(context.Background(), cache.Key{
		ChainID:   chain.BaseChainID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, markets, core.MetadataInfo{
		Version:   lendingMetadataVersion,
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("write stale markets: %v", err)
	}

	_, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain:          core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner:          testOwner,
		Store:          store,
		MetadataMaxAge: time.Hour,
	})
	if err == nil {
		t.Fatal("expected stale metadata error")
	}
	if !strings.Contains(err.Error(), "older than") {
		t.Fatalf("expected stale metadata message, got %v", err)
	}
}

func TestFetcherFetchesLendingAndYieldPositions(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newAaveTestStore(t)
	syncAaveTestMetadata(t, runner, store)
	positions, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	position := findPositionByType(t, positions, core.PositionTypeLending)
	if position.Protocol != ProtocolID || position.Type != core.PositionTypeLending || position.DisplayName != "Aave V3 Base Core Lending" {
		t.Fatalf("unexpected position identity: %#v", position)
	}
	if len(position.Shares) != 1 || position.Shares[0].Token.Symbol != "aBasUSDC" || position.Shares[0].Formatted != "1.5" {
		t.Fatalf("unexpected shares: %#v", position.Shares)
	}
	if len(position.Underlying) != 1 || position.Underlying[0].Token.Symbol != "USDC" || position.Underlying[0].Formatted != "1.5" {
		t.Fatalf("unexpected underlying: %#v", position.Underlying)
	}
	if len(position.Debt) != 1 || position.Debt[0].Token.Symbol != "USDC" || position.Debt[0].Formatted != "0.25" {
		t.Fatalf("unexpected debt: %#v", position.Debt)
	}
	if position.Extra["marketId"] != "base-core" || position.Extra["healthFactorFormatted"] != "6" {
		t.Fatalf("unexpected extra: %#v", position.Extra)
	}

	yieldPosition := findPositionByType(t, positions, core.PositionTypeYield)
	if yieldPosition.DisplayName != "Aave V3 Base Core Yield USDC" {
		t.Fatalf("unexpected yield display name: %#v", yieldPosition)
	}
	if len(yieldPosition.Shares) != 1 || yieldPosition.Shares[0].Token.Symbol != "waBasUSDC" || yieldPosition.Shares[0].Formatted != "1.4" {
		t.Fatalf("unexpected yield shares: %#v", yieldPosition.Shares)
	}
	if len(yieldPosition.Underlying) != 1 || yieldPosition.Underlying[0].Token.Symbol != "USDC" || yieldPosition.Underlying[0].Formatted != "1.6" {
		t.Fatalf("unexpected yield underlying: %#v", yieldPosition.Underlying)
	}
	rewards, ok := yieldPosition.Extra["claimableRewards"].([]map[string]any)
	if !ok || len(rewards) != 1 || rewards[0]["formatted"] != "0.02" {
		t.Fatalf("unexpected yield rewards: %#v", yieldPosition.Extra["claimableRewards"])
	}
}

func syncAaveTestMetadata(t *testing.T, runner MulticallRunner, store cache.Store) {
	t.Helper()

	if _, err := (Syncer{multicallFactory: fakeFactory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Store: store,
	}); err != nil {
		t.Fatalf("sync metadata: %v", err)
	}
}

func findPositionByType(t *testing.T, positions []core.Position, positionType core.PositionType) core.Position {
	t.Helper()

	for _, position := range positions {
		if position.Type == positionType {
			return position
		}
	}
	t.Fatalf("position type %s not found in %#v", positionType, positions)
	return core.Position{}
}

func fakeFactory(runner MulticallRunner) MulticallFactory {
	return func(core.Chain) (MulticallRunner, error) {
		return runner, nil
	}
}

func newAaveTestStore(t *testing.T) *cache.SQLiteStore {
	t.Helper()

	store, err := cache.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
