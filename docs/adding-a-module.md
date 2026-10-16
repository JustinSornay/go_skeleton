# Ajouter un module

Exemple : un module `invoice`. Copier `internal/project` est le plus rapide ; ce guide explique quoi
adapter et dans quel ordre. Chaque étape se vérifie avec `make test`.

## 1. Le cœur : `internal/invoice/`

`invoice.go` — entité, invariants, erreurs :

```go
var (
	ErrNotFound = apperr.NotFound("invoice.not_found", "invoice not found")
	ErrInvalid  = apperr.Invalid("invoice.invalid", "invoice is invalid")
)

func New(id uuid.UUID, ownerID string, amountCents int64, now time.Time) (Invoice, error) { … }
```

- Les erreurs sont des sentinelles `apperr` avec un code `<module>.<raison>` stable : les clients s'en servent.
- Les invariants vivent ici (pas seulement dans les tags de validation des DTO) : un cron ou un autre
  module qui appelle le service est protégé de la même façon.

`repository.go` — le port de persistance, documenté (erreurs renvoyées, portée par propriétaire).

`service.go` — les cas d'usage. Dépendances = interfaces passées au constructeur (`Repository`,
`Transactor`, `audit.Recorder`…). Horloge et générateur d'ID injectables pour les tests.

Interdit dans ce paquet (vérifié par `internal/archtest`) : `net/http`, SQL, pgx, Redis, chi.

## 2. Les tests du cœur

- `invoicetest/memory.go` : faux `Repository` en mémoire.
- `invoicetest/contract.go` : `RepositoryContract(t, newRepo)` décrit le comportement attendu de tout
  repository.
- `invoicetest/memory_test.go` exécute le contrat sur le faux ; `service_test.go` teste les cas d'usage
  avec ce faux.

## 3. La persistance : migration + `internal/invoice/postgres/`

```bash
make migration name=invoices     # crée migrations/0000N_invoices.sql
```

Écrire `-- +goose Up` **et** `-- +goose Down`. Ne jamais modifier une migration déjà appliquée quelque part :
en ajouter une nouvelle.

Le repository utilise `db.Q(ctx)` (pour participer aux transactions) et traduit les erreurs du driver :

```go
if c, ok := database.UniqueViolation(err); ok && c == "invoices_number_key" {
	return invoice.ErrNumberTaken
}
```

`repository_test.go` (tag `integration`) se résume à exécuter le contrat sur PostgreSQL :

```go
//go:build integration

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func TestPostgresRepositoryContract(t *testing.T) {
	invoicetest.RepositoryContract(t, func(t *testing.T) invoice.Repository {
		return postgres.NewRepository(dbtest.New(t))
	})
}
```

## 4. L'HTTP : `internal/invoice/httpapi/`

- `dto.go` : requêtes/réponses JSON avec tags `json` et `validate` ; fonctions de conversion explicites
  (jamais de type de domaine sérialisé directement).
- `handler.go` : interface `Service` côté consommateur, handlers `func(w, r) error`, `Routes() chi.Router`.
  Utiliser `httpx.Bind`, `httpx.BindQuery`, `httpx.PathUUID`, `httpx.JSON`, `httpx.NoContent`.
- L'identité vient de `auth.FromContext(r.Context())`.

## 5. Le câblage : `internal/app/router.go`

```go
func invoiceModule(d Deps) *invoicehttp.Handler {
	svc := invoice.NewService(invoicepg.NewRepository(d.DB), d.DB, audit.NewPostgresRecorder(d.DB))
	return invoicehttp.NewHandler(svc)
}

// dans NewRouter, groupe authentifié :
r.Mount("/invoices", invoiceModule(d).Routes())
// ou réservé à un rôle :
r.With(mw.RequireRole("billing")).Mount("/invoices", invoiceModule(d).Routes())
```

La politique d'accès reste visible dans `router.go`, au même endroit pour toute l'API.

## 6. Le contrat : `api/openapi.yaml`

Documenter chaque nouvelle route, ses schémas et ses réponses d'erreur (réutiliser
`#/components/responses/*`). `make test` échoue tant que routes et documentation ne correspondent pas.

## 7. Avant de pousser

```bash
make check    # lint + tests unitaires et d'intégration (-race) + govulncheck
```

## Tâches planifiées

Une tâche planifiée est un binaire `cmd/<job>` sur le modèle de `cmd/retention` : `main()` minimal,
`run()` testable, `--dry-run`, traitement par lots, délai global, et un acteur `actor.Cron("<job>")` dans le
contexte pour la traçabilité. Il ne lit que la configuration dont il a besoin.
