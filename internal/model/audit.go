package model

import "context"

// AuditEvent describes a sensitive action (who did what, to what, when).
// The actor and request ID come from the context.
type AuditEvent struct {
	Action     string // e.g. "project.delete" (verb last, stable)
	TargetType string // e.g. "project"
	TargetID   string
	Metadata   map[string]any // small, non-sensitive details
}

// AuditRecorder persists audit events; implemented by
// repository.AuditRepository.
//
// Record uses the transaction bound to the context when there is one, so an
// audit entry is committed or rolled back together with the change it
// describes. Its error must therefore be returned, not ignored.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEvent) error
}
