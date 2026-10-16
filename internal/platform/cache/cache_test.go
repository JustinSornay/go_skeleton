package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/JustinSornay/go_skeleton/internal/platform/cache"
)

func stores(t *testing.T) map[string]cache.Store {
	mr := miniredis.RunT(t)
	return map[string]cache.Store{
		"memory": cache.NewMemory(),
		"redis":  cache.NewRedis(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test:"),
	}
}

func TestStoreContract(t *testing.T) {
	ctx := context.Background()
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Get(ctx, "k"); !errors.Is(err, cache.ErrMiss) {
				t.Fatalf("missing key: %v", err)
			}
			if err := cache.SetJSON(ctx, s, "k", map[string]int{"n": 1}, time.Minute); err != nil {
				t.Fatal(err)
			}
			var got map[string]int
			if err := cache.GetJSON(ctx, s, "k", &got); err != nil || got["n"] != 1 {
				t.Fatalf("got %v, %v", got, err)
			}
			if err := s.Delete(ctx, "k", "absent"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(ctx, "k"); !errors.Is(err, cache.ErrMiss) {
				t.Fatalf("deleted key: %v", err)
			}
			if err := s.Ping(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMemoryExpiry(t *testing.T) {
	ctx := context.Background()
	m := cache.NewMemory()
	_ = m.Set(ctx, "short", []byte("v"), time.Millisecond)
	_ = m.Set(ctx, "forever", []byte("v"), 0)
	time.Sleep(5 * time.Millisecond)
	if _, err := m.Get(ctx, "short"); !errors.Is(err, cache.ErrMiss) {
		t.Errorf("expired entry returned: %v", err)
	}
	if _, err := m.Get(ctx, "forever"); err != nil {
		t.Errorf("ttl 0 must not expire: %v", err)
	}
}

func TestLoader(t *testing.T) {
	ctx := context.Background()
	l := cache.NewLoader[string](cache.NewMemory(), time.Minute)

	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		<-release
		return "value", nil
	}

	// Concurrent misses share one fetch.
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if v, err := l.Get(ctx, "k", fetch); err != nil || v != "value" {
				t.Errorf("got %q, %v", v, err)
			}
		})
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("fetch called %d times, want 1", calls.Load())
	}

	// Hits do not fetch.
	if _, err := l.Get(ctx, "k", fetch); err != nil || calls.Load() != 1 {
		t.Fatalf("cache hit fetched again (%d)", calls.Load())
	}

	// Errors are returned and not cached.
	boom := errors.New("upstream down")
	if _, err := l.Get(ctx, "other", func(context.Context) (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("want upstream error, got %v", err)
	}
	if v, err := l.Get(ctx, "other", func(context.Context) (string, error) { return "ok", nil }); err != nil || v != "ok" {
		t.Fatalf("error was cached: %q %v", v, err)
	}

	_ = l.Forget(ctx, "k")
	if _, err := l.Get(ctx, "k", func(context.Context) (string, error) { calls.Add(1); return "v2", nil }); err != nil || calls.Load() != 2 {
		t.Fatal("Forget did not invalidate")
	}
}

func TestLoaderSurvivesBrokenCache(t *testing.T) {
	mr := miniredis.RunT(t)
	store := cache.NewRedis(redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1}), "")
	mr.Close() // cache down: reads and writes fail
	l := cache.NewLoader[int](store, time.Minute)
	v, err := l.Get(context.Background(), "k", func(context.Context) (int, error) { return 42, nil })
	if err != nil || v != 42 {
		t.Fatalf("a broken cache must fall back to the origin: %d, %v", v, err)
	}
}
