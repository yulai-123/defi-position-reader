package adapter

import (
	"context"
	"testing"

	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type registryStubAdapter struct {
	descriptor core.ProtocolDescriptor
}

func (a registryStubAdapter) Descriptor() core.ProtocolDescriptor {
	return a.descriptor
}

func (a registryStubAdapter) Syncer() MetadataSyncer {
	return NoopSyncer{Protocol: a.descriptor.ID}
}

func (registryStubAdapter) Fetcher() PositionFetcher {
	return nil
}

func TestRegistryResolveAllByChain(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "zeta", SupportedChains: []int64{1}}},
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "alpha", SupportedChains: []int64{1, 8453}}},
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "base-only", SupportedChains: []int64{8453}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	items, err := registry.Resolve(nil, 1)
	if err != nil {
		t.Fatalf("resolve all: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 adapters, got %d", len(items))
	}
	if items[0].Descriptor().ID != "alpha" || items[1].Descriptor().ID != "zeta" {
		t.Fatalf("adapters are not sorted by protocol id")
	}
}

func TestRegistryRejectsDuplicateProtocol(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "Demo", SupportedChains: []int64{1}}},
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}}},
	)
	if err == nil {
		t.Fatal("expected duplicate protocol error")
	}
}

func TestRegistryRejectsNilAndEmptyProtocol(t *testing.T) {
	t.Parallel()

	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("expected nil adapter error")
	}
	if _, err := NewRegistry(registryStubAdapter{}); err == nil {
		t.Fatal("expected empty protocol id error")
	}
}

func TestRegistryGetNormalizesProtocolID(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "Demo", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	if _, ok := registry.Get(" demo "); !ok {
		t.Fatal("expected to get demo adapter")
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("did not expect missing adapter")
	}
}

func TestRegistryResolveUnknownProtocol(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	_, err = registry.Resolve([]string{"missing"}, 1)
	if err == nil {
		t.Fatal("expected unknown protocol error")
	}
}

func TestRegistryResolveUnsupportedChain(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	_, err = registry.Resolve([]string{"demo"}, 8453)
	if err == nil {
		t.Fatal("expected unsupported chain error")
	}
}

func TestRegistryResolveDeduplicatesProtocolIDs(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "Demo", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	items, err := registry.Resolve([]string{" demo ", "DEMO", "demo"}, 1)
	if err != nil {
		t.Fatalf("resolve duplicate protocols: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 adapter after de-duplication, got %d", len(items))
	}
}

func TestRegistryResolveIgnoresBlankProtocolIDs(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: "demo", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	items, err := registry.Resolve([]string{" ", "demo"}, 1)
	if err != nil {
		t.Fatalf("resolve protocols: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 adapter, got %d", len(items))
	}
}

func TestRegistryListReturnsCanonicalProtocolIDs(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(
		registryStubAdapter{descriptor: core.ProtocolDescriptor{ID: " Demo ", SupportedChains: []int64{1}}},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	descriptors := registry.List()
	if len(descriptors) != 1 || descriptors[0].ID != "demo" {
		t.Fatalf("expected canonical descriptor id, got %#v", descriptors)
	}
}

func TestNoopSyncerDefaults(t *testing.T) {
	t.Parallel()

	result, err := NoopSyncer{Protocol: "demo"}.Sync(context.Background(), SyncRequest{
		Chain: core.Chain{ID: 1},
	})
	if err != nil {
		t.Fatalf("noop sync: %v", err)
	}
	if result.Metadata.Protocol != "demo" || result.Metadata.Namespace != "default" || result.Metadata.Version != "noop" {
		t.Fatalf("unexpected noop metadata: %#v", result.Metadata)
	}
	if result.Metadata.UpdatedAt.IsZero() {
		t.Fatal("expected updatedAt to be set")
	}
}
