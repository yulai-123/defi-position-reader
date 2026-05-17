package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/core"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite cache path is empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite cache dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite cache: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Set(ctx context.Context, key Key, value any, info core.MetadataInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite cache is nil")
	}

	key, err := normalizeKey(key)
	if err != nil {
		return err
	}
	info = normalizeMetadataInfo(key, info)

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cache data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO metadata_cache (
	chain_id, protocol, namespace, version, block_number, updated_at, source, data
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chain_id, protocol, namespace) DO UPDATE SET
	version = excluded.version,
	block_number = excluded.block_number,
	updated_at = excluded.updated_at,
	source = excluded.source,
	data = excluded.data
`, key.ChainID, key.Protocol, key.Namespace, info.Version, info.BlockNumber, info.UpdatedAt.Format(time.RFC3339Nano), info.Source, string(data))
	if err != nil {
		return fmt.Errorf("write sqlite cache: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, key Key, out any) (core.MetadataInfo, error) {
	if err := ctx.Err(); err != nil {
		return core.MetadataInfo{}, err
	}
	if s == nil || s.db == nil {
		return core.MetadataInfo{}, fmt.Errorf("sqlite cache is nil")
	}
	if out == nil {
		return core.MetadataInfo{}, fmt.Errorf("read cache: output target is nil")
	}

	key, err := normalizeKey(key)
	if err != nil {
		return core.MetadataInfo{}, err
	}

	var info core.MetadataInfo
	var updatedAt string
	var data string
	err = s.db.QueryRowContext(ctx, `
SELECT chain_id, protocol, namespace, version, block_number, updated_at, source, data
FROM metadata_cache
WHERE chain_id = ? AND protocol = ? AND namespace = ?
`, key.ChainID, key.Protocol, key.Namespace).Scan(
		&info.ChainID,
		&info.Protocol,
		&info.Namespace,
		&info.Version,
		&info.BlockNumber,
		&updatedAt,
		&info.Source,
		&data,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.MetadataInfo{}, ErrNotFound
		}
		return core.MetadataInfo{}, fmt.Errorf("read sqlite cache: %w", err)
	}

	info.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return core.MetadataInfo{}, fmt.Errorf("parse cache updatedAt: %w", err)
	}
	if err := json.Unmarshal([]byte(data), out); err != nil {
		return core.MetadataInfo{}, fmt.Errorf("decode cache data: %w", err)
	}
	return info, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS metadata_cache (
	chain_id INTEGER NOT NULL,
	protocol TEXT NOT NULL,
	namespace TEXT NOT NULL,
	version TEXT NOT NULL,
	block_number INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT '',
	data TEXT NOT NULL,
	PRIMARY KEY (chain_id, protocol, namespace)
)
`)
	if err != nil {
		return fmt.Errorf("migrate sqlite cache: %w", err)
	}
	return nil
}

func normalizeKey(key Key) (Key, error) {
	if key.ChainID == 0 {
		return Key{}, fmt.Errorf("cache key chain id is empty")
	}

	protocol, err := normalizeKeyPart(key.Protocol)
	if err != nil {
		return Key{}, fmt.Errorf("cache key protocol: %w", err)
	}
	namespace, err := normalizeKeyPart(key.Namespace)
	if err != nil {
		return Key{}, fmt.Errorf("cache key namespace: %w", err)
	}

	key.Protocol = protocol
	key.Namespace = namespace
	return key, nil
}

func normalizeKeyPart(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("value is empty")
	}
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("value contains null byte")
	}
	return value, nil
}

func normalizeMetadataInfo(key Key, info core.MetadataInfo) core.MetadataInfo {
	info.ChainID = key.ChainID
	info.Protocol = key.Protocol
	info.Namespace = key.Namespace
	if info.UpdatedAt.IsZero() {
		info.UpdatedAt = time.Now().UTC()
	} else {
		info.UpdatedAt = info.UpdatedAt.UTC()
	}
	return info
}
