package compoundv3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	DebankPool            string              `json:"debankPool"`
	ExpectedPositionTypes []core.PositionType `json:"expectedPositionTypes"`
	ExpectedSymbols       []string            `json:"expectedSymbols"`
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
		if _, ok := chain.ByName(fixture.Chain); !ok {
			t.Fatalf("%s: unknown chain %q", fixture.ID, fixture.Chain)
		}
		if fixture.Strategy != "yield" && fixture.Strategy != "lending" {
			t.Fatalf("%s: strategy must be yield or lending, got %q", fixture.ID, fixture.Strategy)
		}
		if !common.IsHexAddress(fixture.Owner) {
			t.Fatalf("%s: owner is not a valid EVM address: %q", fixture.ID, fixture.Owner)
		}
		if _, err := debankPoolIDFromURL(fixture.DebankPool); err != nil {
			t.Fatalf("%s: invalid debank pool URL: %v", fixture.ID, err)
		}
		if len(fixture.ExpectedPositionTypes) == 0 {
			t.Fatalf("%s: expected position types are empty", fixture.ID)
		}
		if len(fixture.ExpectedSymbols) == 0 {
			t.Fatalf("%s: expected symbols are empty", fixture.ID)
		}
		if _, err := time.Parse(time.DateOnly, fixture.SourceCheckedAt); err != nil {
			t.Fatalf("%s: sourceCheckedAt must use YYYY-MM-DD: %v", fixture.ID, err)
		}
		counts[strings.ToLower(fixture.Chain)+":"+fixture.Strategy]++
	}

	for _, chainName := range []string{"ethereum", "arbitrum", "base"} {
		for _, strategy := range []string{"yield", "lending"} {
			key := chainName + ":" + strategy
			if counts[key] < 2 {
				t.Fatalf("expected at least two %s fixtures, got %d", key, counts[key])
			}
		}
	}
}

func TestLiveCompoundV3PositionsFromFixtures(t *testing.T) {
	if os.Getenv("DPR_LIVE_TESTS") != "1" {
		t.Skip("set DPR_LIVE_TESTS=1 to run Compound V3 live RPC smoke tests")
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

	compound := New()
	syncer := compound.Syncer()
	fetcher := compound.Fetcher()
	synced := make(map[int64]struct{})

	for _, fixture := range fixtures {
		target, _ := chain.ByName(fixture.Chain)
		if _, ok := synced[target.ID]; !ok {
			result, err := syncer.Sync(ctx, adapter.SyncRequest{
				Chain: target,
				Store: store,
			})
			if err != nil {
				t.Fatalf("%s: sync metadata for %s: %v", fixture.ID, target.Name, err)
			}
			if result.Items == 0 {
				t.Fatalf("%s: sync metadata for %s returned zero items", fixture.ID, target.Name)
			}
			t.Logf("%s: synced %s metadata items=%d", fixture.ID, target.Name, result.Items)
			synced[target.ID] = struct{}{}
		}

		positions, err := fetcher.Fetch(ctx, adapter.FetchRequest{
			Chain: target,
			Owner: fixture.Owner,
			Store: store,
		})
		if err != nil {
			t.Fatalf("%s: fetch positions: %v", fixture.ID, err)
		}

		poolID, err := debankPoolIDFromURL(fixture.DebankPool)
		if err != nil {
			t.Fatalf("%s: parse debank pool: %v", fixture.ID, err)
		}
		position, ok := findDebankPoolPosition(positions, poolID, fixture.Strategy)
		if !ok {
			t.Fatalf("%s: expected position for DeBank pool %s/%s, got %#v", fixture.ID, poolID, fixture.Strategy, positions)
		}

		symbols := collectPositionSymbols([]core.Position{position})
		t.Logf("%s: position=%s symbols=%v", fixture.ID, position.DisplayName, symbols)
		if !containsPositionType(fixture.ExpectedPositionTypes, position.Type) {
			t.Fatalf("%s: expected type %q in %v", fixture.ID, position.Type, fixture.ExpectedPositionTypes)
		}
		for _, symbol := range fixture.ExpectedSymbols {
			if !containsString(symbols, symbol) {
				t.Fatalf("%s: expected symbol %q in %v", fixture.ID, symbol, symbols)
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

func debankPoolIDFromURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "protocols" || parts[1] != "pool" {
		return "", fmt.Errorf("expected /protocols/pool/{poolID}/... path, got %q", parsed.Path)
	}
	poolID, err := url.PathUnescape(parts[2])
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(poolID) == "" {
		return "", fmt.Errorf("pool id is empty")
	}
	return strings.ToLower(poolID), nil
}

func findDebankPoolPosition(positions []core.Position, debankPoolID string, strategy string) (core.Position, bool) {
	debankPoolID = strings.ToLower(strings.TrimSpace(debankPoolID))
	for _, position := range positions {
		if strings.ToLower(extraValueString(position.Extra, "debankPoolId")) == debankPoolID &&
			extraValueString(position.Extra, "strategy") == strategy {
			return position, true
		}
	}
	return core.Position{}, false
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

func collectPositionSymbols(positions []core.Position) []string {
	seen := make(map[string]struct{})
	for _, position := range positions {
		addTokenAmountSymbols(seen, position.Shares)
		addTokenAmountSymbols(seen, position.Underlying)
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

func containsString(values []string, needle string) bool {
	normalizedNeedle := normalizeTokenSymbol(needle)
	for _, value := range values {
		if normalizeTokenSymbol(value) == normalizedNeedle {
			return true
		}
	}
	return false
}

func normalizeTokenSymbol(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToUpper(value) {
	case "USD₮0", "USDT0":
		return "USDT"
	default:
		return value
	}
}

func containsPositionType(values []core.PositionType, needle core.PositionType) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
