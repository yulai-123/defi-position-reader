package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type serviceStubAdapter struct {
	descriptor core.ProtocolDescriptor
	fetcher    adapter.PositionFetcher
	syncer     adapter.MetadataSyncer
}

func (a serviceStubAdapter) Descriptor() core.ProtocolDescriptor {
	return a.descriptor
}

func (a serviceStubAdapter) Syncer() adapter.MetadataSyncer {
	return a.syncer
}

func (a serviceStubAdapter) Fetcher() adapter.PositionFetcher {
	return a.fetcher
}

type fetcherFunc func(context.Context, adapter.FetchRequest) ([]core.Position, error)

func (f fetcherFunc) Fetch(ctx context.Context, req adapter.FetchRequest) ([]core.Position, error) {
	return f(ctx, req)
}

type syncerFunc func(context.Context, adapter.SyncRequest) (adapter.SyncResult, error)

func (f syncerFunc) Sync(ctx context.Context, req adapter.SyncRequest) (adapter.SyncResult, error) {
	return f(ctx, req)
}

func TestPositionServiceFetchPositionsFiltersAndNormalizes(t *testing.T) {
	t.Parallel()

	called := false
	registry, err := adapter.NewRegistry(
		serviceStubAdapter{
			descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
			fetcher: fetcherFunc(func(_ context.Context, req adapter.FetchRequest) ([]core.Position, error) {
				called = true
				if req.Chain.ID != 1 || req.Owner != "0xabc" || req.Store == nil {
					t.Fatalf("unexpected fetch request: %#v", req)
				}
				return []core.Position{{ID: "position-1"}}, nil
			}),
			syncer: adapter.NoopSyncer{Protocol: "demo"},
		},
		serviceStubAdapter{
			descriptor: core.ProtocolDescriptor{ID: "base-only", SupportedChains: []int64{8453}},
			fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
				t.Fatal("base-only fetcher should not be called")
				return nil, nil
			}),
			syncer: adapter.NoopSyncer{Protocol: "base-only"},
		},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	positions, err := svc.FetchPositions(context.Background(), FetchRequest{
		Chain: core.Chain{ID: 1, Name: "ethereum"},
		Owner: "0xabc",
	})
	if err != nil {
		t.Fatalf("fetch positions: %v", err)
	}
	if !called {
		t.Fatal("expected demo fetcher to be called")
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].ChainID != 1 || positions[0].Protocol != "demo" || positions[0].Owner != "0xabc" {
		t.Fatalf("position was not normalized: %#v", positions[0])
	}
	if positions[0].Type != core.PositionTypeUnknown {
		t.Fatalf("expected default unknown type, got %q", positions[0].Type)
	}
}

func TestPositionServiceFetchPositionsPropagatesFetcherError(t *testing.T) {
	t.Parallel()

	expected := errors.New("boom")
	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
			return nil, expected
		}),
		syncer: adapter.NoopSyncer{Protocol: "demo"},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	_, err = svc.FetchPositions(context.Background(), FetchRequest{
		Chain:     core.Chain{ID: 1, Name: "ethereum"},
		Owner:     "0xabc",
		Protocols: []string{"demo"},
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped fetcher error, got %v", err)
	}
}

func TestPositionServiceFetchPositionsValidatesRequest(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
			return nil, nil
		}),
		syncer: adapter.NoopSyncer{Protocol: "demo"},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	tests := []struct {
		name string
		svc  *PositionService
		req  FetchRequest
	}{
		{
			name: "nil registry",
			svc:  NewPositionService(nil, newServiceTestStore(t)),
			req:  FetchRequest{Chain: core.Chain{ID: 1}, Owner: "0xabc"},
		},
		{
			name: "nil store",
			svc:  NewPositionService(registry, nil),
			req:  FetchRequest{Chain: core.Chain{ID: 1}, Owner: "0xabc"},
		},
		{
			name: "empty chain",
			svc:  NewPositionService(registry, newServiceTestStore(t)),
			req:  FetchRequest{Owner: "0xabc"},
		},
		{
			name: "empty owner",
			svc:  NewPositionService(registry, newServiceTestStore(t)),
			req:  FetchRequest{Chain: core.Chain{ID: 1}, Owner: " "},
		},
	}

	for _, tt := range tests {
		if _, err := tt.svc.FetchPositions(context.Background(), tt.req); err == nil {
			t.Fatalf("expected fetch validation error for %s", tt.name)
		}
	}
}

func TestPositionServiceFetchPositionsRejectsMissingFetcher(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		syncer:     adapter.NoopSyncer{Protocol: "demo"},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	_, err = svc.FetchPositions(context.Background(), FetchRequest{
		Chain:     core.Chain{ID: 1},
		Owner:     "0xabc",
		Protocols: []string{"demo"},
	})
	if err == nil {
		t.Fatal("expected missing fetcher error")
	}
}

func TestPositionServiceFetchPositionsSortsResults(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
			return []core.Position{
				{ID: "b", Protocol: "demo"},
				{ID: "a", Protocol: "demo"},
			}, nil
		}),
		syncer: adapter.NoopSyncer{Protocol: "demo"},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	positions, err := svc.FetchPositions(context.Background(), FetchRequest{
		Chain: core.Chain{ID: 1},
		Owner: "0xabc",
	})
	if err != nil {
		t.Fatalf("fetch positions: %v", err)
	}
	if len(positions) != 2 || positions[0].ID != "a" || positions[1].ID != "b" {
		t.Fatalf("positions were not sorted: %#v", positions)
	}
}

func TestPositionServiceSyncMetadata(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
			return nil, nil
		}),
		syncer: syncerFunc(func(_ context.Context, req adapter.SyncRequest) (adapter.SyncResult, error) {
			if req.Chain.ID != 1 || req.Store == nil {
				t.Fatalf("unexpected sync request: %#v", req)
			}
			return adapter.SyncResult{
				Metadata: core.MetadataInfo{
					ChainID:   req.Chain.ID,
					Protocol:  "demo",
					Namespace: "markets",
					Version:   "v1",
					UpdatedAt: updatedAt,
				},
				Items: 2,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	results, err := svc.SyncMetadata(context.Background(), SyncRequest{
		Chain:     core.Chain{ID: 1, Name: "ethereum"},
		Protocols: []string{"demo"},
	})
	if err != nil {
		t.Fatalf("sync metadata: %v", err)
	}
	if len(results) != 1 || results[0].Items != 2 {
		t.Fatalf("unexpected sync results: %#v", results)
	}
	if !results[0].Metadata.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected metadata: %#v", results[0].Metadata)
	}
}

func TestPositionServiceSyncMetadataValidatesRequest(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(serviceStubAdapter{
		descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}},
		fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
			return nil, nil
		}),
		syncer: adapter.NoopSyncer{Protocol: "demo"},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	tests := []struct {
		name string
		svc  *PositionService
		req  SyncRequest
	}{
		{
			name: "nil registry",
			svc:  NewPositionService(nil, newServiceTestStore(t)),
			req:  SyncRequest{Chain: core.Chain{ID: 1}},
		},
		{
			name: "nil store",
			svc:  NewPositionService(registry, nil),
			req:  SyncRequest{Chain: core.Chain{ID: 1}},
		},
		{
			name: "empty chain",
			svc:  NewPositionService(registry, newServiceTestStore(t)),
			req:  SyncRequest{},
		},
	}

	for _, tt := range tests {
		if _, err := tt.svc.SyncMetadata(context.Background(), tt.req); err == nil {
			t.Fatalf("expected sync validation error for %s", tt.name)
		}
	}
}

func TestPositionServiceSyncMetadataPropagatesErrors(t *testing.T) {
	t.Parallel()

	expected := errors.New("sync failed")
	registry, err := adapter.NewRegistry(
		serviceStubAdapter{
			descriptor: core.ProtocolDescriptor{ID: "no-syncer", SupportedChains: []int64{1}},
			fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
				return nil, nil
			}),
		},
		serviceStubAdapter{
			descriptor: core.ProtocolDescriptor{ID: "broken", SupportedChains: []int64{1}},
			fetcher: fetcherFunc(func(context.Context, adapter.FetchRequest) ([]core.Position, error) {
				return nil, nil
			}),
			syncer: syncerFunc(func(context.Context, adapter.SyncRequest) (adapter.SyncResult, error) {
				return adapter.SyncResult{}, expected
			}),
		},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	svc := NewPositionService(registry, newServiceTestStore(t))
	if _, err := svc.SyncMetadata(context.Background(), SyncRequest{
		Chain:     core.Chain{ID: 1},
		Protocols: []string{"no-syncer"},
	}); err == nil {
		t.Fatal("expected missing syncer error")
	}
	if _, err := svc.SyncMetadata(context.Background(), SyncRequest{
		Chain:     core.Chain{ID: 1},
		Protocols: []string{"broken"},
	}); !errors.Is(err, expected) {
		t.Fatalf("expected wrapped sync error, got %v", err)
	}
}

func newServiceTestStore(t *testing.T) *cache.SQLiteStore {
	t.Helper()

	store, err := cache.NewSQLiteStore(t.TempDir() + "/metadata.sqlite")
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
