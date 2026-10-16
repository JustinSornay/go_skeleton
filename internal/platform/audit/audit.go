// Package audit records sensitive actions (who did what, to what, when) in
// the audit_log table.
//
// Record uses the transaction bound to the context when there is one, so an
// audit entry is committed or rolled back together with the change it
// describes. Its error must therefore be returned, not ignored.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JustinSornay/go_skeleton/internal/platform/actor"
	"github.com/JustinSornay/go_skeleton/internal/platform/database"
	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

// Event describes an action. The actor and request ID come from the context.
type Event struct {
	Action     string // e.g. "project.delete" (verb last, stable)
	TargetType string // e.g. "project"
	TargetID   string
	Metadata   map[string]any // small, non-sensitive details
}

// Recorder persists events.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// PostgresRecorder writes events to audit_log.
type PostgresRecorder struct {
	db *database.DB
}

// NewPostgresRecorder returns a Recorder backed by db.
func NewPostgresRecorder(db *database.DB) *PostgresRecorder {
	return &PostgresRecorder{db: db}
}

// Record implements Recorder.
func (r *PostgresRecorder) Record(ctx context.Context, e Event) error {
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("audit: encoding metadata: %w", err)
	}
	a := actor.From(ctx)
	_, err = r.db.Q(ctx).Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, target_type, target_id, request_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.Type, a.ID, e.Action, e.TargetType, e.TargetID, requestid.From(ctx), b)
	if err != nil {
		return fmt.Errorf("audit: recording %s: %w", e.Action, err)
	}
	return nil
}

// Discard is a Recorder that drops events (unit tests).
type Discard struct{}

// Record implements Recorder.
func (Discard) Record(context.Context, Event) error { return nil }
