//go:build integration

package audit_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/platform/actor"
	"github.com/JustinSornay/go_skeleton/internal/platform/audit"
	"github.com/JustinSornay/go_skeleton/internal/platform/database/dbtest"
	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func TestRecord(t *testing.T) {
	db := dbtest.New(t)
	rec := audit.NewPostgresRecorder(db)
	ctx := actor.With(requestid.With(context.Background(), "req-1"), actor.User("alice"))

	if err := rec.Record(ctx, audit.Event{Action: "thing.delete", TargetType: "thing", TargetID: "42", Metadata: map[string]any{"reason": "test"}}); err != nil {
		t.Fatal(err)
	}
	var actorType, actorID, action, target, reqID, reason string
	err := db.Q(ctx).QueryRow(ctx, `
		SELECT actor_type, actor_id, action, target_type || ':' || target_id, request_id, metadata->>'reason'
		FROM audit_log`).Scan(&actorType, &actorID, &action, &target, &reqID, &reason)
	if err != nil {
		t.Fatal(err)
	}
	if actorType != "user" || actorID != "alice" || action != "thing.delete" || target != "thing:42" || reqID != "req-1" || reason != "test" {
		t.Fatalf("unexpected row: %s %s %s %s %s %s", actorType, actorID, action, target, reqID, reason)
	}
}

func TestRecordIsTransactional(t *testing.T) {
	db := dbtest.New(t)
	rec := audit.NewPostgresRecorder(db)
	boom := errors.New("business failure")
	err := db.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := rec.Record(ctx, audit.Event{Action: "thing.create"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n int
	_ = db.Q(context.Background()).QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n)
	if n != 0 {
		t.Fatal("audit entry survived the rollback")
	}
}
