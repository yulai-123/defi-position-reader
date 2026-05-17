package cache

import (
	"context"
	"errors"

	"github.com/yulai-123/defi-position-reader/pkg/core"
)

var ErrNotFound = errors.New("cache metadata not found")

type Key struct {
	ChainID   int64
	Protocol  string
	Namespace string
}

type Store interface {
	Set(ctx context.Context, key Key, value any, info core.MetadataInfo) error
	Get(ctx context.Context, key Key, out any) (core.MetadataInfo, error)
}
