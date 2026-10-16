package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrator applies the SQL migrations embedded in the binary (see the
// migrations package) with goose. Shipping migrations inside the binary
// guarantees code and schema versions travel together.
type Migrator struct {
	sqlDB    *sql.DB
	provider *goose.Provider
}

// NewMigrator builds a migrator on top of the pool.
func NewMigrator(db *DB, migrations fs.FS) (*Migrator, error) {
	sqlDB := stdlib.OpenDBFromPool(db.pool)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database: migrations: %w", err)
	}
	return &Migrator{sqlDB: sqlDB, provider: p}, nil
}

// Up applies all pending migrations and returns how many ran.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	res, err := m.provider.Up(ctx)
	return len(res), err
}

// Down rolls back the latest migration.
func (m *Migrator) Down(ctx context.Context) error {
	_, err := m.provider.Down(ctx)
	return err
}

// Status lists every migration and whether it is applied.
func (m *Migrator) Status(ctx context.Context) ([]*goose.MigrationStatus, error) {
	return m.provider.Status(ctx)
}

// Version returns the current schema version.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	return m.provider.GetDBVersion(ctx)
}

// Close releases the database/sql adapter (the pool stays open).
func (m *Migrator) Close() error { return m.sqlDB.Close() }
