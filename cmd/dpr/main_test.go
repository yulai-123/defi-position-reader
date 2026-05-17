package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
	}
	for _, args := range commands {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("expected invalid flag error for args %#v", args)
		}
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
