package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

func TestParseProtocols(t *testing.T) {
	t.Parallel()

	if got := parseProtocols("all"); got != nil {
		t.Fatalf("all should resolve to nil, got %#v", got)
	}
	if got := parseProtocols(" "); got != nil {
		t.Fatalf("empty protocols should resolve to nil, got %#v", got)
	}

	got := parseProtocols(" demo, aave-v3 ,, ")
	if len(got) != 2 || got[0] != "demo" || got[1] != "aave-v3" {
		t.Fatalf("unexpected parsed protocols: %#v", got)
	}
}

func TestRunPrintsUsage(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, []string{"help"}} {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err != nil {
			t.Fatalf("run usage with args %#v: %v", args, err)
		}
		if !strings.Contains(out.String(), "DeFi Position Reader") {
			t.Fatalf("usage output missing title: %s", out.String())
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{"missing"}, &out)
	if err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestRunChains(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{"chains"}, &out)
	if err != nil {
		t.Fatalf("run chains: %v", err)
	}
	if !strings.Contains(out.String(), `"ethereum"`) || !strings.Contains(out.String(), `"arbitrum"`) || !strings.Contains(out.String(), `"base"`) {
		t.Fatalf("chains output missing expected networks: %s", out.String())
	}
}

func TestRunProtocolsFiltersByChain(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{"protocols", "-chain", "base"}, &out)
	if err != nil {
		t.Fatalf("run protocols: %v", err)
	}
	if !strings.Contains(out.String(), `"id": "demo"`) {
		t.Fatalf("protocols output missing demo: %s", out.String())
	}
	if !strings.Contains(out.String(), `"id": "uniswap-v3"`) {
		t.Fatalf("protocols output missing uniswap-v3: %s", out.String())
	}
}

func TestRunSyncMetadata(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"sync-metadata",
		"-chain", "ethereum",
		"-protocol", " demo,demo ",
		"-cache-dir", t.TempDir(),
	}, &out)
	if err != nil {
		t.Fatalf("run sync-metadata: %v", err)
	}
	if !strings.Contains(out.String(), `"metadata"`) || !strings.Contains(out.String(), `"items": 1`) {
		t.Fatalf("unexpected sync output: %s", out.String())
	}
}

func TestRunPositions(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"positions",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-address", "0x0000000000000000000000000000000000000001",
		"-cache-dir", t.TempDir(),
	}, &out)
	if err != nil {
		t.Fatalf("run positions: %v", err)
	}
	if !strings.Contains(out.String(), `"type": "liquidity"`) || !strings.Contains(out.String(), `"underlying"`) {
		t.Fatalf("unexpected positions output: %s", out.String())
	}
}

func TestRunPositionsTableFormat(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"positions",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-address", "0x0000000000000000000000000000000000000001",
		"-cache-dir", t.TempDir(),
		"-format", "table",
	}, &out)
	if err != nil {
		t.Fatalf("run positions table: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Positions: Ethereum") || !strings.Contains(output, "Demo WETH / USDC LP") || !strings.Contains(output, "0.42 WETH") {
		t.Fatalf("unexpected table output: %s", output)
	}
}

func TestWritePositionsTableShowsRewardPosition(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	positions := []core.Position{
		{
			Type:        core.PositionTypeReward,
			Protocol:    "compound-v3",
			DisplayName: "Compound V3 Base USDC Rewards",
			Underlying: []core.TokenAmount{
				{
					Token:     core.Token{Symbol: "COMP", Decimals: 18},
					Raw:       "42000000000000000",
					Formatted: "0.042",
				},
			},
		},
	}
	if err := writePositionsTable(&out, core.Chain{ID: 8453, DisplayName: "Base"}, "0x1", positions); err != nil {
		t.Fatalf("write positions table: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "reward") || !strings.Contains(output, "Compound V3 Base USDC Rewards") || !strings.Contains(output, "0.042 COMP") {
		t.Fatalf("unexpected reward table output: %s", output)
	}
}

func TestWritePositionsTableShowsInlineRewards(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	positions := []core.Position{
		{
			Type:        core.PositionTypeLiquidity,
			Protocol:    "uniswap-v3",
			DisplayName: "Uniswap V3 WETH / USDC 0.05% LP",
			Shares: []core.TokenAmount{
				{Token: core.Token{Symbol: "UNI-V3-POS"}, Raw: "1", Formatted: "1"},
			},
			Underlying: []core.TokenAmount{
				{Token: core.Token{Symbol: "WETH"}, Raw: "10", Formatted: "10"},
			},
			Rewards: []core.TokenAmount{
				{Token: core.Token{Symbol: "USDC"}, Raw: "42", Formatted: "42"},
			},
		},
	}
	if err := writePositionsTable(&out, core.Chain{ID: 1, DisplayName: "Ethereum"}, "0x1", positions); err != nil {
		t.Fatalf("write positions table: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "REWARDS") || !strings.Contains(output, "42 USDC") {
		t.Fatalf("unexpected inline rewards table output: %s", output)
	}
}

func TestWritePositionsTableShowsYieldUnderlying(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	positions := []core.Position{
		{
			Type:        core.PositionTypeYield,
			Protocol:    "compound-v3",
			DisplayName: "Compound V3 Base USDC Yield",
			Shares: []core.TokenAmount{
				{
					Token:     core.Token{Symbol: "cUSDCv3", Decimals: 6},
					Raw:       "1500000",
					Formatted: "1.5",
				},
			},
			Underlying: []core.TokenAmount{
				{
					Token:     core.Token{Symbol: "USDC", Decimals: 6},
					Raw:       "1500000",
					Formatted: "1.5",
				},
			},
		},
	}
	if err := writePositionsTable(&out, core.Chain{ID: 8453, DisplayName: "Base"}, "0x1", positions); err != nil {
		t.Fatalf("write positions table: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "yield") || !strings.Contains(output, "1.5 cUSDCv3") || !strings.Contains(output, "1.5 USDC") {
		t.Fatalf("unexpected yield table output: %s", output)
	}
}

func TestRunPositionsDetailTrace(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"positions",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-address", "0x0000000000000000000000000000000000000001",
		"-cache-dir", t.TempDir(),
		"-format", "detail",
		"-trace",
	}, &out)
	if err != nil {
		t.Fatalf("run positions detail trace: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Metadata Cache:") || !strings.Contains(output, "Trace") || !strings.Contains(output, "metadata.read") {
		t.Fatalf("unexpected detail trace output: %s", output)
	}
}

func TestRunSyncMetadataTableFormat(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"sync-metadata",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-cache-dir", t.TempDir(),
		"-format", "table",
	}, &out)
	if err != nil {
		t.Fatalf("run sync table: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Sync Metadata: Ethereum") || !strings.Contains(output, "demo") || !strings.Contains(output, "markets") {
		t.Fatalf("unexpected sync table output: %s", output)
	}
}

func TestWriteSyncDetailIncludesDiscovery(t *testing.T) {
	t.Parallel()

	result := syncResultWithDiscovery()
	var out bytes.Buffer
	err := writeSyncOutput(&out, core.Chain{ID: 1, DisplayName: "Ethereum"}, []adapter.SyncResult{result}, nil, outputOptions{
		Format: outputFormatDetail,
	}, nil)
	if err != nil {
		t.Fatalf("write sync output: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Discovery:") ||
		!strings.Contains(output, "discover lending reserves") ||
		!strings.Contains(output, "DataProvider.getAllReservesTokens") {
		t.Fatalf("discovery output missing expected details: %s", output)
	}
}

func TestAddSyncDiscoveryTrace(t *testing.T) {
	t.Parallel()

	trace := newTraceRecorder(outputOptions{Trace: true, TraceLevel: traceLevelCalls})
	addSyncDiscoveryTrace(trace, []adapter.SyncResult{syncResultWithDiscovery()})
	events := trace.Events()
	if len(events) == 0 {
		t.Fatal("expected discovery trace events")
	}
	var adapterEvent, evmEvent bool
	for _, event := range events {
		if event.Operation == "aave-v3.sync.discover-lending-reserves" {
			adapterEvent = true
		}
		if event.Layer == "evm" && event.Operation == "multicall.aggregate3" {
			evmEvent = true
		}
	}
	if !adapterEvent || !evmEvent {
		t.Fatalf("expected adapter and evm discovery events, got %#v", events)
	}
}

func TestRunExplain(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"explain",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-cache-dir", t.TempDir(),
	}, &out)
	if err != nil {
		t.Fatalf("run explain: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Explain: Ethereum") || !strings.Contains(output, "demo") || !strings.Contains(output, "markets:missing") {
		t.Fatalf("unexpected explain output: %s", output)
	}
}

func syncResultWithDiscovery() adapter.SyncResult {
	return adapter.SyncResult{
		Metadata: core.MetadataInfo{
			ChainID:   1,
			Protocol:  "aave-v3",
			Namespace: "lending-reserves",
			Version:   "test",
			UpdatedAt: time.Date(2026, 5, 24, 1, 0, 0, 0, time.UTC),
		},
		Items: 2,
		Details: map[string]any{
			"discovery": []map[string]any{
				{
					"step":      "discover lending reserves",
					"operation": "DataProvider.getAllReservesTokens",
					"markets":   1,
					"calls":     1,
					"items":     2,
					"status":    "ok",
					"notes":     "one call per market",
				},
			},
		},
	}
}

func TestRunRejectsUnknownChain(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{"positions", "-chain", "unknown", "-address", "0xabc"}, &out)
	if err == nil {
		t.Fatal("expected unknown chain error")
	}
}

func TestRunRejectsInvalidFlags(t *testing.T) {
	t.Parallel()

	commands := [][]string{
		{"protocols", "-bad"},
		{"sync-metadata", "-bad"},
		{"positions", "-bad"},
		{"explain", "-bad"},
	}
	for _, args := range commands {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("expected invalid flag error for args %#v", args)
		}
	}
}

func TestRunRejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	commands := [][]string{
		{"sync-metadata", "-format", "yaml"},
		{"positions", "-format", "yaml"},
		{"explain", "-format", "yaml"},
	}
	for _, args := range commands {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("expected unknown output format error for args %#v", args)
		}
	}
}

func TestRunRejectsUnknownTraceLevel(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{"positions", "-trace-level", "loud"}, &out)
	if err == nil {
		t.Fatal("expected unknown trace level error")
	}
}

func TestRunRejectsUnknownChainForProtocolsAndSync(t *testing.T) {
	t.Parallel()

	commands := [][]string{
		{"protocols", "-chain", "unknown"},
		{"sync-metadata", "-chain", "unknown"},
	}
	for _, args := range commands {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("expected unknown chain error for args %#v", args)
		}
	}
}

func TestRunPositionsRejectsEmptyAddress(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"positions",
		"-chain", "ethereum",
		"-protocol", "demo",
		"-cache-dir", t.TempDir(),
	}, &out)
	if err == nil {
		t.Fatal("expected empty address error")
	}
}
