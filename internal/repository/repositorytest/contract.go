package repositorytest

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/model"
)

// ProjectRepositoryContract runs the behaviour every model.ProjectRepository
// must have. It is executed against the in-memory fake and against
// PostgreSQL, which guarantees the fake used by unit tests never drifts from
// production.
func ProjectRepositoryContract(t *testing.T, newRepo func(t *testing.T) model.ProjectRepository) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(owner, name string, at time.Time) model.Project {
		p, err := model.NewProject(uuid.NewV7(), owner, name, "desc "+name, at)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("create and get", func(t *testing.T) {
		repo := newRepo(t)
		p := mk("alice", "Orchard", t0)
		if err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(ctx, "alice", p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got != p {
			t.Fatalf("got %+v, want %+v", got, p)
		}
	})

	t.Run("get is scoped by owner", func(t *testing.T) {
		repo := newRepo(t)
		p := mk("alice", "Orchard", t0)
		_ = repo.Create(ctx, p)
		if _, err := repo.Get(ctx, "bob", p.ID); !errors.Is(err, model.ErrProjectNotFound) {
			t.Fatalf("want ErrProjectNotFound, got %v", err)
		}
		if _, err := repo.Get(ctx, "alice", uuid.NewV7()); !errors.Is(err, model.ErrProjectNotFound) {
			t.Fatalf("want ErrProjectNotFound, got %v", err)
		}
	})

	t.Run("names are unique per owner, case-insensitively", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Create(ctx, mk("alice", "Orchard", t0)); err != nil {
			t.Fatal(err)
		}
		if err := repo.Create(ctx, mk("alice", "ORCHARD", t0)); !errors.Is(err, model.ErrProjectNameTaken) {
			t.Fatalf("want ErrProjectNameTaken, got %v", err)
		}
		if err := repo.Create(ctx, mk("bob", "Orchard", t0)); err != nil {
			t.Fatalf("other owner: %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		repo := newRepo(t)
		p := mk("alice", "Orchard", t0)
		other := mk("alice", "Vineyard", t0)
		_ = repo.Create(ctx, p)
		_ = repo.Create(ctx, other)

		p.Name, p.Description, p.UpdatedAt = "Orchard 2", "new", t0.Add(time.Hour)
		if err := repo.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got, _ := repo.Get(ctx, "alice", p.ID); got != p {
			t.Fatalf("got %+v, want %+v", got, p)
		}
		p.Name = "vineyard"
		if err := repo.Update(ctx, p); !errors.Is(err, model.ErrProjectNameTaken) {
			t.Fatalf("want ErrProjectNameTaken, got %v", err)
		}
		p.Name, p.OwnerID = "Orchard 3", "bob"
		if err := repo.Update(ctx, p); !errors.Is(err, model.ErrProjectNotFound) {
			t.Fatalf("other owner: want ErrProjectNotFound, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		repo := newRepo(t)
		p := mk("alice", "Orchard", t0)
		_ = repo.Create(ctx, p)
		if err := repo.Delete(ctx, "bob", p.ID); !errors.Is(err, model.ErrProjectNotFound) {
			t.Fatalf("other owner: want ErrProjectNotFound, got %v", err)
		}
		if err := repo.Delete(ctx, "alice", p.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctx, "alice", p.ID); !errors.Is(err, model.ErrProjectNotFound) {
			t.Fatalf("second delete: want ErrProjectNotFound, got %v", err)
		}
	})

	t.Run("list filters, orders and paginates", func(t *testing.T) {
		repo := newRepo(t)
		names := []string{"alpha", "beta", "alphabet", "100%_real"}
		for i, n := range names {
			if err := repo.Create(ctx, mk("alice", n, t0.Add(time.Duration(i)*time.Minute))); err != nil {
				t.Fatal(err)
			}
		}
		_ = repo.Create(ctx, mk("bob", "alpha", t0))

		list := func(q model.ProjectListQuery) ([]string, int) {
			t.Helper()
			q.OwnerID = "alice"
			items, total, err := repo.List(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			out := make([]string, len(items))
			for i, p := range items {
				out[i] = p.Name
			}
			return out, total
		}
		eq := func(got []string, total int, want []string, wantTotal int) {
			t.Helper()
			if total != wantTotal || len(got) != len(want) {
				t.Fatalf("got %v (total %d), want %v (total %d)", got, total, want, wantTotal)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		}

		got, total := list(model.ProjectListQuery{Limit: 10})
		eq(got, total, []string{"100%_real", "alphabet", "beta", "alpha"}, 4)
		got, total = list(model.ProjectListQuery{Limit: 2, Offset: 1})
		eq(got, total, []string{"alphabet", "beta"}, 4)
		got, total = list(model.ProjectListQuery{Search: "ALPHA", Limit: 10})
		eq(got, total, []string{"alphabet", "alpha"}, 2)
		// LIKE wildcards in the search are literal characters.
		got, total = list(model.ProjectListQuery{Search: "%_", Limit: 10})
		eq(got, total, []string{"100%_real"}, 1)
		got, total = list(model.ProjectListQuery{Limit: 10, Offset: 10})
		eq(got, total, []string{}, 4)
		got, total = list(model.ProjectListQuery{Search: "zzz", Limit: 10})
		eq(got, total, []string{}, 0)
	})
}
