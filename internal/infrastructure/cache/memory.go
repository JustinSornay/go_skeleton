package cache

import (
	"context"
	"sync"
	"time"
)

// Memory is an in-process Store for development and tests. It is not shared
// between replicas and holds everything in RAM: use Redis in production.
type Memory struct {
	mu    sync.Mutex
	items map[string]memoryItem
	now   func() time.Time
}

type memoryItem struct {
	value     []byte
	expiresAt time.Time // zero: never expires
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{items: make(map[string]memoryItem), now: time.Now}
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[key]
	if !ok {
		return nil, ErrMiss
	}
	if !it.expiresAt.IsZero() && !m.now().Before(it.expiresAt) {
		delete(m.items, key)
		return nil, ErrMiss
	}
	return append([]byte(nil), it.value...), nil
}

func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := memoryItem{value: append([]byte(nil), value...)}
	if ttl > 0 {
		it.expiresAt = m.now().Add(ttl)
	}
	m.items[key] = it
	return nil
}

func (m *Memory) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.items, k)
	}
	return nil
}

func (m *Memory) Ping(context.Context) error { return nil }
