package compoundv3

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
	testOwner = "0x0000000000000000000000000000000000000001"
	testUSDC  = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	testWETH  = "0x4200000000000000000000000000000000000006"
	testCOMP  = "0x9e1028F5F1D5eDE59748FFceE5532509976840E0"
	testFeed  = "0x00000000000000000000000000000000000000f1"
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
	case bytes.Equal(method, cometABI.Methods["name"].ID):
		return packResult(cometABI.Methods["name"].Outputs.Pack("Compound USDC"))
	case bytes.Equal(method, cometABI.Methods["symbol"].ID), bytes.Equal(method, erc20ABI.Methods["symbol"].ID):
		if isTestComet(call.Target) {
			return packResult(cometABI.Methods["symbol"].Outputs.Pack("cUSDCv3"))
		}
		return packResult(erc20ABI.Methods["symbol"].Outputs.Pack(testSymbol(call.Target)))
	case bytes.Equal(method, cometABI.Methods["decimals"].ID), bytes.Equal(method, erc20ABI.Methods["decimals"].ID):
		if isTestComet(call.Target) {
			return packResult(cometABI.Methods["decimals"].Outputs.Pack(uint8(6)))
		}
		return packResult(erc20ABI.Methods["decimals"].Outputs.Pack(testDecimals(call.Target)))
	case bytes.Equal(method, cometABI.Methods["baseToken"].ID):
		return packResult(cometABI.Methods["baseToken"].Outputs.Pack(common.HexToAddress(testUSDC)))
	case bytes.Equal(method, cometABI.Methods["baseTokenPriceFeed"].ID):
		return packResult(cometABI.Methods["baseTokenPriceFeed"].Outputs.Pack(common.HexToAddress(testFeed)))
	case bytes.Equal(method, cometABI.Methods["baseScale"].ID):
		return packResult(cometABI.Methods["baseScale"].Outputs.Pack(uint64(1_000_000)))
	case bytes.Equal(method, cometABI.Methods["numAssets"].ID):
		return packResult(cometABI.Methods["numAssets"].Outputs.Pack(uint8(1)))
	case bytes.Equal(method, cometABI.Methods["getAssetInfo"].ID):
		return packResult(cometABI.Methods["getAssetInfo"].Outputs.Pack(assetInfoOutput{
			Offset:                    0,
			Asset:                     common.HexToAddress(testWETH),
			PriceFeed:                 common.HexToAddress(testFeed),
			Scale:                     1_000_000_000_000_000_000,
			BorrowCollateralFactor:    800_000_000_000_000_000,
			LiquidateCollateralFactor: 900_000_000_000_000_000,
			LiquidationFactor:         950_000_000_000_000_000,
			SupplyCap:                 big.NewInt(0).Mul(big.NewInt(1000), big.NewInt(1_000_000_000_000_000_000)),
		}))
	case bytes.Equal(method, cometABI.Methods["balanceOf"].ID):
		return packResult(cometABI.Methods["balanceOf"].Outputs.Pack(big.NewInt(1_500_000)))
	case bytes.Equal(method, cometABI.Methods["borrowBalanceOf"].ID):
		return packResult(cometABI.Methods["borrowBalanceOf"].Outputs.Pack(big.NewInt(250_000)))
	case bytes.Equal(method, cometABI.Methods["collateralBalanceOf"].ID):
		return packResult(cometABI.Methods["collateralBalanceOf"].Outputs.Pack(big.NewInt(2_000_000_000_000_000_000)))
	case bytes.Equal(method, cometABI.Methods["isBorrowCollateralized"].ID):
		return packResult(cometABI.Methods["isBorrowCollateralized"].Outputs.Pack(true))
	case bytes.Equal(method, cometABI.Methods["isLiquidatable"].ID):
		return packResult(cometABI.Methods["isLiquidatable"].Outputs.Pack(false))
	case bytes.Equal(method, cometABI.Methods["getPrice"].ID):
		return packResult(cometABI.Methods["getPrice"].Outputs.Pack(big.NewInt(100_000_000)))
	case bytes.Equal(method, rewardsABI.Methods["rewardConfig"].ID):
		return packResult(rewardsABI.Methods["rewardConfig"].Outputs.Pack(
			common.HexToAddress(testCOMP),
			uint64(1),
			true,
			big.NewInt(1_000_000_000_000_000_000),
		))
	case bytes.Equal(method, rewardsABI.Methods["getRewardOwed"].ID):
		return packResult(rewardsABI.Methods["getRewardOwed"].Outputs.Pack(rewardOwedOutput{
			Token: common.HexToAddress(testCOMP),
			Owed:  big.NewInt(42_000_000_000_000_000),
		}))
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

func TestSyncerSyncsMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	result, err := (Syncer{multicallFactory: fakeFactory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Store: store,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Items != 15 {
		t.Fatalf("expected 15 metadata items, got %d", result.Items)
	}

	var markets []Market
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: marketsNamespace}, &markets); err != nil {
		t.Fatalf("read markets: %v", err)
	}
	if len(markets) != 5 || markets[0].BaseToken.Symbol != "USDC" || markets[0].CometToken.Symbol != "cUSDCv3" {
		t.Fatalf("unexpected markets: %#v", markets)
	}
	var collaterals []CollateralAsset
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: collateralAssetsNamespace}, &collaterals); err != nil {
		t.Fatalf("read collaterals: %v", err)
	}
	if len(collaterals) != 5 || collaterals[0].Asset.Symbol != "WETH" || collaterals[0].BorrowCollateralFactor == "" {
		t.Fatalf("unexpected collaterals: %#v", collaterals)
	}
	var rewards []RewardConfig
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: rewardConfigsNamespace}, &rewards); err != nil {
		t.Fatalf("read rewards: %v", err)
	}
	if len(rewards) != 5 || !rewards[0].Supported || rewards[0].RewardToken.Symbol != "COMP" {
		t.Fatalf("unexpected rewards: %#v", rewards)
	}
}

func TestFetcherRequiresSyncedMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	_, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err == nil || !strings.Contains(err.Error(), "sync-metadata") {
		t.Fatalf("expected sync hint, got %v", err)
	}
}

func TestFetcherRejectsExpiredMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	syncTestMetadata(t, runner, store)
	var markets []Market
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: marketsNamespace}, &markets); err != nil {
		t.Fatalf("read markets: %v", err)
	}
	if err := store.Set(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: marketsNamespace}, markets, core.MetadataInfo{
		Version:   metadataVersion,
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
	if err == nil || !strings.Contains(err.Error(), "older than") {
		t.Fatalf("expected stale metadata error, got %v", err)
	}
}

func TestFetcherFetchesYieldAndLendingPositions(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	syncTestMetadata(t, runner, store)

	positions, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(positions) != 15 {
		t.Fatalf("expected 15 positions, got %d", len(positions))
	}

	yield := findPosition(t, positions, "base-usdc", "yield")
	if yield.Type != core.PositionTypeYield {
		t.Fatalf("unexpected yield position type: %#v", yield)
	}
	if len(yield.Shares) != 1 || yield.Shares[0].Token.Symbol != "cUSDCv3" || yield.Shares[0].Formatted != "1.5" {
		t.Fatalf("unexpected yield shares: %#v", yield)
	}
	if len(yield.Underlying) != 1 || yield.Underlying[0].Token.Symbol != "USDC" || yield.Underlying[0].Formatted != "1.5" {
		t.Fatalf("unexpected yield underlying: %#v", yield)
	}
	if yield.Extra["debankPoolId"] != "0xb125e6687d4313864e53df431d5425969c15eb2f:yield" {
		t.Fatalf("unexpected yield extra: %#v", yield.Extra)
	}

	lending := findPosition(t, positions, "base-usdc", "lending")
	if len(lending.Underlying) != 1 || lending.Underlying[0].Token.Symbol != "WETH" || lending.Underlying[0].Formatted != "2" {
		t.Fatalf("unexpected lending collateral: %#v", lending)
	}
	if len(lending.Debt) != 1 || lending.Debt[0].Token.Symbol != "USDC" || lending.Debt[0].Formatted != "0.25" {
		t.Fatalf("unexpected lending debt: %#v", lending.Debt)
	}
	if lending.Extra["isLiquidatable"] != false || lending.Extra["liquidationHealthFormatted"] == "" {
		t.Fatalf("unexpected lending extra: %#v", lending.Extra)
	}
	if _, ok := lending.Extra["claimableRewards"]; ok {
		t.Fatalf("lending position should not carry rewards: %#v", lending.Extra)
	}

	reward := findPosition(t, positions, "base-usdc", "reward")
	if reward.Type != core.PositionTypeReward {
		t.Fatalf("unexpected reward position type: %#v", reward)
	}
	if len(reward.Underlying) != 1 || reward.Underlying[0].Token.Symbol != "COMP" || reward.Underlying[0].Formatted != "0.042" {
		t.Fatalf("unexpected reward underlying: %#v", reward)
	}
	rewards, ok := reward.Extra["claimableRewards"].([]map[string]any)
	if !ok || len(rewards) != 1 || rewards[0]["raw"] != "42000000000000000" || rewards[0]["formatted"] != "0.042" {
		t.Fatalf("unexpected rewards: %#v", reward.Extra["claimableRewards"])
	}
	rewardToken, ok := rewards[0]["token"].(core.Token)
	if !ok || rewardToken.Symbol != "COMP" {
		t.Fatalf("unexpected reward token: %#v", rewards[0]["token"])
	}
}

func syncTestMetadata(t *testing.T, runner MulticallRunner, store cache.Store) {
	t.Helper()

	if _, err := (Syncer{multicallFactory: fakeFactory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Store: store,
	}); err != nil {
		t.Fatalf("sync metadata: %v", err)
	}
}

func findPosition(t *testing.T, positions []core.Position, marketID string, strategy string) core.Position {
	t.Helper()

	for _, position := range positions {
		if position.Extra["marketId"] == marketID && position.Extra["strategy"] == strategy {
			return position
		}
	}
	t.Fatalf("position %s/%s not found in %#v", marketID, strategy, positions)
	return core.Position{}
}

func fakeFactory(runner MulticallRunner) MulticallFactory {
	return func(core.Chain) (MulticallRunner, error) {
		return runner, nil
	}
}

func newTestStore(t *testing.T) *cache.SQLiteStore {
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

func isTestComet(address common.Address) bool {
	for _, config := range marketConfigs(chain.BaseChainID) {
		if address == common.HexToAddress(config.Comet) {
			return true
		}
	}
	return false
}

func testSymbol(address common.Address) string {
	switch address {
	case common.HexToAddress(testWETH):
		return "WETH"
	case common.HexToAddress(testCOMP):
		return "COMP"
	default:
		return "USDC"
	}
}

func testDecimals(address common.Address) uint8 {
	switch address {
	case common.HexToAddress(testWETH), common.HexToAddress(testCOMP):
		return 18
	default:
		return 6
	}
}
