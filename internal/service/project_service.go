package service

import (
	"context"
	"fmt"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/model"
)

// ProjectService implements the project use cases.
type ProjectService struct {
	repo  model.ProjectRepository
	tx    model.Transactor
	audit model.AuditRecorder
	options
}

// NewProjectService wires the project use cases.
func NewProjectService(repo model.ProjectRepository, tx model.Transactor, audit model.AuditRecorder, opts ...Option) *ProjectService {
	return &ProjectService{repo: repo, tx: tx, audit: audit, options: newOptions(opts)}
}

// CreateProjectInput carries the fields of a new project.
type CreateProjectInput struct {
	Name        string
	Description string
}

// Create adds a project for ownerID and audits it in the same transaction.
func (s *ProjectService) Create(ctx context.Context, ownerID string, in CreateProjectInput) (model.Project, error) {
	p, err := model.NewProject(s.newID(), ownerID, in.Name, in.Description, s.now())
	if err != nil {
		return model.Project{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, p); err != nil {
			return err
		}
		return s.audit.Record(ctx, model.AuditEvent{Action: "project.create", TargetType: "project", TargetID: p.ID.String()})
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// Get returns one of ownerID's projects.
func (s *ProjectService) Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Project, error) {
	return s.repo.Get(ctx, ownerID, id)
}

// List returns a page of ownerID's projects.
func (s *ProjectService) List(ctx context.Context, q model.ProjectListQuery) ([]model.Project, int, error) {
	return s.repo.List(ctx, q)
}

// Update applies a partial update.
func (s *ProjectService) Update(ctx context.Context, ownerID string, id uuid.UUID, pt model.ProjectPatch) (model.Project, error) {
	var updated model.Project
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		cur, err := s.repo.Get(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if pt.Empty() {
			updated = cur
			return nil
		}
		if updated, err = cur.Apply(pt, s.now()); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, updated); err != nil {
			return err
		}
		return s.audit.Record(ctx, model.AuditEvent{Action: "project.update", TargetType: "project", TargetID: id.String()})
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("update project: %w", err)
	}
	return updated, nil
}

// Delete removes a project and audits it atomically.
func (s *ProjectService) Delete(ctx context.Context, ownerID string, id uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, ownerID, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, model.AuditEvent{Action: "project.delete", TargetType: "project", TargetID: id.String()})
	})
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}
