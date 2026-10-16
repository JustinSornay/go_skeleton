# `repository/` — persistance PostgreSQL

Les implémentations PostgreSQL des interfaces déclarées dans [`model/`](../model/) : SQL écrit à la
main sur pgx, sans ORM, pour qu'il reste visible et relu. C'est ici que les erreurs du driver
deviennent des erreurs métier.

## Ce qui va ici

- Un repository par interface : `ProjectRepository` (implémente `model.ProjectRepository`, table
  `projects`) et `AuditRepository` (implémente `model.AuditRecorder`, table `audit_log` ; il fournit
  aussi la purge utilisée par `cmd/retention` : `CountOlderThan`, `DeleteOlderThan`).
- La structure de ligne avec ses tags `db` (`row`), sa conversion vers le modèle (`toDomain`) et la
  traduction des erreurs (`mapErr`).
- Les tests d'intégration (`//go:build integration`), qui tournent sur une vraie base.
- [`repositorytest/`](repositorytest/) : le kit de test (fakes en mémoire et contrats).

## Ce qui n'y va pas

- Les règles métier → `model/` ; les transactions → le service les ouvre avec `model.Transactor`, le
  repository se contente de `r.db.Q(ctx)`.
- HTTP, cache, appels externes : seul `infrastructure/database` est autorisé ici.

## Dépendances

`model/`, `infrastructure/database`, `utils/*` (et pgx). Jamais `net/http` ni chi (vérifié par
`internal/archtest`). Seuls [`app/`](../app/) et les tâches planifiées `cmd/*` l'importent en
production ; les services ne voient que les interfaces.

## Conventions

- Fichier `<entité>_repository.go`, type `<Entité>Repository`, constructeur
  `New<Entité>Repository(db *database.DB)`, et l'assertion `var _ model.<Port> = (*<Entité>Repository)(nil)`.
- Le paquet est partagé : `row`, `columns`, `mapErr`, `uniqueOwnerName` appartiennent à l'exemple
  project. Préfixez ceux d'une nouvelle entité (`invoiceRow`, `invoiceColumns`, `mapInvoiceErr`) ;
  `escapeLike` est générique et réutilisable.
- Toujours `r.db.Q(ctx)` : la requête rejoint la transaction ouverte par `db.WithinTx`, s'il y en a une.
- Chaque requête filtre sur `owner_id` ; « 0 ligne touchée » ou `database.IsNoRows(err)` →
  `model.Err<Entité>NotFound`.
- Contraintes → erreurs métier par leur nom : `database.UniqueViolation(err)` (aussi
  `ForeignKeyViolation`, `CheckViolation`). Le reste est enveloppé (`fmt.Errorf("%s: %w", op, err)`) et
  deviendra un 500.
- Schéma : `make migration name=invoices` crée `migrations/0000N_invoices.sql`, avec `-- +goose Up` et
  `-- +goose Down`. Ne jamais modifier une migration déjà appliquée : en ajouter une.

## Exemple

Référence : [`project_repository.go`](project_repository.go) et son test
[`project_repository_test.go`](project_repository_test.go). Pas-à-pas complet, liste, modification et
suppression comprises : [`docs/adding-a-feature.md`](../../docs/adding-a-feature.md). Squelette pour
`invoice` (`repository/invoice_repository.go`) :

```go
// InvoiceRepository implements model.InvoiceRepository on PostgreSQL.
type InvoiceRepository struct {
	db *database.DB
}

// NewInvoiceRepository returns a Postgres-backed invoice repository.
func NewInvoiceRepository(db *database.DB) *InvoiceRepository { return &InvoiceRepository{db: db} }

var _ model.InvoiceRepository = (*InvoiceRepository)(nil)

const invoiceColumns = `id, owner_id, amount_cents, created_at`

type invoiceRow struct {
	ID          uuid.UUID `db:"id"`
	OwnerID     string    `db:"owner_id"`
	AmountCents int64     `db:"amount_cents"`
	CreatedAt   time.Time `db:"created_at"`
}

func (r *InvoiceRepository) Create(ctx context.Context, inv model.Invoice) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO invoices (`+invoiceColumns+`) VALUES ($1, $2, $3, $4)`,
		inv.ID, inv.OwnerID, inv.AmountCents, inv.CreatedAt)
	return mapInvoiceErr(err, "insert invoice")
}

func (r *InvoiceRepository) Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Invoice, error) {
	rows, _ := r.db.Q(ctx).Query(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE id = $1 AND owner_id = $2`, id, ownerID)
	got, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[invoiceRow])
	if err != nil {
		return model.Invoice{}, mapInvoiceErr(err, "get invoice")
	}
	return model.Invoice{ID: got.ID, OwnerID: got.OwnerID, AmountCents: got.AmountCents, CreatedAt: got.CreatedAt.UTC()}, nil
}

func mapInvoiceErr(err error, op string) error {
	switch {
	case err == nil:
		return nil
	case database.IsNoRows(err):
		return model.ErrInvoiceNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
```

Le test d'intégration (`invoice_repository_test.go`) se résume au contrat. Il est en paquet externe
`repository_test`, comme les autres tests du dossier. N'ajoutez pas de `TestMain` : le paquet n'en a
qu'un, dans `audit_repository_test.go`.

```go
//go:build integration

package repository_test

import (
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database/dbtest"
	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/repository"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
)

func TestInvoiceRepositoryContract(t *testing.T) {
	repositorytest.InvoiceRepositoryContract(t, func(t *testing.T) model.InvoiceRepository {
		return repository.NewInvoiceRepository(dbtest.New(t))
	})
}
```

## Tester

Tous les tests du paquet portent le tag `integration` : sans lui, `go test` ne trouve rien à lancer.
Chaque `dbtest.New(t)` fournit une base neuve, migrée (Docker via testcontainers, ou
`TEST_DATABASE_URL`).

```bash
go test -tags integration ./internal/repository/...
make test-integration
```
