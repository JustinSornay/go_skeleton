package cache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/cache"
)

// ExampleLoader reads through the cache (cache-aside): the first Get misses
// and calls fetch, the second one is served from the store.
func ExampleLoader() {
	ctx := context.Background()
	loader := cache.NewLoader[string](cache.NewMemory(), time.Hour)

	fetch := func(context.Context) (string, error) {
		fmt.Println("fetching from the origin")
		return "Paris", nil
	}
	for range 2 {
		city, err := loader.Get(ctx, "geocode:48.8566,2.3522", fetch)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(city)
	}
	// Output:
	// fetching from the origin
	// Paris
	// Paris
}

// geocoder is a slow dependency, e.g. an HTTP client living in its own
// infrastructure/<name> package.
type geocoder interface {
	City(ctx context.Context, lat, lon float64) (string, error)
}

type slowGeocoder struct{ calls int }

func (g *slowGeocoder) City(context.Context, float64, float64) (string, error) {
	g.calls++
	return "Paris", nil
}

// cachedGeocoder is a decorator: it implements the same interface as the
// client it wraps, so the caller cannot tell whether the cache is there.
type cachedGeocoder struct {
	next   geocoder
	loader *cache.Loader[string]
}

func (c cachedGeocoder) City(ctx context.Context, lat, lon float64) (string, error) {
	key := fmt.Sprintf("geocode:%.4f,%.4f", lat, lon) // rounding = spatial bucketing
	return c.loader.Get(ctx, key, func(ctx context.Context) (string, error) {
		return c.next.City(ctx, lat, lon)
	})
}

// Example_decorator adds caching to a client without touching it. Errors
// from the client are never cached, and a broken store only costs a call to
// the origin.
func Example_decorator() {
	origin := &slowGeocoder{}
	var g geocoder = cachedGeocoder{next: origin, loader: cache.NewLoader[string](cache.NewMemory(), time.Hour)}

	ctx := context.Background()
	for _, lat := range []float64{48.85661, 48.85664, 45.76400} {
		city, _ := g.City(ctx, lat, 2.3522)
		fmt.Println(city, origin.calls)
	}
	// Output:
	// Paris 1
	// Paris 1
	// Paris 2
}
