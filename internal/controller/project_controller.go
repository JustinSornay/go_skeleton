package controller

import (
	"context"
	"net/http"
	"uuid"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/dto"
	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/service"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
)

// ProjectService is what ProjectController needs from the business logic
// (consumer-side interface); *service.ProjectService implements it.
type ProjectService interface {
	Create(ctx context.Context, ownerID string, in service.CreateProjectInput) (model.Project, error)
	Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Project, error)
	List(ctx context.Context, q model.ProjectListQuery) ([]model.Project, int, error)
	Update(ctx context.Context, ownerID string, id uuid.UUID, pt model.ProjectPatch) (model.Project, error)
	Delete(ctx context.Context, ownerID string, id uuid.UUID) error
}

// ProjectController serves /v1/projects. Routes require an authenticated
// principal; the router mounts them behind middleware.RequireAuth.
type ProjectController struct {
	svc ProjectService
}

// NewProjectController returns a project controller.
func NewProjectController(svc ProjectService) *ProjectController {
	return &ProjectController{svc: svc}
}

// Routes returns the controller's router, to be mounted by internal/app.
func (c *ProjectController) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", httpx.Handle(c.list))
	r.Post("/", httpx.Handle(c.create))
	r.Get("/{id}", httpx.Handle(c.get))
	r.Patch("/{id}", httpx.Handle(c.update))
	r.Delete("/{id}", httpx.Handle(c.delete))
	return r
}

func (c *ProjectController) list(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	var req dto.ListProjectsRequest
	if err := httpx.BindQuery(r, &req); err != nil {
		return err
	}
	page := req.Normalize()
	items, total, err := c.svc.List(r.Context(), model.ProjectListQuery{
		OwnerID: owner, Search: req.Search, Limit: page.Limit, Offset: page.Offset,
	})
	if err != nil {
		return err
	}
	out := httpx.List[dto.ProjectResponse]{
		Data: make([]dto.ProjectResponse, len(items)),
		Page: httpx.Page{Limit: page.Limit, Offset: page.Offset, Total: total},
	}
	for i, p := range items {
		out.Data[i] = dto.ToProjectResponse(p)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (c *ProjectController) create(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	req, err := httpx.Bind[dto.CreateProjectRequest](r)
	if err != nil {
		return err
	}
	p, err := c.svc.Create(r.Context(), owner, service.CreateProjectInput{Name: req.Name, Description: req.Description})
	if err != nil {
		return err
	}
	w.Header().Set("Location", r.URL.Path+"/"+p.ID.String())
	return httpx.JSON(w, http.StatusCreated, dto.ToProjectResponse(p))
}

func (c *ProjectController) get(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return model.ErrProjectNotFound
	}
	p, err := c.svc.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, dto.ToProjectResponse(p))
}

func (c *ProjectController) update(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return model.ErrProjectNotFound
	}
	req, err := httpx.Bind[dto.UpdateProjectRequest](r)
	if err != nil {
		return err
	}
	p, err := c.svc.Update(r.Context(), owner, id, model.ProjectPatch{Name: req.Name, Description: req.Description})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, dto.ToProjectResponse(p))
}

func (c *ProjectController) delete(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return model.ErrProjectNotFound
	}
	if err := c.svc.Delete(r.Context(), owner, id); err != nil {
		return err
	}
	return httpx.NoContent(w)
}
