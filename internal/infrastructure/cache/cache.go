// Package cache provides a small key/value cache abstraction with Redis and
// in-memory implementations, plus a read-through helper.
//
// Typical use is a decorator around a slow dependency (an external API
// client, an expensive aggregation): the decorator implements the same
// interface as the wrapped component and calls ReadThrough. Cache failures
// are logged and bypassed — the cache must never take the service down.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
)

// ErrMiss is returned by Get when the key does not exist.
var ErrMiss = errors.New("cache: miss")

// Store is a byte-oriented cache.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Ping(ctx context.Context) error
}

// GetJSON decodes the cached JSON value at key into dst.
func GetJSON(ctx context.Context, s Store, key string, dst any) error {
	b, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("cache: decoding %q: %w", key, err)
	}
	return nil
}

// SetJSON stores v as JSON at key.
func SetJSON(ctx context.Context, s Store, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("cache: encoding %q: %w", key, err)
	}
	return s.Set(ctx, key, b, ttl)
}

// Loader implements cache-aside reads for values of type T: check the cache,
// otherwise call fetch and populate the cache. Concurrent misses on the same
// key share a single fetch (no thundering herd on a cold key).
type Loader[T any] struct {
	store Store
	ttl   time.Duration
	group singleflight.Group
}

// NewLoader returns a Loader storing values for ttl.
func NewLoader[T any](store Store, ttl time.Duration) *Loader[T] {
	return &Loader[T]{store: store, ttl: ttl}
}

// Get returns the cached value for key or loads it with fetch. Errors from
// fetch are returned as-is and never cached.
func (l *Loader[T]) Get(ctx context.Context, key string, fetch func(ctx context.Context) (T, error)) (T, error) {
	var v T
	err := GetJSON(ctx, l.store, key, &v)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, ErrMiss) {
		logging.FromContext(ctx).LogAttrs(ctx, slog.LevelWarn, "cache read failed", slog.String("key", key), slog.Any("error", err))
	}

	res, err, _ := l.group.Do(key, func() (any, error) {
		// The shared fetch must not be cancelled because the first caller
		// went away; it is still bounded by fetch's own timeouts.
		fctx := context.WithoutCancel(ctx)
		val, err := fetch(fctx)
		if err != nil {
			return val, err
		}
		if err := SetJSON(fctx, l.store, key, val, l.ttl); err != nil {
			logging.FromContext(ctx).LogAttrs(ctx, slog.LevelWarn, "cache write failed", slog.String("key", key), slog.Any("error", err))
		}
		return val, nil
	})
	if err != nil {
		return v, err
	}
	return res.(T), nil
}

// Forget deletes key (call it after the underlying data changed).
func (l *Loader[T]) Forget(ctx context.Context, key string) error {
	return l.store.Delete(ctx, key)
}
