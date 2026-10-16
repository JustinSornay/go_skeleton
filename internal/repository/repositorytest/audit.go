package repositorytest

import (
	"context"
	"slices"
	"sync"

	"github.com/JustinSornay/go_skeleton/internal/model"
)

// AuditRecorder is a goroutine-safe in-memory model.AuditRecorder that keeps
// the recorded events so tests can assert on them. The zero value is ready
// to use.
type AuditRecorder struct {
	mu     sync.Mutex
	events []model.AuditEvent

	// Err, when set, is returned by Record and nothing is recorded (failure
	// injection).
	Err error
}

var _ model.AuditRecorder = (*AuditRecorder)(nil)

// Record implements model.AuditRecorder.
func (r *AuditRecorder) Record(_ context.Context, e model.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.events = append(r.events, e)
	return nil
}

// Events returns a copy of the recorded events, oldest first.
func (r *AuditRecorder) Events() []model.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}
