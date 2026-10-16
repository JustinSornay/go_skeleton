//go:build integration

// Package dbtest provides isolated PostgreSQL databases for integration
// tests (build tag "integration").
//
// One PostgreSQL server is used per test binary: TEST_DATABASE_URL when set
// (CI service, local instance), otherwise a throwaway testcontainers
// container. Migrations are applied once to a template database; every test
// then gets its own database cloned from it (milliseconds), so tests are
// isolated and can run in parallel.
//
// Usage in a package:
//
//	func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
//
//	func TestSomething(t *testing.T) {
//		db := dbtest.New(t)
//		...
//	}
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/migrations"
)

// Image is the PostgreSQL image used when TEST_DATABASE_URL is not set.
// Keep it in sync with compose.yaml and the CI service.
const Image = "postgres:18-alpine"

var (
	adminURL string // server-level connection used to create databases
	template string
	createMu sync.Mutex // CREATE DATABASE ... TEMPLATE must not run concurrently
)

// Main prepares the server and the template database, runs the tests and
// cleans up. Its result is the process exit code.
func Main(m *testing.M) int {
	ctx := context.Background()
	cleanup, err := setup(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dbtest:", err)
		return 1
	}
	code := m.Run()
	cleanup()
	return code
}

func setup(ctx context.Context) (func(), error) {
	cleanup := func() {}
	adminURL = os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		c, err := tcpostgres.Run(ctx, Image,
			tcpostgres.WithDatabase("test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			return nil, fmt.Errorf("starting %s (set TEST_DATABASE_URL to use an existing server): %w", Image, err)
		}
		cleanup = func() { _ = c.Terminate(context.Background()) }
		if adminURL, err = c.ConnectionString(ctx, "sslmode=disable"); err != nil {
			cleanup()
			return nil, err
		}
	}

	template = "tmpl_" + randomSuffix()
	if err := exec(ctx, adminURL, "CREATE DATABASE "+pgx.Identifier{template}.Sanitize()); err != nil {
		cleanup()
		return nil, err
	}
	dropTemplate := func() {
		_ = exec(context.Background(), adminURL, "DROP DATABASE IF EXISTS "+pgx.Identifier{template}.Sanitize()+" WITH (FORCE)")
	}
	if err := migrate(ctx, withDatabase(adminURL, template)); err != nil {
		dropTemplate()
		cleanup()
		return nil, err
	}
	return func() { dropTemplate(); cleanup() }, nil
}

// New returns a connection to a fresh, fully migrated database that is
// dropped when the test ends.
func New(t testing.TB) *database.DB {
	t.Helper()
	ctx := context.Background()
	name := "t_" + randomSuffix()

	createMu.Lock()
	err := exec(ctx, adminURL, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s",
		pgx.Identifier{name}.Sanitize(), pgx.Identifier{template}.Sanitize()))
	createMu.Unlock()
	if err != nil {
		t.Fatalf("dbtest: creating database: %v", err)
	}

	db, err := database.Open(ctx, database.Options{URL: withDatabase(adminURL, name), MaxConns: 4})
	if err != nil {
		t.Fatalf("dbtest: opening database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		_ = exec(context.Background(), adminURL, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})
	return db
}

// URL returns the connection string of db's database (for code under test
// that opens its own connections, e.g. a cmd's run function).
func URL(t testing.TB, db *database.DB) string {
	t.Helper()
	return withDatabase(adminURL, db.Pool().Config().ConnConfig.Database)
}

func migrate(ctx context.Context, url string) error {
	db, err := database.Open(ctx, database.Options{URL: url, MaxConns: 2})
	if err != nil {
		return err
	}
	defer db.Close()
	m, err := database.NewMigrator(db, migrations.FS)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	_, err = m.Up(ctx)
	return err
}

func exec(ctx context.Context, url, sql string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, sql)
	return err
}

func withDatabase(raw, name string) string {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + name
	return u.String()
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
