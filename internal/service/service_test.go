package service_test

// Helpers shared by every service test of the package. Reuse them in a new
// <entity>_service_test.go; only the <entity>-prefixed helpers belong to one
// feature.

import (
	"time"

	"github.com/JustinSornay/go_skeleton/internal/service"
)

// steppingClock returns a deterministic clock: each call is one second after
// the previous one, starting at 2026-01-02T03:04:06Z.
func steppingClock() service.Option {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return service.WithClock(func() time.Time {
		now = now.Add(time.Second)
		return now
	})
}
