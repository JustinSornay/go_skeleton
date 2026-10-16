package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/utils/actor"
	"github.com/JustinSornay/go_skeleton/internal/utils/requestid"
)

// AuditRepository records sensitive actions (who did what, to what, when)
// in the audit_log table. It implements model.AuditRecorder.
//
// Record uses the transaction bound to the context when there is one, so an
// audit entry is committed or rolled back together with the change it
// describes.
type AuditRepository struct {
	db *database.DB
}

// NewAuditRepository returns an audit recorder backed by db.
func NewAuditRepository(db *database.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

var _ model.AuditRecorder = (*AuditRepository)(nil)

// Record implements model.AuditRecorder. The actor and request ID come from
// the context.
func (r *AuditRepository) Record(ctx context.Context, e model.AuditEvent) error {
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

// CountOlderThan returns the number of entries recorded before cutoff.
func (r *AuditRepository) CountOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE occurred_at < $1`, cutoff).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("audit: counting entries: %w", err)
	}
	return n, nil
}

// DeleteOlderThan deletes at most limit entries recorded before cutoff, oldest
// first, and returns how many it deleted. Callers loop until it deletes fewer
// than limit, so a large backlog never holds long locks.
func (r *AuditRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `
		DELETE FROM audit_log WHERE id IN (
			SELECT id FROM audit_log WHERE occurred_at < $1 ORDER BY id LIMIT $2
		)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("audit: deleting entries: %w", err)
	}
	return tag.RowsAffected(), nil
}
