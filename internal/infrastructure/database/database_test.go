//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func setup(t *testing.T) *database.DB {
	t.Helper()
	db := dbtest.New(t)
	if _, err := db.Q(context.Background()).Exec(context.Background(),
		`CREATE TABLE items (name text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func count(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	if err := db.Q(context.Background()).QueryRow(context.Background(), `SELECT count(*) FROM items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func insert(ctx context.Context, db *database.DB, name string) error {
	_, err := db.Q(ctx).Exec(ctx, `INSERT INTO items (name) VALUES ($1)`, name)
	return err
}

func TestWithinTxCommits(t *testing.T) {
	db := setup(t)
	err := db.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := insert(ctx, db, "a"); err != nil {
			return err
		}
		// Nested calls join the outer transaction.
		return db.WithinTx(ctx, func(ctx context.Context) error { return insert(ctx, db, "b") })
	})
	if err != nil || count(t, db) != 2 {
		t.Fatalf("err=%v count=%d", err, count(t, db))
	}
}

func TestWithinTxRollsBackOnError(t *testing.T) {
	db := setup(t)
	boom := errors.New("boom")
	err := db.WithinTx(context.Background(), func(ctx context.Context) error {
		_ = insert(ctx, db, "a")
		return boom
	})
	if !errors.Is(err, boom) || count(t, db) != 0 {
		t.Fatalf("err=%v count=%d", err, count(t, db))
	}
}

func TestWithinTxRollsBackOnPanic(t *testing.T) {
	db := setup(t)
	func() {
		defer func() { _ = recover() }()
		_ = db.WithinTx(context.Background(), func(ctx context.Context) error {
			_ = insert(ctx, db, "a")
			panic("bug")
		})
	}()
	if count(t, db) != 0 {
		t.Fatal("panic did not roll back")
	}
}

func TestErrorHelpers(t *testing.T) {
	db := setup(t)
	ctx := context.Background()
	_ = insert(ctx, db, "a")
	err := insert(ctx, db, "a")
	if c, ok := database.UniqueViolation(err); !ok || c != "items_pkey" {
		t.Fatalf("UniqueViolation(%v) = %q, %v", err, c, ok)
	}
	if _, ok := database.ForeignKeyViolation(err); ok {
		t.Fatal("not a foreign key violation")
	}
	err = db.Q(ctx).QueryRow(ctx, `SELECT name FROM items WHERE name = 'zzz'`).Scan(new(string))
	if !database.IsNoRows(err) {
		t.Fatalf("IsNoRows(%v) = false", err)
	}
}
