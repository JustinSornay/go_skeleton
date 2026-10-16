// Package repositorytest is the test kit of the repository layer:
// handwritten in-memory fakes of the model interfaces (no generated mocks)
// for the unit tests of services and controllers, and the contracts every
// implementation, fake or PostgreSQL, must pass.
package repositorytest

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/model"
)

// ProjectRepository is a goroutine-safe fake model.ProjectRepository. It mirrors
// the Postgres semantics: owner scoping, case-insensitive unique names and
// newest-first ordering.
type ProjectRepository struct {
	mu       sync.Mutex
	projects map[uuid.UUID]model.Project

	// Err, when set, is returned by every method (failure injection).
	Err error
}

// NewProjectRepository returns an empty repository.
func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{projects: make(map[uuid.UUID]model.Project)}
}

var _ model.ProjectRepository = (*ProjectRepository)(nil)

func (r *ProjectRepository) Create(_ context.Context, p model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.nameTaken(p) {
		return model.ErrProjectNameTaken
	}
	r.projects[p.ID] = p
	return nil
}

func (r *ProjectRepository) Get(_ context.Context, ownerID string, id uuid.UUID) (model.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return model.Project{}, r.Err
	}
	p, ok := r.projects[id]
	if !ok || p.OwnerID != ownerID {
		return model.Project{}, model.ErrProjectNotFound
	}
	return p, nil
}

func (r *ProjectRepository) List(_ context.Context, q model.ProjectListQuery) ([]model.Project, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, 0, r.Err
	}
	var all []model.Project
	for _, p := range r.projects {
		if p.OwnerID != q.OwnerID {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(q.Search)) {
			continue
		}
		all = append(all, p)
	}
	slices.SortFunc(all, func(a, b model.Project) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID.String(), a.ID.String())
	})
	total := len(all)
	start := min(q.Offset, total)
	end := min(start+q.Limit, total)
	return all[start:end], total, nil
}

func (r *ProjectRepository) Update(_ context.Context, p model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	cur, ok := r.projects[p.ID]
	if !ok || cur.OwnerID != p.OwnerID {
		return model.ErrProjectNotFound
	}
	if r.nameTaken(p) {
		return model.ErrProjectNameTaken
	}
	r.projects[p.ID] = p
	return nil
}

func (r *ProjectRepository) Delete(_ context.Context, ownerID string, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	p, ok := r.projects[id]
	if !ok || p.OwnerID != ownerID {
		return model.ErrProjectNotFound
	}
	delete(r.projects, id)
	return nil
}

func (r *ProjectRepository) nameTaken(p model.Project) bool {
	for _, o := range r.projects {
		if o.ID != p.ID && o.OwnerID == p.OwnerID && strings.EqualFold(o.Name, p.Name) {
			return true
		}
	}
	return false
}

// Tx runs fn directly: the in-memory repository has no transactions.
type Tx struct{}

// WithinTx implements model.Transactor.
func (Tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }
