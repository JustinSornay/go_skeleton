package project

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/platform/audit"
)

// Transactor runs fn atomically; implemented by database.DB.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service implements the project use cases. Dependencies are interfaces
// injected through the constructor: tests swap them for in-memory fakes.
type Service struct {
	repo  Repository
	tx    Transactor
	audit audit.Recorder
	now   func() time.Time
	newID func() uuid.UUID
}

// Option customises a Service (tests).
type Option func(*Service)

// WithClock overrides time.Now.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithIDGenerator overrides uuid.NewV7.
func WithIDGenerator(f func() uuid.UUID) Option { return func(s *Service) { s.newID = f } }

// NewService wires the use cases.
func NewService(repo Repository, tx Transactor, rec audit.Recorder, opts ...Option) *Service {
	s := &Service{
		repo:  repo,
		tx:    tx,
		audit: rec,
		// PostgreSQL stores microseconds: truncate so the returned value
		// equals what a later read returns.
		now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		// UUIDv7 is time-ordered: inserts stay local in the primary key index.
		newID: uuid.NewV7,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// CreateInput carries the fields of a new project.
type CreateInput struct {
	Name        string
	Description string
}

// Create adds a project for ownerID and audits it in the same transaction.
func (s *Service) Create(ctx context.Context, ownerID string, in CreateInput) (Project, error) {
	p, err := New(s.newID(), ownerID, in.Name, in.Description, s.now())
	if err != nil {
		return Project{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, p); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Event{Action: "project.create", TargetType: "project", TargetID: p.ID.String()})
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// Get returns one of ownerID's projects.
func (s *Service) Get(ctx context.Context, ownerID string, id uuid.UUID) (Project, error) {
	return s.repo.Get(ctx, ownerID, id)
}

// List returns a page of ownerID's projects.
func (s *Service) List(ctx context.Context, q ListQuery) ([]Project, int, error) {
	return s.repo.List(ctx, q)
}

// Update applies a partial update.
func (s *Service) Update(ctx context.Context, ownerID string, id uuid.UUID, pt Patch) (Project, error) {
	var updated Project
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
		return s.audit.Record(ctx, audit.Event{Action: "project.update", TargetType: "project", TargetID: id.String()})
	})
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return updated, nil
}

// Delete removes a project and audits it atomically.
func (s *Service) Delete(ctx context.Context, ownerID string, id uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, ownerID, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Event{Action: "project.delete", TargetType: "project", TargetID: id.String()})
	})
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}
