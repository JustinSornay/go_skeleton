// Package migrations embeds the SQL migrations (goose format) in the binary.
//
// Rules:
//   - never edit a migration that has been applied anywhere: add a new one;
//   - every file has a working "-- +goose Down" section;
//   - create files with `make migration name=<snake_case_name>`.
package migrations

import "embed"

// FS contains every *.sql migration file.
//
//go:embed *.sql
var FS embed.FS
