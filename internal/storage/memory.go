package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/agentic-demo/platform/internal/domain"
)

// MemoryObjectStore is an in-memory ObjectStore for unit tests and local
// development. It is safe for concurrent use.
type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string][]byte // full key -> content
}

// NewMemoryObjectStore returns an empty in-memory object store.
func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{objects: make(map[string][]byte)}
}

// Put implements ObjectStore.
func (s *MemoryObjectStore) Put(ctx context.Context, tenantID domain.TenantID, key string, r io.Reader, size int64) error {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[fullKey] = data
	return nil
}

// Get implements ObjectStore.
func (s *MemoryObjectStore) Get(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, error) {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[fullKey]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Delete implements ObjectStore.
func (s *MemoryObjectStore) Delete(ctx context.Context, tenantID domain.TenantID, key string) error {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, fullKey)
	return nil
}

// DeleteTenant implements ObjectStore.
func (s *MemoryObjectStore) DeleteTenant(ctx context.Context, tenantID domain.TenantID) error {
	prefix := tenantKeyPrefix(tenantID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for fullKey := range s.objects {
		if strings.HasPrefix(fullKey, prefix) {
			delete(s.objects, fullKey)
		}
	}
	return nil
}
