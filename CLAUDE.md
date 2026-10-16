# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

## Working principles

**Think before coding.** State assumptions explicitly. When ambiguity exists, present interpretations and ask rather than guessing silently. Push back if a simpler approach exists.

**Surgical changes.** Touch only what the task requires. Don't refactor, reformat, or "improve" adjacent code. Match existing style even if you'd do it differently. If unrelated dead code is spotted, mention it — don't remove it.

**Goal-driven execution.** For bugs: write a test that reproduces it, then fix it. For features: define success criteria before implementing. Multi-step tasks get a brief plan (`step → verify`) upfront.

**Simplicity first.** Minimum code that solves the problem. No speculative features, no abstractions for single use, no flexibility that wasn't requested.

## Commands

```bash
make help              # every target
make up && make migrate && make run   # local API on :8080 (needs .env, see .env.example)
make test              # unit tests, no Docker
make test-integration  # + PostgreSQL tests (testcontainers, or TEST_DATABASE_URL)
make lint              # golangci-lint v2 (config: .golangci.yml)
make check             # what CI runs: lint + integration tests with -race + govulncheck
make migration name=add_things        # new SQL migration in migrations/
go test ./internal/project/... -run TestCreate   # single test
```

Local auth: `AUTH_MODE=dev` makes the bearer token the user id (`Authorization: Bearer alice`, or `alice:admin` for roles).

## Architecture

Package-by-feature with a clean core per module. Read `docs/architecture.md` before structural changes.

- `internal/<module>/` — domain entities, invariants, sentinel errors, repository port, service (use cases). Must NOT import net/http, SQL, pgx, Redis, chi, `platform/{database,httpx,middleware}`, nor its own adapters.
- `internal/<module>/httpapi` — DTOs + handlers + `Routes()`. `internal/<module>/postgres` — SQL implementation of the port. `internal/<module>/<module>test` — in-memory fake + `RepositoryContract`.
- `internal/platform/` — technical building blocks, no business code, never imports modules.
- `internal/app/` — the only composition root: wiring, middleware order, route mounting, servers.
- `cmd/` — thin `main()` calling a testable `run()`.
- These rules are enforced by `internal/archtest`; fix the design rather than the test.

`internal/project` is the reference module: copy its structure for new features (`docs/adding-a-module.md`).

## Conventions

### Errors
- Handlers have the signature `func(w http.ResponseWriter, r *http.Request) error` and are wrapped with `httpx.Handle`. Never panic to report an error and never write error responses by hand.
- Business errors are `*apperr.Error` sentinels declared in the module (`apperr.NotFound("project.not_found", …)`). The `Kind` decides the HTTP status (`httpx.StatusFor`); the `Code` is a stable public identifier — never rename an existing code.
- Wrap with context using `fmt.Errorf("doing x: %w", err)`. Non-`apperr` errors become opaque 500s (logged, never shown).
- Resources owned by someone else are reported as not found (404), never forbidden.

### HTTP
- Bind input with `httpx.Bind[T]` (JSON body) and `httpx.BindQuery` (`query:"name"` tags); validate with `validate` tags. Domain invariants are still enforced in the module.
- Responses go through explicit DTO conversion functions; never serialize domain types.
- List endpoints embed `httpx.PageQuery` and return `httpx.List[T]` (`{"data": [...], "page": {...}}`).
- Access policy (`RequireAuth`, `RequireRole`) is declared in `internal/app/router.go`, not in modules.
- Every route must be documented in `api/openapi.yaml`; `TestRoutesMatchOpenAPI` fails otherwise.

### Data
- SQL is handwritten in the `postgres` adapters. Always use `db.Q(ctx)` so calls join the transaction started by `db.WithinTx`.
- Translate driver errors into module errors in the adapter (`database.UniqueViolation`, `database.IsNoRows`).
- Migrations: goose SQL files with working `Up` and `Down`. Never modify a migration that has been applied anywhere — add a new one.
- Sensitive mutations record an `audit.Event` inside the same transaction.

### Tests
- Handwritten fakes, no generated mocks. A module's fake must pass the same `RepositoryContract` as the PostgreSQL implementation.
- Integration tests carry `//go:build integration` and use `dbtest.Main` / `dbtest.New(t)` (one fresh database per test).

### Configuration
- Environment variables only, declared with type and default in `internal/platform/config/config.go` and listed in `.env.example`. Production safety rules belong in `Config.Validate`.
