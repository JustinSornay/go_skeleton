package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a Store backed by Redis. Keys are namespaced with a prefix so
// several services (or environments) can share an instance safely.
type Redis struct {
	client *redis.Client
	prefix string
}

// OpenRedis connects to url (redis://…) and checks connectivity.
func OpenRedis(ctx context.Context, url, prefix string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("cache: parsing redis URL: %w", err)
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache: redis ping: %w", err)
	}
	return NewRedis(client, prefix), nil
}

// NewRedis wraps an existing client.
func NewRedis(client *redis.Client, prefix string) *Redis {
	return &Redis{client: client, prefix: prefix}
}

func (r *Redis) key(k string) string { return r.prefix + k }

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := r.client.Get(ctx, r.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	return b, err
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, r.key(key), value, ttl).Err()
}

func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = r.key(k)
	}
	return r.client.Del(ctx, full...).Err()
}

func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close closes the connection pool.
func (r *Redis) Close() error { return r.client.Close() }
