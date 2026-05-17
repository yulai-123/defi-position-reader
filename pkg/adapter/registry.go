package adapter

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{adapters: make(map[string]Adapter)}
	for _, item := range adapters {
		if err := r.Register(item); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(item Adapter) error {
	if item == nil {
		return fmt.Errorf("register adapter: adapter is nil")
	}

	descriptor := item.Descriptor()
	id := normalizeProtocolID(descriptor.ID)
	if id == "" {
		return fmt.Errorf("register adapter: protocol id is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.adapters[id]; exists {
		return fmt.Errorf("register adapter: duplicated protocol %q", id)
	}
	r.adapters[id] = item
	return nil
}

func (r *Registry) Get(protocolID string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.adapters[normalizeProtocolID(protocolID)]
	return item, ok
}

func (r *Registry) List() []core.ProtocolDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	descriptors := make([]core.ProtocolDescriptor, 0, len(r.adapters))
	for _, item := range r.adapters {
		descriptors = append(descriptors, canonicalDescriptor(item.Descriptor()))
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID < descriptors[j].ID
	})
	return descriptors
}

func (r *Registry) Resolve(protocolIDs []string, chainID int64) ([]Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(protocolIDs) == 0 {
		items := make([]Adapter, 0, len(r.adapters))
		for _, item := range r.adapters {
			if item.Descriptor().SupportsChain(chainID) {
				items = append(items, item)
			}
		}
		sortAdapters(items)
		return items, nil
	}

	items := make([]Adapter, 0, len(protocolIDs))
	seen := make(map[string]struct{}, len(protocolIDs))
	for _, protocolID := range protocolIDs {
		id := normalizeProtocolID(protocolID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		item, ok := r.adapters[id]
		if !ok {
			return nil, fmt.Errorf("resolve adapter: unknown protocol %q", id)
		}
		if !item.Descriptor().SupportsChain(chainID) {
			return nil, fmt.Errorf("resolve adapter: protocol %q does not support chain %d", id, chainID)
		}
		items = append(items, item)
	}
	sortAdapters(items)
	return items, nil
}

func sortAdapters(items []Adapter) {
	sort.Slice(items, func(i, j int) bool {
		return normalizeProtocolID(items[i].Descriptor().ID) < normalizeProtocolID(items[j].Descriptor().ID)
	})
}

func normalizeProtocolID(protocolID string) string {
	return strings.ToLower(strings.TrimSpace(protocolID))
}

func canonicalDescriptor(descriptor core.ProtocolDescriptor) core.ProtocolDescriptor {
	descriptor.ID = normalizeProtocolID(descriptor.ID)
	return descriptor
}
