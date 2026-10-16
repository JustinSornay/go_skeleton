// Command migrate manages the database schema with the migrations embedded
// in the binary. Run it as a release step before starting a new API version.
//
//	migrate up              apply all pending migrations
//	migrate down            roll back the latest migration
//	migrate status          list migrations and their state
//	migrate version         print the current schema version
//	migrate create <name>   add an empty SQL migration to ./migrations
//
// Only DATABASE_URL is required (create needs no database).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/JustinSornay/go_skeleton/internal/config"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/migrations"
)

const usage = "usage: migrate up|down|status|version|create <name>"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	if args[0] == "create" {
		if len(args) != 2 {
			return errors.New("usage: migrate create <snake_case_name>")
		}
		goose.SetSequential(true)
		return goose.Create(nil, "migrations", args[1], "sql")
	}

	dbCfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, database.Options{URL: dbCfg.URL, MaxConns: 2})
	if err != nil {
		return err
	}
	defer db.Close()
	m, err := database.NewMigrator(db, migrations.FS)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()

	switch args[0] {
	case "up":
		n, err := m.Up(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "applied %d migration(s)\n", n)
	case "down":
		return m.Down(ctx)
	case "version":
		v, err := m.Version(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, v)
	case "status":
		st, err := m.Status(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "VERSION\tSTATE\tAPPLIED AT\tFILE")
		for _, s := range st {
			applied := "-"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", s.Source.Version, s.State, applied, s.Source.Path)
		}
		return tw.Flush()
	default:
		return errors.New(usage)
	}
	return nil
}
