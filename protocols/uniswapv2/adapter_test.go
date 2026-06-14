package uniswapv2

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
	testOwner   = "0x0000000000000000000000000000000000000001"
	testFactory = "0x00000000000000000000000000000000000000F0"
	testRouter  = "0x00000000000000000000000000000000000000F1"
	testPair    = "0x00000000000000000000000000000000000000AA"
	testStaking = "0x00000000000000000000000000000000000000BB"
	testWETH    = "0x00000000000000000000000000000000000000C0"
	testUSDC    = "0x00000000000000000000000000000000000000C1"
	testUNI     = "0x00000000000000000000000000000000000000C2"
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
	case bytes.Equal(method, factoryABI.Methods["allPairsLength"].ID):
		return packResult(factoryABI.Methods["allPairsLength"].Outputs.Pack(big.NewInt(1)))
	case bytes.Equal(method, factoryABI.Methods["allPairs"].ID):
		return packResult(factoryABI.Methods["allPairs"].Outputs.Pack(common.HexToAddress(testPair)))
	case bytes.Equal(method, factoryABI.Methods["getPair"].ID):
		return packResult(factoryABI.Methods["getPair"].Outputs.Pack(common.HexToAddress(testPair)))
	case bytes.Equal(method, factoryABI.Methods["feeTo"].ID):
		return packResult(factoryABI.Methods["feeTo"].Outputs.Pack(common.Address{}))
	case bytes.Equal(method, pairABI.Methods["token0"].ID):
		return packResult(pairABI.Methods["token0"].Outputs.Pack(common.HexToAddress(testWETH)))
	case bytes.Equal(method, pairABI.Methods["token1"].ID):
		return packResult(pairABI.Methods["token1"].Outputs.Pack(common.HexToAddress(testUSDC)))
	case bytes.Equal(method, pairABI.Methods["symbol"].ID), bytes.Equal(method, erc20StringABI.Methods["symbol"].ID):
		if call.Target == common.HexToAddress(testPair) {
			return packResult(pairABI.Methods["symbol"].Outputs.Pack("UNI-V2"))
		}
		return packResult(erc20StringABI.Methods["symbol"].Outputs.Pack(testSymbol(call.Target)))
	case bytes.Equal(method, pairABI.Methods["decimals"].ID), bytes.Equal(method, erc20StringABI.Methods["decimals"].ID):
		if call.Target == common.HexToAddress(testPair) {
			return packResult(pairABI.Methods["decimals"].Outputs.Pack(uint8(18)))
		}
		return packResult(erc20StringABI.Methods["decimals"].Outputs.Pack(testDecimals(call.Target)))
	case bytes.Equal(method, stakingABI.Methods["stakingToken"].ID):
		return packResult(stakingABI.Methods["stakingToken"].Outputs.Pack(common.HexToAddress(testPair)))
	case bytes.Equal(method, stakingABI.Methods["rewardsToken"].ID):
		return packResult(stakingABI.Methods["rewardsToken"].Outputs.Pack(common.HexToAddress(testUNI)))
	case bytes.Equal(method, stakingABI.Methods["periodFinish"].ID):
		return packResult(stakingABI.Methods["periodFinish"].Outputs.Pack(big.NewInt(1_605_568_800)))
	case bytes.Equal(method, pairABI.Methods["getReserves"].ID):
		return packResult(pairABI.Methods["getReserves"].Outputs.Pack(
			amount(1000, 18),
			amount(2_000_000, 6),
			uint32(12345),
		))
	case bytes.Equal(method, pairABI.Methods["totalSupply"].ID):
		return packResult(pairABI.Methods["totalSupply"].Outputs.Pack(amount(100, 18)))
	case bytes.Equal(method, pairABI.Methods["kLast"].ID):
		return packResult(pairABI.Methods["kLast"].Outputs.Pack(big.NewInt(0)))
	case bytes.Equal(method, pairABI.Methods["balanceOf"].ID):
		switch call.Target {
		case common.HexToAddress(testPair):
			return packResult(pairABI.Methods["balanceOf"].Outputs.Pack(amount(1, 18)))
		case common.HexToAddress(testStaking):
			return packResult(stakingABI.Methods["balanceOf"].Outputs.Pack(amount(2, 18)))
		case common.HexToAddress(testWETH):
			return packResult(erc20StringABI.Methods["balanceOf"].Outputs.Pack(amount(1000, 18)))
		case common.HexToAddress(testUSDC):
			return packResult(erc20StringABI.Methods["balanceOf"].Outputs.Pack(amount(2_000_000, 6)))
		default:
			r.t.Fatalf("unexpected balanceOf target=%s", call.Target.Hex())
		}
	case bytes.Equal(method, stakingABI.Methods["earned"].ID):
		return packResult(stakingABI.Methods["earned"].Outputs.Pack(amount(42, 18)))
	default:
		r.t.Fatalf("unexpected call target=%s data=%x", call.Target.Hex(), call.CallData)
	}
	return evm.Result{}, nil
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

func TestSyncerSyncsPairsMetadata(t *testing.T) {
	t.Setenv("DPR_UNISWAP_V2_DISCOVERY_MODE", "full")
	t.Setenv("DPR_UNISWAP_V2_MAX_PAIRS", "")
	t.Setenv("DPR_UNISWAP_V2_SEED_PAIRS", "")

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
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

	var pairs []Pair
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: pairsNamespace}, &pairs); err != nil {
		t.Fatalf("read pairs: %v", err)
	}
	if len(pairs) != 1 || pairs[0].Token0.Symbol != "WETH" || pairs[0].Token1.Symbol != "USDC" {
		t.Fatalf("unexpected pairs: %#v", pairs)
	}
	var farming []FarmingPool
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: farmingPoolsNamespace}, &farming); err != nil {
		t.Fatalf("read farming pools: %v", err)
	}
	if len(farming) != 0 {
		t.Fatalf("expected no base farming pools, got %#v", farming)
	}
}

func TestSyncerDefaultsToFullDiscovery(t *testing.T) {
	t.Setenv("DPR_UNISWAP_V2_DISCOVERY_MODE", "")
	t.Setenv("DPR_UNISWAP_V2_MAX_PAIRS", "")
	t.Setenv("DPR_UNISWAP_V2_SEED_PAIRS", "")

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	result, err := (Syncer{multicallFactory: fakeFactory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Store: store,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Items != 2 {
		t.Fatalf("expected market and one pair metadata item, got %d", result.Items)
	}
	foundAllPairsLength := false
	for _, batch := range runner.calls {
		for _, call := range batch {
			if bytes.Equal(call.CallData[:4], factoryABI.Methods["allPairsLength"].ID) {
				foundAllPairsLength = true
			}
		}
	}
	if !foundAllPairsLength {
		t.Fatal("default discovery should use full factory allPairsLength calls")
	}

	var pairs []Pair
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: pairsNamespace}, &pairs); err != nil {
		t.Fatalf("read pairs: %v", err)
	}
	if len(pairs) != 1 || pairs[0].Token0.Symbol != "WETH" || pairs[0].Token1.Symbol != "USDC" {
		t.Fatalf("unexpected default pairs: %#v", pairs)
	}
}

func TestDiscoveryOptionsOwnerForcesUserMode(t *testing.T) {
	t.Setenv("DPR_UNISWAP_V2_DISCOVERY_MODE", "")
	t.Setenv("DPR_UNISWAP_V2_MAX_PAIRS", "100")
	t.Setenv("DPR_UNISWAP_V2_SEED_PAIRS", "")

	options, err := discoveryOptionsFromEnv(testOwner)
	if err != nil {
		t.Fatalf("discovery options: %v", err)
	}
	if options.mode != discoveryModeUser || !options.hasOwner || options.owner != common.HexToAddress(testOwner) {
		t.Fatalf("unexpected user options: %#v", options)
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
	syncTestMetadata(t, store, time.Now().Add(-2*time.Hour))
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

func TestFetcherFetchesLiquidityAndFarmingPositions(t *testing.T) {
	t.Parallel()

	runner := &fakeMulticallRunner{t: t}
	store := newTestStore(t)
	syncTestMetadata(t, store, time.Now())

	positions, err := (Fetcher{multicallFactory: fakeFactory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d: %#v", len(positions), positions)
	}

	direct := findPosition(t, positions, "liquidity")
	if direct.Type != core.PositionTypeLiquidity || len(direct.Shares) != 1 || direct.Shares[0].Formatted != "1" {
		t.Fatalf("unexpected direct liquidity position: %#v", direct)
	}
	if len(direct.Underlying) != 2 || direct.Underlying[0].Formatted != "10" || direct.Underlying[1].Formatted != "20000" {
		t.Fatalf("unexpected direct underlying: %#v", direct.Underlying)
	}

	farming := findPosition(t, positions, "farming")
	if farming.Type != core.PositionTypeLiquidity || len(farming.Shares) != 1 || farming.Shares[0].Formatted != "2" {
		t.Fatalf("unexpected farming liquidity position: %#v", farming)
	}
	if len(farming.Underlying) != 2 || farming.Underlying[0].Formatted != "20" || farming.Underlying[1].Formatted != "40000" {
		t.Fatalf("unexpected farming underlying: %#v", farming.Underlying)
	}

	reward := findPosition(t, positions, "farming-reward")
	if reward.Type != core.PositionTypeReward || len(reward.Underlying) != 1 || reward.Underlying[0].Formatted != "42" {
		t.Fatalf("unexpected reward position: %#v", reward)
	}
}

func TestPendingProtocolLiquidity(t *testing.T) {
	t.Parallel()

	liquidity := pendingProtocolLiquidity(
		amount(100, 18),
		amount(1000, 18),
		amount(1000, 18),
		new(big.Int).Mul(amount(900, 18), amount(900, 18)),
		true,
	)
	if liquidity.Sign() <= 0 {
		t.Fatalf("expected positive pending protocol liquidity, got %s", liquidity)
	}
	if disabled := pendingProtocolLiquidity(amount(100, 18), amount(1000, 18), amount(1000, 18), liquidity, false); disabled.Sign() != 0 {
		t.Fatalf("expected disabled fee to produce zero liquidity, got %s", disabled)
	}
}

func syncTestMetadata(t *testing.T, store cache.Store, updatedAt time.Time) {
	t.Helper()

	markets := []Market{
		{
			ID:               "base-core",
			Name:             "UniswapV2Base",
			DisplayName:      "Uniswap V2 Base",
			ChainID:          chain.BaseChainID,
			Factory:          testFactory,
			Router:           testRouter,
			DebankProtocolID: "base_uniswap2",
		},
	}
	pair := Pair{
		MarketID: "base-core",
		Address:  common.HexToAddress(testPair).Hex(),
		Index:    0,
		PairToken: core.Token{
			ChainID:  chain.BaseChainID,
			Address:  common.HexToAddress(testPair).Hex(),
			Symbol:   "UNI-V2",
			Decimals: 18,
		},
		Token0: core.Token{
			ChainID:  chain.BaseChainID,
			Address:  common.HexToAddress(testWETH).Hex(),
			Symbol:   "WETH",
			Decimals: 18,
		},
		Token1: core.Token{
			ChainID:  chain.BaseChainID,
			Address:  common.HexToAddress(testUSDC).Hex(),
			Symbol:   "USDC",
			Decimals: 6,
		},
		DebankPoolID: strings.ToLower(common.HexToAddress(testPair).Hex()),
	}
	farming := []FarmingPool{
		{
			ID:              "base-test-farming",
			MarketID:        "base-core",
			Name:            "UniswapV2TestRewards",
			DisplayName:     "Uniswap V2 WETH / USDC Farming",
			StakingContract: common.HexToAddress(testStaking).Hex(),
			Pair:            pair.Address,
			PairToken:       pair.PairToken,
			Token0:          pair.Token0,
			Token1:          pair.Token1,
			RewardToken: core.Token{
				ChainID:  chain.BaseChainID,
				Address:  common.HexToAddress(testUNI).Hex(),
				Symbol:   "UNI",
				Decimals: 18,
			},
			PeriodFinish: "1605568800",
			DebankPoolID: strings.ToLower(common.HexToAddress(testStaking).Hex()),
		},
	}
	for _, item := range []struct {
		namespace string
		value     any
	}{
		{marketsNamespace, markets},
		{pairsNamespace, []Pair{pair}},
		{farmingPoolsNamespace, farming},
	} {
		if err := store.Set(context.Background(), cache.Key{
			ChainID:   chain.BaseChainID,
			Protocol:  ProtocolID,
			Namespace: item.namespace,
		}, item.value, core.MetadataInfo{
			Version:   metadataVersion,
			UpdatedAt: updatedAt,
			Source:    "test",
		}); err != nil {
			t.Fatalf("write %s metadata: %v", item.namespace, err)
		}
	}
}

func findPosition(t *testing.T, positions []core.Position, strategy string) core.Position {
	t.Helper()
	for _, position := range positions {
		if position.Extra["strategy"] == strategy {
			return position
		}
	}
	t.Fatalf("position with strategy %q not found: %#v", strategy, positions)
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
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testSymbol(address common.Address) string {
	switch address {
	case common.HexToAddress(testWETH):
		return "WETH"
	case common.HexToAddress(testUSDC):
		return "USDC"
	case common.HexToAddress(testUNI):
		return "UNI"
	default:
		return "UNKNOWN"
	}
}

func testDecimals(address common.Address) uint8 {
	switch address {
	case common.HexToAddress(testUSDC):
		return 6
	default:
		return 18
	}
}

func amount(value int64, decimals uint8) *big.Int {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Int).Mul(big.NewInt(value), scale)
}
