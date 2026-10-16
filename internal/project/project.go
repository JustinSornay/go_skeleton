// Package project is the example business module. It shows the layout every
// module follows:
//
//	project/            domain + use cases (this package): no HTTP, no SQL
//	project/httpapi     HTTP adapter: DTOs, handlers, routes
//	project/postgres    persistence adapter implementing Repository
//	project/projecttest in-memory Repository for tests
//
// This package may import only the standard library and the stable platform
// kernel (apperr, actor, audit events). The architecture test enforces it.
package project

import (
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
)

// Limits shared by the domain and the database constraints.
const (
	MaxNameLength        = 100
	MaxDescriptionLength = 2000
)

// Domain errors. They are sentinels: compare with errors.Is.
var (
	ErrNotFound  = apperr.NotFound("project.not_found", "project not found")
	ErrNameTaken = apperr.Conflict("project.name_taken", "a project with this name already exists")
	ErrInvalid   = apperr.Invalid("project.invalid", "project is invalid")
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

// New builds a valid Project, normalising user input.
func New(id uuid.UUID, ownerID, name, description string, now time.Time) (Project, error) {
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

// Patch is a partial update: nil fields are left unchanged.
type Patch struct {
	Name        *string
	Description *string
}

// Empty reports whether the patch changes nothing.
func (pt Patch) Empty() bool { return pt.Name == nil && pt.Description == nil }

// Apply returns a copy of p with the patch applied and validated.
func (p Project) Apply(pt Patch, now time.Time) (Project, error) {
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
	case n > MaxNameLength:
		fields = append(fields, apperr.FieldError{Field: "name", Rule: "max", Message: "must be at most 100 characters"})
	}
	if utf8.RuneCountInString(p.Description) > MaxDescriptionLength {
		fields = append(fields, apperr.FieldError{Field: "description", Rule: "max", Message: "must be at most 2000 characters"})
	}
	if len(fields) > 0 {
		return ErrInvalid.WithFields(fields...)
	}
	return nil
}
