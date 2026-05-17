package cache

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/core"
)

func TestSQLiteStoreSetGet(t *testing.T) {
	t.Parallel()

	type market struct {
		ID     string   `json:"id"`
		Tokens []string `json:"tokens"`
	}

	ctx := context.Background()
	store := newTestSQLiteStore(t)
	input := []market{
		{ID: "weth-usdc", Tokens: []string{"WETH", "USDC"}},
	}
	updatedAt := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	err := store.Set(ctx, Key{
		ChainID:   1,
		Protocol:  "Demo",
		Namespace: "Markets",
	}, input, core.MetadataInfo{
		Version:     "v1",
		BlockNumber: 123,
		UpdatedAt:   updatedAt,
		Source:      "unit-test",
	})
	if err != nil {
		t.Fatalf("set cache: %v", err)
	}

	var got []market
	info, err := store.Get(ctx, Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "markets",
	}, &got)
	if err != nil {
		t.Fatalf("get cache: %v", err)
	}

	if !reflect.DeepEqual(got, input) {
		t.Fatalf("cache data mismatch\nwant: %#v\n got: %#v", input, got)
	}
	if info.ChainID != 1 || info.Protocol != "demo" || info.Namespace != "markets" {
		t.Fatalf("metadata key mismatch: %#v", info)
	}
	if info.Version != "v1" || info.BlockNumber != 123 || info.Source != "unit-test" {
		t.Fatalf("metadata content mismatch: %#v", info)
	}
	if !info.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("metadata updatedAt mismatch: %s", info.UpdatedAt)
	}
}

func TestSQLiteStoreGetNotFound(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	var out map[string]any
	_, err := store.Get(context.Background(), Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "missing",
	}, &out)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStoreRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	tests := []Key{
		{ChainID: 0, Protocol: "demo", Namespace: "markets"},
		{ChainID: 1, Protocol: "", Namespace: "markets"},
		{ChainID: 1, Protocol: "demo", Namespace: ""},
		{ChainID: 1, Protocol: "demo\x00", Namespace: "markets"},
	}
	for _, key := range tests {
		err := store.Set(context.Background(), key, map[string]string{"ok": "no"}, core.MetadataInfo{})
		if err == nil {
			t.Fatalf("expected invalid key error for %#v", key)
		}
	}
}

func TestNewSQLiteStoreRejectsEmptyPath(t *testing.T) {
	t.Parallel()

	if _, err := NewSQLiteStore(" "); err == nil {
		t.Fatal("expected empty path error")
	}
}

func TestSQLiteStoreHandlesNilStore(t *testing.T) {
	t.Parallel()

	var store *SQLiteStore
	if err := store.Close(); err != nil {
		t.Fatalf("close nil store: %v", err)
	}
	if err := store.Set(context.Background(), Key{ChainID: 1, Protocol: "demo", Namespace: "markets"}, nil, core.MetadataInfo{}); err == nil {
		t.Fatal("expected set on nil store error")
	}
	var out map[string]string
	if _, err := store.Get(context.Background(), Key{ChainID: 1, Protocol: "demo", Namespace: "markets"}, &out); err == nil {
		t.Fatal("expected get on nil store error")
	}
}

func TestSQLiteStoreRejectsCanceledContext(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.Set(ctx, Key{ChainID: 1, Protocol: "demo", Namespace: "markets"}, map[string]string{"id": "x"}, core.MetadataInfo{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled set, got %v", err)
	}
	var out map[string]string
	_, err = store.Get(ctx, Key{ChainID: 1, Protocol: "demo", Namespace: "markets"}, &out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled get, got %v", err)
	}
}

func TestSQLiteStoreRejectsBadValueAndOutput(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	err := store.Set(context.Background(), Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "markets",
	}, func() {}, core.MetadataInfo{})
	if err == nil || !strings.Contains(err.Error(), "marshal cache data") {
		t.Fatalf("expected marshal error, got %v", err)
	}

	_, err = store.Get(context.Background(), Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "markets",
	}, nil)
	if err == nil {
		t.Fatal("expected nil output error")
	}
}

func TestSQLiteStorePersistsAcrossConnections(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metadata.sqlite")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	err = store.Set(context.Background(), Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "markets",
	}, map[string]string{"id": "weth-usdc"}, core.MetadataInfo{Version: "v1"})
	if err != nil {
		t.Fatalf("set cache: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close cache: %v", err)
	}

	reopened, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	var got map[string]string
	_, err = reopened.Get(context.Background(), Key{
		ChainID:   1,
		Protocol:  "demo",
		Namespace: "markets",
	}, &got)
	if err != nil {
		t.Fatalf("get reopened cache: %v", err)
	}
	if got["id"] != "weth-usdc" {
		t.Fatalf("unexpected reopened cache data: %#v", got)
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()

	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "metadata.sqlite"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
