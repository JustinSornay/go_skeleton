// Package projecttest provides an in-memory project.Repository for unit
// tests of the service and of the HTTP adapter (handwritten fake, no mocks).
package projecttest

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/project"
)

// Repository is a goroutine-safe in-memory project.Repository. It mirrors
// the Postgres semantics: owner scoping, case-insensitive unique names and
// newest-first ordering.
type Repository struct {
	mu       sync.Mutex
	projects map[uuid.UUID]project.Project

	// Err, when set, is returned by every method (failure injection).
	Err error
}

// NewRepository returns an empty repository.
func NewRepository() *Repository {
	return &Repository{projects: make(map[uuid.UUID]project.Project)}
}

var _ project.Repository = (*Repository)(nil)

func (r *Repository) Create(_ context.Context, p project.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.nameTaken(p) {
		return project.ErrNameTaken
	}
	r.projects[p.ID] = p
	return nil
}

func (r *Repository) Get(_ context.Context, ownerID string, id uuid.UUID) (project.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return project.Project{}, r.Err
	}
	p, ok := r.projects[id]
	if !ok || p.OwnerID != ownerID {
		return project.Project{}, project.ErrNotFound
	}
	return p, nil
}

func (r *Repository) List(_ context.Context, q project.ListQuery) ([]project.Project, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, 0, r.Err
	}
	var all []project.Project
	for _, p := range r.projects {
		if p.OwnerID != q.OwnerID {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(q.Search)) {
			continue
		}
		all = append(all, p)
	}
	slices.SortFunc(all, func(a, b project.Project) int {
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

func (r *Repository) Update(_ context.Context, p project.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	cur, ok := r.projects[p.ID]
	if !ok || cur.OwnerID != p.OwnerID {
		return project.ErrNotFound
	}
	if r.nameTaken(p) {
		return project.ErrNameTaken
	}
	r.projects[p.ID] = p
	return nil
}

func (r *Repository) Delete(_ context.Context, ownerID string, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	p, ok := r.projects[id]
	if !ok || p.OwnerID != ownerID {
		return project.ErrNotFound
	}
	delete(r.projects, id)
	return nil
}

func (r *Repository) nameTaken(p project.Project) bool {
	for _, o := range r.projects {
		if o.ID != p.ID && o.OwnerID == p.OwnerID && strings.EqualFold(o.Name, p.Name) {
			return true
		}
	}
	return false
}

// Tx runs fn directly: the in-memory repository has no transactions.
type Tx struct{}

// WithinTx implements project.Transactor.
func (Tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }
