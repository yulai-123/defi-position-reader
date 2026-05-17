package adapter

import (
	"context"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type Adapter interface {
	Descriptor() core.ProtocolDescriptor
	Syncer() MetadataSyncer
	Fetcher() PositionFetcher
}

type SyncRequest struct {
	Chain core.Chain
	Store cache.Store
}

type SyncResult struct {
	Metadata core.MetadataInfo `json:"metadata"`
	Items    int               `json:"items"`
}

type FetchRequest struct {
	Chain core.Chain
	Owner string
	Store cache.Store
}

type MetadataSyncer interface {
	Sync(ctx context.Context, req SyncRequest) (SyncResult, error)
}

type PositionFetcher interface {
	Fetch(ctx context.Context, req FetchRequest) ([]core.Position, error)
}

type NoopSyncer struct {
	Protocol  string
	Namespace string
	Version   string
}

func (s NoopSyncer) Sync(_ context.Context, req SyncRequest) (SyncResult, error) {
	namespace := s.Namespace
	if namespace == "" {
		namespace = "default"
	}
	version := s.Version
	if version == "" {
		version = "noop"
	}

	return SyncResult{
		Metadata: core.MetadataInfo{
			ChainID:   req.Chain.ID,
			Protocol:  s.Protocol,
			Namespace: namespace,
			Version:   version,
			UpdatedAt: time.Now().UTC(),
			Source:    "noop",
		},
	}, nil
}
