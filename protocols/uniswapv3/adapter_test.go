package uniswapv3

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
	testOwner           = "0x0000000000000000000000000000000000000001"
	testFactory         = "0x00000000000000000000000000000000000000F0"
	testPositionManager = "0x00000000000000000000000000000000000000F1"
	testPool            = "0x00000000000000000000000000000000000000AA"
	testWETH            = "0x00000000000000000000000000000000000000C0"
	testUSDC            = "0x00000000000000000000000000000000000000C1"
)

var (
	testLiquidity = big.NewInt(1000)
	testTokenID   = big.NewInt(77)
)

type fakeV3MulticallRunner struct {
	t *testing.T
}

func (r fakeV3MulticallRunner) Aggregate3(_ context.Context, calls []evm.Call) ([]evm.Result, error) {
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

func (r fakeV3MulticallRunner) resultFor(call evm.Call) (evm.Result, error) {
	method := call.CallData[:4]
	switch {
	case bytes.Equal(method, positionManagerABI.Methods["balanceOf"].ID):
		return packV3Result(positionManagerABI.Methods["balanceOf"].Outputs.Pack(big.NewInt(1)))
	case bytes.Equal(method, positionManagerABI.Methods["tokenOfOwnerByIndex"].ID):
		return packV3Result(positionManagerABI.Methods["tokenOfOwnerByIndex"].Outputs.Pack(testTokenID))
	case bytes.Equal(method, positionManagerABI.Methods["positions"].ID):
		return packV3Result(positionManagerABI.Methods["positions"].Outputs.Pack(
			big.NewInt(0),
			common.Address{},
			common.HexToAddress(testWETH),
			common.HexToAddress(testUSDC),
			big.NewInt(500),
			big.NewInt(-600),
			big.NewInt(600),
			testLiquidity,
			q128Multiple(6),
			q128Multiple(4),
			big.NewInt(100),
			big.NewInt(200),
		))
	case bytes.Equal(method, factoryABI.Methods["getPool"].ID):
		return packV3Result(factoryABI.Methods["getPool"].Outputs.Pack(common.HexToAddress(testPool)))
	case bytes.Equal(method, poolABI.Methods["tickSpacing"].ID):
		return packV3Result(poolABI.Methods["tickSpacing"].Outputs.Pack(big.NewInt(10)))
	case bytes.Equal(method, erc20StringABI.Methods["symbol"].ID):
		return packV3Result(erc20StringABI.Methods["symbol"].Outputs.Pack(testSymbol(call.Target)))
	case bytes.Equal(method, erc20StringABI.Methods["decimals"].ID):
		return packV3Result(erc20StringABI.Methods["decimals"].Outputs.Pack(uint8(0)))
	case bytes.Equal(method, poolABI.Methods["slot0"].ID):
		sqrtPrice, err := sqrtRatioAtTick(0)
		if err != nil {
			return evm.Result{}, err
		}
		return packV3Result(poolABI.Methods["slot0"].Outputs.Pack(
			sqrtPrice,
			big.NewInt(0),
			uint16(0),
			uint16(0),
			uint16(0),
			uint8(0),
			true,
		))
	case bytes.Equal(method, poolABI.Methods["liquidity"].ID):
		return packV3Result(poolABI.Methods["liquidity"].Outputs.Pack(big.NewInt(10_000)))
	case bytes.Equal(method, poolABI.Methods["feeGrowthGlobal0X128"].ID):
		return packV3Result(poolABI.Methods["feeGrowthGlobal0X128"].Outputs.Pack(q128Multiple(12)))
	case bytes.Equal(method, poolABI.Methods["feeGrowthGlobal1X128"].ID):
		return packV3Result(poolABI.Methods["feeGrowthGlobal1X128"].Outputs.Pack(q128Multiple(7)))
	case bytes.Equal(method, poolABI.Methods["ticks"].ID):
		tick, err := unpackTickInput(call.CallData)
		if err != nil {
			return evm.Result{}, err
		}
		fee0 := q128Multiple(3)
		fee1 := q128Multiple(1)
		if tick < 0 {
			fee0 = q128Multiple(1)
			fee1 = q128Multiple(1)
		}
		return packV3Result(poolABI.Methods["ticks"].Outputs.Pack(
			big.NewInt(1000),
			big.NewInt(0),
			fee0,
			fee1,
			big.NewInt(0),
			big.NewInt(0),
			uint32(0),
			true,
		))
	default:
		r.t.Fatalf("unexpected call target=%s data=%x", call.Target.Hex(), call.CallData)
	}
	return evm.Result{}, nil
}

func TestDescriptor(t *testing.T) {
	t.Parallel()

	descriptor := New().Descriptor()
	if descriptor.ID != ProtocolID || !descriptor.SupportsChain(chain.BaseChainID) {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
}

func TestSyncerUserScopedPoolsMetadata(t *testing.T) {
	t.Setenv("DPR_UNISWAP_V3_DISCOVERY_MODE", "")
	t.Setenv("DPR_UNISWAP_V3_MAX_POOLS", "")

	runner := fakeV3MulticallRunner{t: t}
	store := newTestStore(t)
	result, err := (Syncer{multicallFactory: fakeV3Factory(runner)}).Sync(context.Background(), adapter.SyncRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Items != 2 {
		t.Fatalf("expected market and pool metadata, got %d", result.Items)
	}

	var pools []Pool
	if _, err := store.Get(context.Background(), cache.Key{ChainID: chain.BaseChainID, Protocol: ProtocolID, Namespace: poolsNamespace}, &pools); err != nil {
		t.Fatalf("read pools: %v", err)
	}
	if len(pools) != 1 || pools[0].Fee != 500 || pools[0].TickSpacing != 10 || pools[0].Token0.Symbol != "WETH" || pools[0].Token1.Symbol != "USDC" {
		t.Fatalf("unexpected pools: %#v", pools)
	}
}

func TestFetcherRequiresSyncedMetadata(t *testing.T) {
	t.Parallel()

	runner := fakeV3MulticallRunner{t: t}
	store := newTestStore(t)
	_, err := (Fetcher{multicallFactory: fakeV3Factory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err == nil || !strings.Contains(err.Error(), "sync-metadata") {
		t.Fatalf("expected sync hint, got %v", err)
	}
}

func TestFetcherFetchesLiquidityPositionWithRewards(t *testing.T) {
	t.Parallel()

	runner := fakeV3MulticallRunner{t: t}
	store := newTestStore(t)
	syncTestMetadata(t, store, time.Now())

	positions, err := (Fetcher{multicallFactory: fakeV3Factory(runner)}).Fetch(context.Background(), adapter.FetchRequest{
		Chain: core.Chain{ID: chain.BaseChainID, Name: "base"},
		Owner: testOwner,
		Store: store,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d: %#v", len(positions), positions)
	}
	position := positions[0]
	if position.Type != core.PositionTypeLiquidity || position.Extra["strategy"] != "liquidity-pool" {
		t.Fatalf("unexpected position: %#v", position)
	}
	if len(position.Shares) != 1 || position.Shares[0].Token.Symbol != "UNI-V3-POS" || position.Shares[0].Raw != "1" {
		t.Fatalf("unexpected shares: %#v", position.Shares)
	}
	if len(position.Underlying) != 2 || position.Underlying[0].Raw == "0" || position.Underlying[1].Raw == "0" {
		t.Fatalf("expected in-range underlying for both tokens, got %#v", position.Underlying)
	}
	if len(position.Rewards) != 2 || position.Rewards[0].Raw != "2100" || position.Rewards[1].Raw != "1200" {
		t.Fatalf("unexpected rewards: %#v", position.Rewards)
	}
	if position.Extra["rangeStatus"] != "in-range" || position.Extra["feeFormula"] == "" {
		t.Fatalf("unexpected extra: %#v", position.Extra)
	}
}

func TestAmountsForLiquidityRangeStates(t *testing.T) {
	t.Parallel()

	lower, err := sqrtRatioAtTick(-600)
	if err != nil {
		t.Fatalf("lower sqrt: %v", err)
	}
	mid, err := sqrtRatioAtTick(0)
	if err != nil {
		t.Fatalf("mid sqrt: %v", err)
	}
	upper, err := sqrtRatioAtTick(600)
	if err != nil {
		t.Fatalf("upper sqrt: %v", err)
	}
	amount0, amount1 := amountsForLiquidity(lower, lower, upper, testLiquidity)
	if amount0.Sign() <= 0 || amount1.Sign() != 0 {
		t.Fatalf("below range should be token0-only: %s %s", amount0, amount1)
	}
	amount0, amount1 = amountsForLiquidity(mid, lower, upper, testLiquidity)
	if amount0.Sign() <= 0 || amount1.Sign() <= 0 {
		t.Fatalf("in range should have both tokens: %s %s", amount0, amount1)
	}
	amount0, amount1 = amountsForLiquidity(upper, lower, upper, testLiquidity)
	if amount0.Sign() != 0 || amount1.Sign() <= 0 {
		t.Fatalf("above range should be token1-only: %s %s", amount0, amount1)
	}
}

func TestSubUint256Wraps(t *testing.T) {
	t.Parallel()

	got := subUint256(big.NewInt(1), big.NewInt(2))
	want := new(big.Int).Sub(uint256, big.NewInt(1))
	if got.Cmp(want) != 0 {
		t.Fatalf("unexpected wrapped subtraction: got %s want %s", got, want)
	}
}

func syncTestMetadata(t *testing.T, store cache.Store, updatedAt time.Time) {
	t.Helper()

	markets := []Market{
		{
			ID:               "base-core",
			Name:             "UniswapV3Base",
			DisplayName:      "Uniswap V3 Base",
			ChainID:          chain.BaseChainID,
			Factory:          common.HexToAddress(testFactory).Hex(),
			PositionManager:  common.HexToAddress(testPositionManager).Hex(),
			DebankProtocolID: "base_uniswap3",
		},
	}
	pools := []Pool{
		{
			MarketID: "base-core",
			Address:  common.HexToAddress(testPool).Hex(),
			Token0: core.Token{
				ChainID:  chain.BaseChainID,
				Address:  common.HexToAddress(testWETH).Hex(),
				Symbol:   "WETH",
				Decimals: 0,
			},
			Token1: core.Token{
				ChainID:  chain.BaseChainID,
				Address:  common.HexToAddress(testUSDC).Hex(),
				Symbol:   "USDC",
				Decimals: 0,
			},
			Fee:          500,
			FeeFormatted: "0.0500%",
			TickSpacing:  10,
			DebankPoolID: strings.ToLower(common.HexToAddress(testPool).Hex()),
		},
	}
	for _, item := range []struct {
		namespace string
		value     any
	}{
		{marketsNamespace, markets},
		{poolsNamespace, pools},
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

func fakeV3Factory(runner MulticallRunner) MulticallFactory {
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
	default:
		return "UNKNOWN"
	}
}

func packV3Result(data []byte, err error) (evm.Result, error) {
	if err != nil {
		return evm.Result{}, err
	}
	return evm.Result{Success: true, ReturnData: data}, nil
}

func q128Multiple(value int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(value), q128)
}

func unpackTickInput(data []byte) (int32, error) {
	values, err := poolABI.Methods["ticks"].Inputs.Unpack(data[4:])
	if err != nil {
		return 0, err
	}
	if len(values) != 1 {
		return 0, nil
	}
	return abiInt32(values[0])
}
