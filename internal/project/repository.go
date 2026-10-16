package project

import (
	"context"
	"uuid"
)

// Repository is the persistence port, implemented by project/postgres.
// Every read and write is scoped by owner: a project owned by someone else
// is reported as ErrNotFound, never as forbidden (no existence leak).
type Repository interface {
	// Create inserts p. Returns ErrNameTaken if the owner already has a
	// project with the same name (case-insensitive).
	Create(ctx context.Context, p Project) error
	// Get returns ErrNotFound if the project does not exist for this owner.
	Get(ctx context.Context, ownerID string, id uuid.UUID) (Project, error)
	// List returns one page of the owner's projects, newest first, and the
	// total number of matching projects.
	List(ctx context.Context, q ListQuery) ([]Project, int, error)
	// Update persists name, description and updated_at.
	// Returns ErrNotFound or ErrNameTaken.
	Update(ctx context.Context, p Project) error
	// Delete returns ErrNotFound if nothing was deleted.
	Delete(ctx context.Context, ownerID string, id uuid.UUID) error
}

// ListQuery filters and paginates List.
type ListQuery struct {
	OwnerID string
	Search  string // case-insensitive substring of the name; empty: no filter
	Limit   int
	Offset  int
}
