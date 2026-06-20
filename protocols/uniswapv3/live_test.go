package uniswapv3

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type liveAccountFixture struct {
	ID                    string              `json:"id"`
	Chain                 string              `json:"chain"`
	Strategy              string              `json:"strategy"`
	Owner                 string              `json:"owner"`
	DebankProfile         string              `json:"debankProfile"`
	DebankProtocolID      string              `json:"debankProtocolId"`
	ExpectedPositionTypes []core.PositionType `json:"expectedPositionTypes"`
	ExpectedSymbols       []string            `json:"expectedSymbols"`
	ExpectedTokenIDs      []string            `json:"expectedTokenIds"`
	SourceCheckedAt       string              `json:"sourceCheckedAt"`
	Notes                 string              `json:"notes,omitempty"`
}

func TestLiveAccountFixturesAreValid(t *testing.T) {
	t.Parallel()

	fixtures := loadLiveAccountFixtures(t)
	if len(fixtures) == 0 {
		t.Fatal("expected at least one live account fixture")
	}

	seen := make(map[string]struct{}, len(fixtures))
	counts := make(map[string]int)
	for _, fixture := range fixtures {
		if _, ok := seen[fixture.ID]; ok {
			t.Fatalf("duplicate fixture id %q", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}

		if strings.TrimSpace(fixture.ID) == "" {
			t.Fatal("fixture id is empty")
		}
		target, ok := chain.ByName(fixture.Chain)
		if !ok {
			t.Fatalf("%s: unknown chain %q", fixture.ID, fixture.Chain)
		}
		if fixture.Strategy != "liquidity-pool" {
			t.Fatalf("%s: strategy must be liquidity-pool, got %q", fixture.ID, fixture.Strategy)
		}
		if !common.IsHexAddress(fixture.Owner) {
			t.Fatalf("%s: owner is not a valid EVM address: %q", fixture.ID, fixture.Owner)
		}
		if !strings.HasPrefix(fixture.DebankProfile, "https://debank.com/profile/") {
			t.Fatalf("%s: debank profile must be a DeBank profile URL: %q", fixture.ID, fixture.DebankProfile)
		}
		if fixture.DebankProtocolID != expectedDebankProtocolID(t, target.ID) {
			t.Fatalf("%s: debank protocol id = %q, want %q", fixture.ID, fixture.DebankProtocolID, expectedDebankProtocolID(t, target.ID))
		}
		if len(fixture.ExpectedPositionTypes) == 0 {
			t.Fatalf("%s: expected position types are empty", fixture.ID)
		}
		if len(fixture.ExpectedSymbols) == 0 {
			t.Fatalf("%s: expected symbols are empty", fixture.ID)
		}
		if len(fixture.ExpectedTokenIDs) == 0 {
			t.Fatalf("%s: expected token ids are empty", fixture.ID)
		}
		if _, err := time.Parse(time.DateOnly, fixture.SourceCheckedAt); err != nil {
			t.Fatalf("%s: sourceCheckedAt must use YYYY-MM-DD: %v", fixture.ID, err)
		}
		counts[strings.ToLower(fixture.Chain)]++
	}

	for _, chainName := range []string{"ethereum", "arbitrum", "base"} {
		if counts[chainName] < 3 {
			t.Fatalf("expected at least three %s fixtures, got %d", chainName, counts[chainName])
		}
	}
}

func TestLiveUniswapV3PositionsFromFixtures(t *testing.T) {
	if os.Getenv("DPR_LIVE_TESTS") != "1" {
		t.Skip("set DPR_LIVE_TESTS=1 to run Uniswap V3 live RPC smoke tests")
	}

	fixtures := filterLiveFixtures(loadLiveAccountFixtures(t), os.Getenv("DPR_LIVE_TEST_FILTER"))
	if len(fixtures) == 0 {
		t.Fatal("no live fixtures selected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	store, err := cache.NewSQLiteStore(filepath.Join(t.TempDir(), "metadata.sqlite"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	uniswap := New()
	syncer := uniswap.Syncer()
	fetcher := uniswap.Fetcher()

	for _, fixture := range fixtures {
		target, _ := chain.ByName(fixture.Chain)
		result, err := syncer.Sync(ctx, adapter.SyncRequest{
			Chain: target,
			Owner: fixture.Owner,
			Store: store,
		})
		if err != nil {
			t.Fatalf("%s: sync user-scoped metadata for %s: %v", fixture.ID, target.Name, err)
		}
		if result.Items == 0 {
			t.Fatalf("%s: sync metadata for %s returned zero items", fixture.ID, target.Name)
		}

		positions, err := fetcher.Fetch(ctx, adapter.FetchRequest{
			Chain: target,
			Owner: fixture.Owner,
			Store: store,
		})
		if err != nil {
			t.Fatalf("%s: fetch positions: %v", fixture.ID, err)
		}
		if len(positions) == 0 {
			t.Fatalf("%s: expected live fixture to return at least one position", fixture.ID)
		}

		types := collectPositionTypes(positions)
		symbols := collectPositionSymbols(positions)
		tokenIDs := collectPositionTokenIDs(positions)
		rangeStatuses := collectPositionRangeStatuses(positions)
		t.Logf("%s: positions=%d types=%v symbols=%v tokenIDs=%v ranges=%v", fixture.ID, len(positions), types, symbols, tokenIDs, rangeStatuses)

		for _, expectedType := range fixture.ExpectedPositionTypes {
			if !containsPositionType(types, expectedType) {
				t.Fatalf("%s: expected type %q in %v", fixture.ID, expectedType, types)
			}
		}
		for _, symbol := range fixture.ExpectedSymbols {
			if !containsString(symbols, symbol) {
				t.Fatalf("%s: expected symbol %q in %v", fixture.ID, symbol, symbols)
			}
		}
		for _, tokenID := range fixture.ExpectedTokenIDs {
			if !containsString(tokenIDs, tokenID) {
				t.Fatalf("%s: expected token id %q in %v", fixture.ID, tokenID, tokenIDs)
			}
		}
	}
}

func loadLiveAccountFixtures(t *testing.T) []liveAccountFixture {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "live_accounts.json"))
	if err != nil {
		t.Fatalf("read live account fixtures: %v", err)
	}

	var fixtures []liveAccountFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode live account fixtures: %v", err)
	}
	return fixtures
}

func filterLiveFixtures(fixtures []liveAccountFixture, filter string) []liveAccountFixture {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return fixtures
	}

	out := make([]liveAccountFixture, 0, len(fixtures))
	for _, fixture := range fixtures {
		if strings.Contains(strings.ToLower(fixture.ID), filter) ||
			strings.EqualFold(fixture.Chain, filter) ||
			strings.EqualFold(fixture.Strategy, filter) {
			out = append(out, fixture)
		}
	}
	return out
}

func expectedDebankProtocolID(t *testing.T, chainID int64) string {
	t.Helper()

	configs := marketConfigs(chainID)
	if len(configs) != 1 {
		t.Fatalf("expected exactly one uniswap v3 market config for chain %d, got %d", chainID, len(configs))
	}
	return configs[0].DebankProtocolID
}

func collectPositionTypes(positions []core.Position) []core.PositionType {
	seen := make(map[core.PositionType]struct{})
	for _, position := range positions {
		seen[position.Type] = struct{}{}
	}

	types := make([]core.PositionType, 0, len(seen))
	for positionType := range seen {
		types = append(types, positionType)
	}
	sort.Slice(types, func(i, j int) bool {
		return types[i] < types[j]
	})
	return types
}

func collectPositionSymbols(positions []core.Position) []string {
	seen := make(map[string]struct{})
	for _, position := range positions {
		addTokenAmountSymbols(seen, position.Shares)
		addTokenAmountSymbols(seen, position.Underlying)
		addTokenAmountSymbols(seen, position.Rewards)
		addTokenAmountSymbols(seen, position.Debt)
	}

	symbols := make([]string, 0, len(seen))
	for symbol := range seen {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

func addTokenAmountSymbols(seen map[string]struct{}, amounts []core.TokenAmount) {
	for _, amount := range amounts {
		symbol := strings.TrimSpace(amount.Token.Symbol)
		if symbol == "" {
			symbol = fmt.Sprintf("address:%s", strings.ToLower(amount.Token.Address))
		}
		seen[symbol] = struct{}{}
	}
}

func collectPositionTokenIDs(positions []core.Position) []string {
	seen := make(map[string]struct{})
	for _, position := range positions {
		tokenID := extraValueString(position.Extra, "tokenId")
		if tokenID != "" {
			seen[tokenID] = struct{}{}
		}
	}

	tokenIDs := make([]string, 0, len(seen))
	for tokenID := range seen {
		tokenIDs = append(tokenIDs, tokenID)
	}
	sort.Strings(tokenIDs)
	return tokenIDs
}

func collectPositionRangeStatuses(positions []core.Position) []string {
	seen := make(map[string]struct{})
	for _, position := range positions {
		status := extraValueString(position.Extra, "rangeStatus")
		if status != "" {
			seen[status] = struct{}{}
		}
	}

	statuses := make([]string, 0, len(seen))
	for status := range seen {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	return statuses
}

func extraValueString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func containsPositionType(types []core.PositionType, expected core.PositionType) bool {
	for _, positionType := range types {
		if positionType == expected {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
