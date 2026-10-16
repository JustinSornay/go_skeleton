// Package service implements the use cases (business logic). Services depend
// only on the interfaces declared in internal/model, injected through their
// constructor: tests swap them for the in-memory fakes of
// internal/repository/repositorytest. No HTTP, no SQL.
package service

import (
	"time"
	"uuid"
)

// options are the dependencies every service may override (tests).
type options struct {
	now   func() time.Time
	newID func() uuid.UUID
}

// Option customises a service (tests).
type Option func(*options)

// WithClock overrides time.Now.
func WithClock(now func() time.Time) Option { return func(o *options) { o.now = now } }

// WithIDGenerator overrides uuid.NewV7.
func WithIDGenerator(f func() uuid.UUID) Option { return func(o *options) { o.newID = f } }

func newOptions(opts []Option) options {
	o := options{
		// PostgreSQL stores microseconds: truncate so the returned value
		// equals what a later read returns.
		now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		// UUIDv7 is time-ordered: inserts stay local in the primary key index.
		newID: uuid.NewV7,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
