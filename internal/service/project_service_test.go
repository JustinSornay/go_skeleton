package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
	"github.com/JustinSornay/go_skeleton/internal/service"
	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

func newProjectService(repo *repositorytest.ProjectRepository, rec *repositorytest.AuditRecorder) *service.ProjectService {
	return service.NewProjectService(repo, repositorytest.Tx{}, rec, steppingClock())
}

func TestCreateProject(t *testing.T) {
	repo, rec := repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{}
	svc := newProjectService(repo, rec)

	p, err := svc.Create(context.Background(), "alice", service.CreateProjectInput{Name: "  Orchard  ", Description: " apples "})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Orchard" || p.Description != "apples" || p.OwnerID != "alice" || p.ID == (uuid.UUID{}) {
		t.Fatalf("unexpected project %+v", p)
	}
	if !p.CreatedAt.Equal(p.UpdatedAt) || p.CreatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", p)
	}
	if events := rec.Events(); len(events) != 1 || events[0].Action != "project.create" || events[0].TargetID != p.ID.String() {
		t.Fatalf("audit events: %+v", events)
	}
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	svc := newProjectService(repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{})
	tests := map[string]service.CreateProjectInput{
		"blank name":       {Name: "   "},
		"name too long":    {Name: strings.Repeat("é", model.ProjectMaxNameLength+1)},
		"description long": {Name: "ok", Description: strings.Repeat("x", model.ProjectMaxDescriptionLength+1)},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), "alice", in)
			if !errors.Is(err, model.ErrProjectInvalid) {
				t.Fatalf("want ErrProjectInvalid, got %v", err)
			}
			if ae, _ := apperr.As(err); len(ae.Fields) == 0 {
				t.Fatal("validation error must list fields")
			}
		})
	}
	// Multi-byte names are measured in characters, not bytes.
	if _, err := svc.Create(context.Background(), "alice", service.CreateProjectInput{Name: strings.Repeat("é", model.ProjectMaxNameLength)}); err != nil {
		t.Fatalf("100 characters must be accepted: %v", err)
	}
}

func TestCreateProjectNameIsUniquePerOwner(t *testing.T) {
	svc := newProjectService(repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "alice", service.CreateProjectInput{Name: "Orchard"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "alice", service.CreateProjectInput{Name: "ORCHARD"}); !errors.Is(err, model.ErrProjectNameTaken) {
		t.Fatalf("want ErrProjectNameTaken, got %v", err)
	}
	if _, err := svc.Create(ctx, "bob", service.CreateProjectInput{Name: "Orchard"}); err != nil {
		t.Fatalf("another owner may reuse the name: %v", err)
	}
}

func TestCreateProjectFailsWhenAuditFails(t *testing.T) {
	boom := errors.New("audit down")
	svc := newProjectService(repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{Err: boom})
	if _, err := svc.Create(context.Background(), "alice", service.CreateProjectInput{Name: "x"}); !errors.Is(err, boom) {
		t.Fatalf("want audit error, got %v", err)
	}
}

func TestUpdateProject(t *testing.T) {
	repo, rec := repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{}
	svc := newProjectService(repo, rec)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "alice", service.CreateProjectInput{Name: "Orchard", Description: "apples"})
	_, _ = svc.Create(ctx, "alice", service.CreateProjectInput{Name: "Vineyard"})

	desc := "pears"
	got, err := svc.Update(ctx, "alice", p.ID, model.ProjectPatch{Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Orchard" || got.Description != "pears" || !got.UpdatedAt.After(p.UpdatedAt) || !got.CreatedAt.Equal(p.CreatedAt) {
		t.Fatalf("partial update: %+v", got)
	}

	// Empty patch: no write, no audit.
	before := len(rec.Events())
	if same, err := svc.Update(ctx, "alice", p.ID, model.ProjectPatch{}); err != nil || !same.UpdatedAt.Equal(got.UpdatedAt) || len(rec.Events()) != before {
		t.Fatalf("empty patch: %+v %v", same, err)
	}

	taken := "vineyard"
	if _, err := svc.Update(ctx, "alice", p.ID, model.ProjectPatch{Name: &taken}); !errors.Is(err, model.ErrProjectNameTaken) {
		t.Fatalf("want ErrProjectNameTaken, got %v", err)
	}
	blank := " "
	if _, err := svc.Update(ctx, "alice", p.ID, model.ProjectPatch{Name: &blank}); !errors.Is(err, model.ErrProjectInvalid) {
		t.Fatalf("want ErrProjectInvalid, got %v", err)
	}
	if _, err := svc.Update(ctx, "bob", p.ID, model.ProjectPatch{Description: &desc}); !errors.Is(err, model.ErrProjectNotFound) {
		t.Fatalf("other owner: want ErrProjectNotFound, got %v", err)
	}
}

func TestDeleteProject(t *testing.T) {
	repo, rec := repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{}
	svc := newProjectService(repo, rec)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "alice", service.CreateProjectInput{Name: "Orchard"})

	if err := svc.Delete(ctx, "bob", p.ID); !errors.Is(err, model.ErrProjectNotFound) {
		t.Fatalf("other owner: %v", err)
	}
	if err := svc.Delete(ctx, "alice", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, "alice", p.ID); !errors.Is(err, model.ErrProjectNotFound) {
		t.Fatalf("deleted project still readable: %v", err)
	}
	events := rec.Events()
	if last := events[len(events)-1]; last.Action != "project.delete" {
		t.Fatalf("audit: %+v", last)
	}
}

func TestListProjects(t *testing.T) {
	svc := newProjectService(repositorytest.NewProjectRepository(), &repositorytest.AuditRecorder{})
	ctx := context.Background()
	for _, n := range []string{"alpha", "beta", "alphabet"} {
		if _, err := svc.Create(ctx, "alice", service.CreateProjectInput{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = svc.Create(ctx, "bob", service.CreateProjectInput{Name: "alpha"})

	items, total, err := svc.List(ctx, model.ProjectListQuery{OwnerID: "alice", Search: "ALPHA", Limit: 1})
	if err != nil || total != 2 || len(items) != 1 || items[0].Name != "alphabet" {
		t.Fatalf("got %v total=%d err=%v", items, total, err)
	}
}
