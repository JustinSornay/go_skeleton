// Package dto holds the request and response types of the HTTP API: the
// public JSON contract (documented in api/openapi.yaml). They are distinct
// from the model types so the model can evolve without breaking clients.
// Validation tags check the request shape; business invariants are enforced
// again by the model.
package dto

import (
	"time"

	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
)

// CreateProjectRequest is the body of POST /v1/projects.
type CreateProjectRequest struct {
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=2000"`
}

// UpdateProjectRequest is a partial update: absent fields are left unchanged.
type UpdateProjectRequest struct {
	Name        *string `json:"name" validate:"omitnil,min=1,max=100"`
	Description *string `json:"description" validate:"omitnil,max=2000"`
}

// ListProjectsRequest is the query string of GET /v1/projects.
type ListProjectsRequest struct {
	httpx.PageQuery
	Search string `query:"q" validate:"max=100"`
}

// ProjectResponse is the JSON representation of a project.
type ProjectResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ToProjectResponse maps a model.Project to its JSON representation.
func ToProjectResponse(p model.Project) ProjectResponse {
	return ProjectResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
