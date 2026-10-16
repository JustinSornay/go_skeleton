// Package database owns the PostgreSQL connection pool (pgx), transactions
// and migrations.
//
// Repositories never hold a *pgxpool.Pool directly: they call db.Q(ctx),
// which returns the transaction started by WithinTx when there is one, or
// the pool otherwise. Services can therefore make several repository calls
// atomic without knowing anything about pgx.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options configures the pool.
type Options struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// Querier is implemented by both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DB wraps the pool.
type DB struct {
	pool *pgxpool.Pool
}

// Open creates the pool and checks connectivity.
func Open(ctx context.Context, o Options) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(o.URL)
	if err != nil {
		return nil, fmt.Errorf("database: parsing URL: %w", err)
	}
	if o.MaxConns > 0 {
		cfg.MaxConns = o.MaxConns
	}
	cfg.MinConns = o.MinConns
	if o.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = o.MaxConnLifetime
	}
	if o.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = o.MaxConnIdleTime
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: creating pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// New wraps an existing pool (tests).
func New(pool *pgxpool.Pool) *DB { return &DB{pool: pool} }

// Pool exposes the underlying pool for infrastructure code (migrations,
// health checks). Repositories should use Q.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Ping checks connectivity (readiness probe).
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// Close releases all connections.
func (db *DB) Close() { db.pool.Close() }

type txKey struct{}

// Q returns the transaction bound to ctx, or the pool.
func (db *DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return db.pool
}

// WithinTx runs fn in a transaction: committed if fn returns nil, rolled back
// otherwise (including on panic). Nested calls join the outer transaction.
func (db *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("database: rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}
