package demo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type failingStore struct {
	err error
}

func (s failingStore) Set(context.Context, cache.Key, any, core.MetadataInfo) error {
	return s.err
}

func (s failingStore) Get(context.Context, cache.Key, any) (core.MetadataInfo, error) {
	return core.MetadataInfo{}, s.err
}

func TestDemoAdapterDescriptor(t *testing.T) {
	t.Parallel()

	descriptor := New().Descriptor()
	if descriptor.ID != ProtocolID || descriptor.Name == "" || descriptor.Category == "" {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
	if !descriptor.SupportsChain(chain.EthereumChainID) || !descriptor.SupportsChain(chain.ArbitrumChainID) || !descriptor.SupportsChain(chain.BaseChainID) {
		t.Fatalf("descriptor missing supported chains: %#v", descriptor.SupportedChains)
	}
}

func TestDemoAdapterSyncAndFetch(t *testing.T) {
	t.Parallel()

	target, ok := chain.ByName("base")
	if !ok {
		t.Fatal("base chain should exist")
	}

	store := newDemoTestStore(t)
	item := New()

	syncResult, err := item.Syncer().Sync(context.Background(), adapter.SyncRequest{
		Chain: target,
		Store: store,
	})
	if err != nil {
		t.Fatalf("sync demo metadata: %v", err)
	}
	if syncResult.Items != 1 {
		t.Fatalf("expected 1 demo market, got %d", syncResult.Items)
	}
	if syncResult.Metadata.Protocol != ProtocolID || syncResult.Metadata.ChainID != target.ID {
		t.Fatalf("unexpected sync metadata: %#v", syncResult.Metadata)
	}

	positions, err := item.Fetcher().Fetch(context.Background(), adapter.FetchRequest{
		Chain: target,
		Owner: "0xABC",
		Store: store,
	})
	if err != nil {
		t.Fatalf("fetch demo positions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 demo position, got %d", len(positions))
	}

	position := positions[0]
	if position.Protocol != ProtocolID || position.ChainID != target.ID {
		t.Fatalf("unexpected position identity: %#v", position)
	}
	if position.Owner != strings.ToLower("0xABC") {
		t.Fatalf("owner should be normalized to lowercase, got %q", position.Owner)
	}
	if len(position.Underlying) != 2 {
		t.Fatalf("expected 2 underlying tokens, got %d", len(position.Underlying))
	}
	if position.Underlying[0].Token.ChainID != target.ID || position.Underlying[1].Token.ChainID != target.ID {
		t.Fatalf("underlying tokens should use target chain: %#v", position.Underlying)
	}
}

func TestDemoFetcherFallsBackToEmbeddedMarkets(t *testing.T) {
	t.Parallel()

	target, ok := chain.ByName("ethereum")
	if !ok {
		t.Fatal("ethereum chain should exist")
	}

	positions, err := New().Fetcher().Fetch(context.Background(), adapter.FetchRequest{
		Chain: target,
		Owner: "0xabc",
		Store: newDemoTestStore(t),
	})
	if err != nil {
		t.Fatalf("fetch demo positions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected fallback demo position, got %d", len(positions))
	}
}

func TestDemoSyncAndFetchWorkWithoutStore(t *testing.T) {
	t.Parallel()

	target, ok := chain.ByName("ethereum")
	if !ok {
		t.Fatal("ethereum chain should exist")
	}

	item := New()
	syncResult, err := item.Syncer().Sync(context.Background(), adapter.SyncRequest{
		Chain: target,
	})
	if err != nil {
		t.Fatalf("sync without store: %v", err)
	}
	if syncResult.Items != 1 {
		t.Fatalf("expected 1 synced item, got %d", syncResult.Items)
	}

	positions, err := item.Fetcher().Fetch(context.Background(), adapter.FetchRequest{
		Chain: target,
		Owner: "0xabc",
	})
	if err != nil {
		t.Fatalf("fetch without store: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected fallback position, got %d", len(positions))
	}
}

func TestDemoFetcherRejectsInvalidCachedMarket(t *testing.T) {
	t.Parallel()

	target, ok := chain.ByName("ethereum")
	if !ok {
		t.Fatal("ethereum chain should exist")
	}

	store := newDemoTestStore(t)
	err := store.Set(context.Background(), cache.Key{
		ChainID:   target.ID,
		Protocol:  ProtocolID,
		Namespace: marketsNamespace,
	}, []Market{
		{
			ID:          "broken-market",
			DisplayName: "Broken Market",
			ShareToken:  wethToken(target.ID),
			Underlying:  []core.Token{wethToken(target.ID)},
		},
	}, core.MetadataInfo{Version: metadataVersion})
	if err != nil {
		t.Fatalf("seed invalid market: %v", err)
	}

	_, err = New().Fetcher().Fetch(context.Background(), adapter.FetchRequest{
		Chain: target,
		Owner: "0xabc",
		Store: store,
	})
	if err == nil {
		t.Fatal("expected invalid cached market error")
	}
}

func TestDemoAdapterPropagatesStoreErrors(t *testing.T) {
	t.Parallel()

	target, ok := chain.ByName("ethereum")
	if !ok {
		t.Fatal("ethereum chain should exist")
	}
	expected := errors.New("store failed")
	store := failingStore{err: expected}

	_, err := New().Syncer().Sync(context.Background(), adapter.SyncRequest{
		Chain: target,
		Store: store,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected sync store error, got %v", err)
	}

	_, err = New().Fetcher().Fetch(context.Background(), adapter.FetchRequest{
		Chain: target,
		Owner: "0xabc",
		Store: store,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected fetch store error, got %v", err)
	}
}

func newDemoTestStore(t *testing.T) *cache.SQLiteStore {
	t.Helper()

	store, err := cache.NewSQLiteStore(t.TempDir() + "/metadata.sqlite")
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
