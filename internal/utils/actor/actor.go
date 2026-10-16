// Package actor identifies who triggers an action (a user, a cron job, the
// system). It is set once at the edge — the auth middleware for HTTP requests,
// main() for batch jobs — and read by persistence and audit code, so "who did
// this" never has to be threaded through every function signature.
package actor

import "context"

// Type values are persisted (audit_log.actor_type): never rename them.
const (
	TypeUser   = "user"
	TypeCron   = "cron"
	TypeSystem = "system"
)

// Actor identifies the author of an action.
type Actor struct {
	Type string // one of the Type* constants
	ID   string // user subject, job name, or "" for the system
}

// System is returned when no actor was set: never a zero value.
var System = Actor{Type: TypeSystem}

// User returns the actor for an authenticated user.
func User(subject string) Actor { return Actor{Type: TypeUser, ID: subject} }

// Cron returns the actor for a scheduled job.
func Cron(job string) Actor { return Actor{Type: TypeCron, ID: job} }

type ctxKey struct{}

// With returns a copy of ctx carrying a.
func With(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// From returns the actor stored in ctx, or System.
func From(ctx context.Context) Actor {
	if a, ok := ctx.Value(ctxKey{}).(Actor); ok {
		return a
	}
	return System
}
