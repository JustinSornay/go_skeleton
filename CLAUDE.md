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
make test              # unit tests + architecture rules, no Docker
make test-integration  # + PostgreSQL tests (testcontainers, or TEST_DATABASE_URL)
make lint              # golangci-lint v2 (config: .golangci.yml)
make check             # what CI runs: lint + integration tests with -race + govulncheck
make migration name=add_things        # new SQL migration in migrations/
go test ./internal/service/... -run TestCreateProject   # single test
go test ./internal/archtest/                     # dependency rules only
go test -tags integration ./internal/repository/...   # repository tests (PostgreSQL)
```

Local auth: `AUTH_MODE=dev` makes the bearer token the user id (`Authorization: Bearer alice`, or `alice:admin` for roles).

## Architecture

Global layer folders: one folder per classic role under `internal/`, each with a `README.md` (in French) giving its role, what goes in, its dependency rules, naming conventions, the reference example and a skeleton for a new feature. Read `internal/README.md` (map, dependency diagram, rules) and `docs/architecture.md` before structural changes.

| Folder | Role | May import (internal) |
|---|---|---|
| `internal/model/` | Entities, invariants, business errors, interfaces (ports: `ProjectRepository`, `Transactor`, `AuditRecorder`) | `utils/apperr` only; never `net/http`, `database/sql`, pgx, go-redis, chi |
| `internal/service/` | Use cases; depend on `model` interfaces injected through the constructor; shared options `WithClock`, `WithIDGenerator` in `service.go` | `model`, `utils/*` except `utils/httpx`; never `net/http`, `database/sql`, pgx, go-redis, chi |
| `internal/repository/` | PostgreSQL implementations of the `model` interfaces | `model`, `infrastructure/database`, `utils/*`; never `net/http`, chi |
| `internal/repository/repositorytest/` | Test kit: in-memory fakes, `<Interface>Contract` suites, `Tx`, `AuditRecorder` | same as `repository` |
| `internal/dto/` | JSON request/response types and `To<Entity>Response` conversions | `model`, `utils/*` |
| `internal/controller/` | HTTP handlers + `Routes()`; declares the service interface it consumes (`controller.ProjectService`) | `service`, `dto`, `model`, `utils/*`, `infrastructure/auth` |
| `internal/middleware/` | Cross-cutting HTTP middleware; declares the small interfaces it consumes (consumer side) | `utils/*`, `infrastructure/auth` |
| `internal/infrastructure/{auth,cache,database,health,metrics,server}` | Connections to the outside world; no business code. `database/dbtest` is a test kit | `infrastructure/*`, `utils/*`, `migrations` |
| `internal/utils/{actor,apperr,httpx,logging,requestid}` | Small stateless helpers | `utils/*` |
| `internal/config/` | Environment configuration | nothing internal |
| `internal/app/` | The API's composition root: wiring (one `<feature>Module(d Deps)` per feature in `router.go`), middleware order, route mounting, servers | anything |
| `cmd/` | Thin `main()` calling a testable `run()`. A scheduled job (`cmd/retention`) is its binary's composition root: it builds the repositories and services it uses | anything but test kits |
| `migrations/`, `api/` | Module packages outside `internal/` and `cmd/` | nothing internal |

Global rules:

- Only `cmd/*` imports `internal/app`; nothing imports `cmd/*`.
- Packages outside `internal/` and `cmd/` (`migrations`, `api`) import nothing from `internal/`.
- No production package imports a test kit (`repositorytest`, `dbtest`); `_test.go` files may.
- A folder under `internal/` holding Go files (even only tests) outside the known layers fails `internal/archtest`. A new layer means updating `layers` and `rules` in `internal/archtest/arch_test.go` and `internal/README.md`.
- These rules are enforced by `internal/archtest`; fix the design rather than the test.

The `project` entity is the reference example across every layer: copy its files for a new feature (`docs/adding-a-feature.md`). Naming: one file per entity and layer (`model/project.go`, `repository/project_repository.go`, `service/project_service.go`, `dto/project_dto.go`, `controller/project_controller.go`). Packages are shared by all features, so prefix identifiers with the entity (`ErrProjectNotFound`, `NewProjectService`). The unprefixed helpers of `repository/project_repository.go` (`row`, `columns`, `mapErr`) are the exception; prefix a new entity's ones (`invoiceRow`, `mapInvoiceErr`).

## Conventions

### Errors
- Handlers have the signature `func(w http.ResponseWriter, r *http.Request) error` and are wrapped with `httpx.Handle`. Never panic to report an error and never write error responses by hand.
- Business errors are `*apperr.Error` sentinels declared in `internal/model` (`apperr.NotFound("project.not_found", …)`). The `Kind` decides the HTTP status (`httpx.StatusFor`); the `Code` (`<entity>.<reason>`) is a stable public identifier — never rename an existing code.
- Wrap with context using `fmt.Errorf("doing x: %w", err)`. Non-`apperr` errors become opaque 500s (logged, never shown).
- Resources owned by someone else are reported as not found (404), never forbidden.

### HTTP
- Bind input with `httpx.Bind[T]` (JSON body) and `httpx.BindQuery` (`query:"name"` tags), using types from `internal/dto`; validate with `validate` tags. Invariants are still enforced by the `model` constructors.
- Responses go through explicit DTO conversion functions (`dto.ToProjectResponse`); never serialize model types.
- List endpoints embed `httpx.PageQuery` and return `httpx.List[T]` (`{"data": [...], "page": {...}}`).
- The authenticated user comes from `currentUser(r)` (`internal/controller/controller.go`).
- Access policy (`RequireAuth`, `RequireRole`) is declared in `internal/app/router.go`, not in controllers.
- Every route must be documented in `api/openapi.yaml`; `TestRoutesMatchOpenAPI` fails otherwise.

### Data
- SQL is handwritten in `internal/repository`. Always use `r.db.Q(ctx)` so calls join the transaction started by `db.WithinTx`.
- Services open transactions through `model.Transactor` (`s.tx.WithinTx`); `internal/app` passes `*database.DB` for it.
- Translate driver errors into model errors in the repository (`database.UniqueViolation`, `database.IsNoRows`).
- Migrations: goose SQL files with working `Up` and `Down`. Never modify a migration that has been applied anywhere — add a new one.
- Sensitive mutations record a `model.AuditEvent` through `model.AuditRecorder` inside the same transaction, and return its error.
- A service needing a cache or an external client gets an interface declared in `internal/model`, using only simple or standard types (`City(ctx, lat, lon float64) (string, error)`): the client in `internal/infrastructure/<name>` satisfies it without importing `model` (archtest forbids that import), and `internal/app` wires it. When the interface needs model types (a client returning `model.Forecast`, a cache decorator around a `model.ProjectRepository`), the adapter or decorator lives in `internal/app` (`app/forecast_adapter.go`), the only package that may import both; it translates types and calls the cache, nothing more. See `docs/architecture.md` (« Cache et services externes »).

### Tests
- Handwritten fakes in `internal/repository/repositorytest`, no generated mocks. A repository fake must pass the same `<Interface>Contract` as the PostgreSQL implementation; `Tx` and `AuditRecorder` are plain test doubles without a contract.
- Service and controller tests use the fakes (`repositorytest.NewProjectRepository()`, `repositorytest.Tx{}`, `&repositorytest.AuditRecorder{}`).
- Tests live in external packages (`<layer>_test`). Shared helpers sit in feature-neutral files: `mount`, `client`, `decode` in `internal/controller/controller_test.go`, `steppingClock` in `internal/service/service_test.go`. Feature test helpers and test names carry the entity (`newProjectServer`, `TestProjectCRUD`, `newProjectService`, `TestCreateProject`).
- Integration tests carry `//go:build integration` and use `dbtest.Main` / `dbtest.New(t)` (one fresh database per test). `internal/repository` has a single `TestMain`, in `audit_repository_test.go`.

### Configuration
- Environment variables only, declared with type and default in `internal/config/config.go` and listed in `.env.example`. Production safety rules belong in `Config.Validate`. Infrastructure packages never read configuration: `internal/app` maps it to their `Options`.

### Documentation
- Human-facing docs (`README.md` files, `docs/*.md`) are written in French; code and code comments in English.
- When a layer's conventions or rules change, update its `README.md` and `internal/README.md`.
