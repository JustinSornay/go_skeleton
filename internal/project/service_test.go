package project_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/audit"
	"github.com/JustinSornay/go_skeleton/internal/project"
	"github.com/JustinSornay/go_skeleton/internal/project/projecttest"
)

// recorder captures audit events.
type recorder struct {
	events []audit.Event
	err    error
}

func (r *recorder) Record(_ context.Context, e audit.Event) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, e)
	return nil
}

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newService(repo *projecttest.Repository, rec *recorder) *project.Service {
	now := t0
	return project.NewService(repo, projecttest.Tx{}, rec, project.WithClock(func() time.Time {
		now = now.Add(time.Second)
		return now
	}))
}

func TestCreate(t *testing.T) {
	repo, rec := projecttest.NewRepository(), &recorder{}
	svc := newService(repo, rec)

	p, err := svc.Create(context.Background(), "alice", project.CreateInput{Name: "  Orchard  ", Description: " apples "})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Orchard" || p.Description != "apples" || p.OwnerID != "alice" || p.ID == (uuid.UUID{}) {
		t.Fatalf("unexpected project %+v", p)
	}
	if !p.CreatedAt.Equal(p.UpdatedAt) || p.CreatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", p)
	}
	if len(rec.events) != 1 || rec.events[0].Action != "project.create" || rec.events[0].TargetID != p.ID.String() {
		t.Fatalf("audit events: %+v", rec.events)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	svc := newService(projecttest.NewRepository(), &recorder{})
	tests := map[string]project.CreateInput{
		"blank name":       {Name: "   "},
		"name too long":    {Name: strings.Repeat("é", project.MaxNameLength+1)},
		"description long": {Name: "ok", Description: strings.Repeat("x", project.MaxDescriptionLength+1)},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), "alice", in)
			if !errors.Is(err, project.ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
			if ae, _ := apperr.As(err); len(ae.Fields) == 0 {
				t.Fatal("validation error must list fields")
			}
		})
	}
	// Multi-byte names are measured in characters, not bytes.
	if _, err := svc.Create(context.Background(), "alice", project.CreateInput{Name: strings.Repeat("é", project.MaxNameLength)}); err != nil {
		t.Fatalf("100 characters must be accepted: %v", err)
	}
}

func TestCreateNameIsUniquePerOwner(t *testing.T) {
	svc := newService(projecttest.NewRepository(), &recorder{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "alice", project.CreateInput{Name: "Orchard"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "alice", project.CreateInput{Name: "ORCHARD"}); !errors.Is(err, project.ErrNameTaken) {
		t.Fatalf("want ErrNameTaken, got %v", err)
	}
	if _, err := svc.Create(ctx, "bob", project.CreateInput{Name: "Orchard"}); err != nil {
		t.Fatalf("another owner may reuse the name: %v", err)
	}
}

func TestAuditFailureFailsTheOperation(t *testing.T) {
	boom := errors.New("audit down")
	svc := newService(projecttest.NewRepository(), &recorder{err: boom})
	if _, err := svc.Create(context.Background(), "alice", project.CreateInput{Name: "x"}); !errors.Is(err, boom) {
		t.Fatalf("want audit error, got %v", err)
	}
}

func TestUpdate(t *testing.T) {
	repo, rec := projecttest.NewRepository(), &recorder{}
	svc := newService(repo, rec)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "alice", project.CreateInput{Name: "Orchard", Description: "apples"})
	_, _ = svc.Create(ctx, "alice", project.CreateInput{Name: "Vineyard"})

	desc := "pears"
	got, err := svc.Update(ctx, "alice", p.ID, project.Patch{Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Orchard" || got.Description != "pears" || !got.UpdatedAt.After(p.UpdatedAt) || !got.CreatedAt.Equal(p.CreatedAt) {
		t.Fatalf("partial update: %+v", got)
	}

	// Empty patch: no write, no audit.
	before := len(rec.events)
	if same, err := svc.Update(ctx, "alice", p.ID, project.Patch{}); err != nil || !same.UpdatedAt.Equal(got.UpdatedAt) || len(rec.events) != before {
		t.Fatalf("empty patch: %+v %v", same, err)
	}

	taken := "vineyard"
	if _, err := svc.Update(ctx, "alice", p.ID, project.Patch{Name: &taken}); !errors.Is(err, project.ErrNameTaken) {
		t.Fatalf("want ErrNameTaken, got %v", err)
	}
	blank := " "
	if _, err := svc.Update(ctx, "alice", p.ID, project.Patch{Name: &blank}); !errors.Is(err, project.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if _, err := svc.Update(ctx, "bob", p.ID, project.Patch{Description: &desc}); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("other owner: want ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	repo, rec := projecttest.NewRepository(), &recorder{}
	svc := newService(repo, rec)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "alice", project.CreateInput{Name: "Orchard"})

	if err := svc.Delete(ctx, "bob", p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}
	if err := svc.Delete(ctx, "alice", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, "alice", p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("deleted project still readable: %v", err)
	}
	if last := rec.events[len(rec.events)-1]; last.Action != "project.delete" {
		t.Fatalf("audit: %+v", last)
	}
}

func TestList(t *testing.T) {
	svc := newService(projecttest.NewRepository(), &recorder{})
	ctx := context.Background()
	for _, n := range []string{"alpha", "beta", "alphabet"} {
		if _, err := svc.Create(ctx, "alice", project.CreateInput{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = svc.Create(ctx, "bob", project.CreateInput{Name: "alpha"})

	items, total, err := svc.List(ctx, project.ListQuery{OwnerID: "alice", Search: "ALPHA", Limit: 1})
	if err != nil || total != 2 || len(items) != 1 || items[0].Name != "alphabet" {
		t.Fatalf("got %v total=%d err=%v", items, total, err)
	}
}
