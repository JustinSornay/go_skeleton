//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func TestPurgeAuditLog(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	_, err := db.Q(ctx).Exec(ctx, `
		INSERT INTO audit_log (actor_type, action, occurred_at)
		SELECT 'system', 'test', now() - make_interval(days => d)
		FROM generate_series(1, 10) AS d`)
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-5*24*time.Hour - time.Hour) // keeps days 1..5

	n, err := purgeAuditLog(ctx, db, cutoff, 2, true)
	if err != nil || n != 5 {
		t.Fatalf("dry run: n=%d err=%v", n, err)
	}
	n, err = purgeAuditLog(ctx, db, cutoff, 2, false) // batches of 2: 2+2+1
	if err != nil || n != 5 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
	var left int
	_ = db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&left)
	if left != 5 {
		t.Fatalf("%d rows left, want 5", left)
	}
}
