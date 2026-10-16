package httpapi

import (
	"time"

	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/project"
)

// DTOs are the public JSON contract (documented in api/openapi.yaml). They
// are distinct from domain types so the domain can evolve without breaking
// clients. Validation tags check the request shape; business invariants are
// enforced again by the domain.

type createRequest struct {
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=2000"`
}

// updateRequest is a partial update: absent fields are left unchanged.
type updateRequest struct {
	Name        *string `json:"name" validate:"omitnil,min=1,max=100"`
	Description *string `json:"description" validate:"omitnil,max=2000"`
}

type listRequest struct {
	httpx.PageQuery
	Search string `query:"q" validate:"max=100"`
}

type projectResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toResponse(p project.Project) projectResponse {
	return projectResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
