// Package httpapi exposes the project use cases over HTTP.
package httpapi

import (
	"context"
	"net/http"
	"uuid"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/auth"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/project"
)

// Service is what the handler needs from the domain (consumer-side
// interface); *project.Service implements it.
type Service interface {
	Create(ctx context.Context, ownerID string, in project.CreateInput) (project.Project, error)
	Get(ctx context.Context, ownerID string, id uuid.UUID) (project.Project, error)
	List(ctx context.Context, q project.ListQuery) ([]project.Project, int, error)
	Update(ctx context.Context, ownerID string, id uuid.UUID, pt project.Patch) (project.Project, error)
	Delete(ctx context.Context, ownerID string, id uuid.UUID) error
}

// Handler serves /v1/projects. Routes require an authenticated principal;
// the router mounts them behind middleware.RequireAuth.
type Handler struct {
	svc Service
}

// NewHandler returns a project handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// Routes returns the module router, to be mounted by internal/app.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", httpx.Handle(h.list))
	r.Post("/", httpx.Handle(h.create))
	r.Get("/{id}", httpx.Handle(h.get))
	r.Patch("/{id}", httpx.Handle(h.update))
	r.Delete("/{id}", httpx.Handle(h.delete))
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	owner, err := ownerID(r)
	if err != nil {
		return err
	}
	var req listRequest
	if err := httpx.BindQuery(r, &req); err != nil {
		return err
	}
	page := req.Normalize()
	items, total, err := h.svc.List(r.Context(), project.ListQuery{
		OwnerID: owner, Search: req.Search, Limit: page.Limit, Offset: page.Offset,
	})
	if err != nil {
		return err
	}
	out := httpx.List[projectResponse]{
		Data: make([]projectResponse, len(items)),
		Page: httpx.Page{Limit: page.Limit, Offset: page.Offset, Total: total},
	}
	for i, p := range items {
		out.Data[i] = toResponse(p)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	owner, err := ownerID(r)
	if err != nil {
		return err
	}
	req, err := httpx.Bind[createRequest](r)
	if err != nil {
		return err
	}
	p, err := h.svc.Create(r.Context(), owner, project.CreateInput{Name: req.Name, Description: req.Description})
	if err != nil {
		return err
	}
	w.Header().Set("Location", r.URL.Path+"/"+p.ID.String())
	return httpx.JSON(w, http.StatusCreated, toResponse(p))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	owner, err := ownerID(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return project.ErrNotFound
	}
	p, err := h.svc.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toResponse(p))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	owner, err := ownerID(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return project.ErrNotFound
	}
	req, err := httpx.Bind[updateRequest](r)
	if err != nil {
		return err
	}
	p, err := h.svc.Update(r.Context(), owner, id, project.Patch{Name: req.Name, Description: req.Description})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toResponse(p))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	owner, err := ownerID(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return project.ErrNotFound
	}
	if err := h.svc.Delete(r.Context(), owner, id); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

// ownerID returns the authenticated subject. RequireAuth guarantees it in
// production; the check keeps the handler safe if it is ever mounted
// without that middleware.
func ownerID(r *http.Request) (string, error) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		return "", apperr.Unauthenticated("auth.required", "authentication is required")
	}
	return p.Subject, nil
}
