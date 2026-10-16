// Package model holds the business entities, their invariants and errors,
// and the interfaces (ports) the services depend on: repositories, the
// transactor and the audit recorder. Implementations live elsewhere
// (internal/repository), so this package stays free of HTTP and SQL.
//
// It may import only utils/apperr. The architecture test enforces it.
package model

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

// Limits shared by the domain and the database constraints.
const (
	ProjectMaxNameLength        = 100
	ProjectMaxDescriptionLength = 2000
)

// Project errors. They are sentinels: compare with errors.Is.
var (
	ErrProjectNotFound  = apperr.NotFound("project.not_found", "project not found")
	ErrProjectNameTaken = apperr.Conflict("project.name_taken", "a project with this name already exists")
	ErrProjectInvalid   = apperr.Invalid("project.invalid", "project is invalid")
)

// Project is owned by a single user, identified by the auth subject.
type Project struct {
	ID          uuid.UUID
	OwnerID     string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewProject builds a valid Project, normalising user input.
func NewProject(id uuid.UUID, ownerID, name, description string, now time.Time) (Project, error) {
	p := Project{
		ID:          id,
		OwnerID:     ownerID,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return p, p.validate()
}

// ProjectPatch is a partial update: nil fields are left unchanged.
type ProjectPatch struct {
	Name        *string
	Description *string
}

// Empty reports whether the patch changes nothing.
func (pt ProjectPatch) Empty() bool { return pt.Name == nil && pt.Description == nil }

// Apply returns a copy of p with the patch applied and validated.
func (p Project) Apply(pt ProjectPatch, now time.Time) (Project, error) {
	if pt.Name != nil {
		p.Name = strings.TrimSpace(*pt.Name)
	}
	if pt.Description != nil {
		p.Description = strings.TrimSpace(*pt.Description)
	}
	p.UpdatedAt = now
	return p, p.validate()
}

func (p Project) validate() error {
	var fields []apperr.FieldError
	if p.OwnerID == "" {
		fields = append(fields, apperr.FieldError{Field: "owner_id", Rule: "required", Message: "is required"})
	}
	switch n := utf8.RuneCountInString(p.Name); {
	case n == 0:
		fields = append(fields, apperr.FieldError{Field: "name", Rule: "required", Message: "is required"})
	case n > ProjectMaxNameLength:
		fields = append(fields, apperr.FieldError{Field: "name", Rule: "max", Message: "must be at most 100 characters"})
	}
	if utf8.RuneCountInString(p.Description) > ProjectMaxDescriptionLength {
		fields = append(fields, apperr.FieldError{Field: "description", Rule: "max", Message: "must be at most 2000 characters"})
	}
	if len(fields) > 0 {
		return ErrProjectInvalid.WithFields(fields...)
	}
	return nil
}

// ProjectRepository is the persistence port, implemented by
// repository.ProjectRepository. Every read and write is scoped by owner: a
// project owned by someone else is reported as ErrProjectNotFound, never as
// forbidden (no existence leak).
type ProjectRepository interface {
	// Create inserts p. Returns ErrProjectNameTaken if the owner already has
	// a project with the same name (case-insensitive).
	Create(ctx context.Context, p Project) error
	// Get returns ErrProjectNotFound if the project does not exist for this
	// owner.
	Get(ctx context.Context, ownerID string, id uuid.UUID) (Project, error)
	// List returns one page of the owner's projects, newest first, and the
	// total number of matching projects.
	List(ctx context.Context, q ProjectListQuery) ([]Project, int, error)
	// Update persists name, description and updated_at.
	// Returns ErrProjectNotFound or ErrProjectNameTaken.
	Update(ctx context.Context, p Project) error
	// Delete returns ErrProjectNotFound if nothing was deleted.
	Delete(ctx context.Context, ownerID string, id uuid.UUID) error
}

// ProjectListQuery filters and paginates ProjectRepository.List.
type ProjectListQuery struct {
	OwnerID string
	Search  string // case-insensitive substring of the name; empty: no filter
	Limit   int
	Offset  int
}
