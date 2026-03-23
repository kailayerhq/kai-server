// Package blobstore provides pluggable blob storage for segments.
package blobstore

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// Store is the interface for blob storage backends.
type Store interface {
	// Put stores a blob and returns its key.
	Put(ctx context.Context, repoID string, key string, data []byte) error
	// Get retrieves a blob by key.
	Get(ctx context.Context, repoID string, key string) ([]byte, error)
	// GetRange retrieves a byte range from a blob.
	GetRange(ctx context.Context, repoID string, key string, offset, length int64) ([]byte, error)
	// Delete removes a blob.
	Delete(ctx context.Context, repoID string, key string) error
}

// InlineStore stores blobs inline in the database (no external storage).
// This is the default when no external blob store is configured.
type InlineStore struct{}

func (s *InlineStore) Put(ctx context.Context, repoID, key string, data []byte) error {
	return nil // no-op, data stored in DB
}

func (s *InlineStore) Get(ctx context.Context, repoID, key string) ([]byte, error) {
	return nil, fmt.Errorf("inline store: blob should be read from DB")
}

func (s *InlineStore) GetRange(ctx context.Context, repoID, key string, offset, length int64) ([]byte, error) {
	return nil, fmt.Errorf("inline store: blob should be read from DB")
}

func (s *InlineStore) Delete(ctx context.Context, repoID, key string) error {
	return nil // no-op
}

// Global blob store instance
var (
	globalStore Store = &InlineStore{}
	storeMu     sync.RWMutex
)

// SetGlobal sets the global blob store.
func SetGlobal(s Store) {
	storeMu.Lock()
	globalStore = s
	storeMu.Unlock()
}

// Global returns the global blob store.
func Global() Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return globalStore
}

// IsExternal returns true if the global store is not inline.
func IsExternal() bool {
	storeMu.RLock()
	defer storeMu.RUnlock()
	_, isInline := globalStore.(*InlineStore)
	return !isInline
}

// SegmentKey returns the storage key for a segment.
func SegmentKey(segmentID int64) string {
	return fmt.Sprintf("segments/%d.zst", segmentID)
}

// Ensure io import is used
var _ io.Reader
