// Command retention deletes audit_log rows older than the retention period.
// It is the template for scheduled jobs (cron, Kubernetes CronJob):
//
//   - thin main(), testable run();
//   - --dry-run reports without deleting;
//   - bounded batches, so a large backlog never holds long locks;
//   - the context carries a "cron" actor for traceability;
//   - a global timeout so a stuck job cannot run forever;
//   - no SQL here: the job builds the repositories it uses, like internal/app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/config"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/internal/repository"
	"github.com/JustinSornay/go_skeleton/internal/utils/actor"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type options struct {
	olderThan time.Duration
	batchSize int
	dryRun    bool
	timeout   time.Duration
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("retention", flag.ContinueOnError)
	var o options
	fs.DurationVar(&o.olderThan, "older-than", 365*24*time.Hour, "delete audit entries older than this")
	fs.IntVar(&o.batchSize, "batch-size", 5000, "rows deleted per statement")
	fs.BoolVar(&o.dryRun, "dry-run", false, "only count the rows that would be deleted")
	fs.DurationVar(&o.timeout, "timeout", 30*time.Minute, "abort after this duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.olderThan <= 0 || o.batchSize <= 0 {
		return errors.New("--older-than and --batch-size must be positive")
	}

	dbCfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	log := logging.New(out, logging.Options{Format: "text", Service: "retention"})

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	ctx = actor.With(ctx, actor.Cron("retention"))

	db, err := database.Open(ctx, database.Options{URL: dbCfg.URL, MaxConns: 2})
	if err != nil {
		return err
	}
	defer db.Close()

	n, err := purgeAuditLog(ctx, db, time.Now().Add(-o.olderThan), o.batchSize, o.dryRun)
	if err != nil {
		return err
	}
	log.Info("audit_log retention done", slog.Bool("dry_run", o.dryRun), slog.Int64("rows", n))
	return nil
}

// purgeAuditLog deletes (or counts, when dryRun) rows older than cutoff, in
// batches of batchSize.
func purgeAuditLog(ctx context.Context, db *database.DB, cutoff time.Time, batchSize int, dryRun bool) (int64, error) {
	audit := repository.NewAuditRepository(db)
	if dryRun {
		return audit.CountOlderThan(ctx, cutoff)
	}
	var total int64
	for {
		n, err := audit.DeleteOlderThan(ctx, cutoff, batchSize)
		total += n
		if err != nil || n < int64(batchSize) {
			return total, err
		}
	}
}
