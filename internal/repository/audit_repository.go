package repository

import (
	"context"
	"encoding/json"
	"fmt"

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
