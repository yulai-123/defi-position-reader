package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type PositionService struct {
	registry *adapter.Registry
	store    cache.Store
}

func NewPositionService(registry *adapter.Registry, store cache.Store) *PositionService {
	return &PositionService{
		registry: registry,
		store:    store,
	}
}

type FetchRequest struct {
	Chain          core.Chain
	Owner          string
	Protocols      []string
	MetadataMaxAge time.Duration
}

type SyncRequest struct {
	Chain     core.Chain
	Protocols []string
}

func (s *PositionService) FetchPositions(ctx context.Context, req FetchRequest) ([]core.Position, error) {
	if s.registry == nil {
		return nil, fmt.Errorf("fetch positions: registry is nil")
	}
	if s.store == nil {
		return nil, fmt.Errorf("fetch positions: cache store is nil")
	}
	if req.Chain.ID == 0 {
		return nil, fmt.Errorf("fetch positions: chain is empty")
	}
	if strings.TrimSpace(req.Owner) == "" {
		return nil, fmt.Errorf("fetch positions: owner is empty")
	}

	items, err := s.registry.Resolve(req.Protocols, req.Chain.ID)
	if err != nil {
		return nil, err
	}

	positions := make([]core.Position, 0)
	for _, item := range items {
		descriptor := item.Descriptor()
		fetcher := item.Fetcher()
		if fetcher == nil {
			return nil, fmt.Errorf("fetch positions: protocol %q has no fetcher", descriptor.ID)
		}

		result, err := fetcher.Fetch(ctx, adapter.FetchRequest{
			Chain:          req.Chain,
			Owner:          strings.TrimSpace(req.Owner),
			Store:          s.store,
			MetadataMaxAge: req.MetadataMaxAge,
		})
		if err != nil {
			return nil, fmt.Errorf("fetch positions: protocol %q: %w", descriptor.ID, err)
		}
		for _, position := range result {
			positions = append(positions, normalizePosition(position, req, descriptor))
		}
	}

	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Protocol == positions[j].Protocol {
			return positions[i].ID < positions[j].ID
		}
		return positions[i].Protocol < positions[j].Protocol
	})
	return positions, nil
}

func (s *PositionService) SyncMetadata(ctx context.Context, req SyncRequest) ([]adapter.SyncResult, error) {
	if s.registry == nil {
		return nil, fmt.Errorf("sync metadata: registry is nil")
	}
	if s.store == nil {
		return nil, fmt.Errorf("sync metadata: cache store is nil")
	}
	if req.Chain.ID == 0 {
		return nil, fmt.Errorf("sync metadata: chain is empty")
	}

	items, err := s.registry.Resolve(req.Protocols, req.Chain.ID)
	if err != nil {
		return nil, err
	}

	results := make([]adapter.SyncResult, 0, len(items))
	for _, item := range items {
		descriptor := item.Descriptor()
		syncer := item.Syncer()
		if syncer == nil {
			return nil, fmt.Errorf("sync metadata: protocol %q has no metadata syncer", descriptor.ID)
		}

		result, err := syncer.Sync(ctx, adapter.SyncRequest{
			Chain: req.Chain,
			Store: s.store,
		})
		if err != nil {
			return nil, fmt.Errorf("sync metadata: protocol %q: %w", descriptor.ID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func normalizePosition(position core.Position, req FetchRequest, descriptor core.ProtocolDescriptor) core.Position {
	if position.ChainID == 0 {
		position.ChainID = req.Chain.ID
	}
	if position.Protocol == "" {
		position.Protocol = descriptor.ID
	}
	if position.Owner == "" {
		position.Owner = strings.TrimSpace(req.Owner)
	}
	if position.Type == "" {
		position.Type = core.PositionTypeUnknown
	}
	return position
}
