# `service/` — logique métier (cas d'usage)

Un service implémente les cas d'usage d'une entité : il construit ou modifie les entités de
[`model/`](../model/), ouvre les transactions, appelle les repositories et audite les mutations
sensibles. Il ne connaît que des interfaces, injectées par son constructeur : `app/` y branche
PostgreSQL, les tests y branchent les fakes en mémoire.

## Ce qui va ici

- [`service.go`](service.go) : la doc du paquet et les options communes à tous les services : `Option`,
  `WithClock`, `WithIDGenerator`. Elles remplissent la structure non exportée `options` (champs `now`,
  `newID`), à embarquer dans chaque service et à construire avec `newOptions(opts)`.
- Un fichier par entité : `<Entité>Service`, `New<Entité>Service(…, opts ...Option)`, les types d'entrée
  (`CreateProjectInput`) et une méthode par cas d'usage (`Create`, `Get`, `List`, `Update`, `Delete`).

## Ce qui n'y va pas

- HTTP (requête, statut, JSON) → [`controller/`](../controller/) et [`dto/`](../dto/).
- SQL et types concrets (`*repository.ProjectRepository`, `*database.DB`) → injectez l'interface de
  `model/` ; c'est [`app/`](../app/) qui fournit l'implémentation.
- Les invariants d'une entité → dans son constructeur, dans `model/`. Le service les appelle.
- Besoin d'un cache ou d'un client externe ? Déclarez l'interface dans `model/`, avec des types simples
  ou standard (`City(ctx, lat, lon float64) (string, error)`), et faites-la câbler par `app/` :
  `service/` n'importe pas `infrastructure/*`, et le client d'`infrastructure/` la satisfait sans
  importer `model/`. Si l'interface a besoin de types du modèle, l'adaptateur vit dans `app/` (voir
  [architecture.md](../../docs/architecture.md#cache-et-services-externes)).

## Dépendances

`model/` et `utils/*` sauf `utils/httpx`. Jamais `net/http`, `database/sql`, pgx, go-redis ni chi
(vérifié par `internal/archtest`). Seuls `controller/` (via son interface), `app/` et les tâches
planifiées `cmd/*` en dépendent.

## Conventions

- Fichier `<entité>_service.go`, type `<Entité>Service`, constructeur `New<Entité>Service`, entrées
  `<Verbe><Entité>Input`.
- Le propriétaire est un paramètre explicite (`ownerID`) de chaque méthode, juste après `ctx`, ou le
  champ `OwnerID` de la requête de liste (`List(ctx, model.ProjectListQuery)`).
- Une mutation sensible et son `model.AuditEvent` sont écrits dans le même `s.tx.WithinTx` ; l'erreur
  de `Record` est renvoyée, jamais ignorée. Action d'audit : `<entité>.<verbe>` (`project.delete`).
- Erreurs enveloppées avec contexte : `fmt.Errorf("create project: %w", err)`. Les sentinelles `apperr`
  restent détectables par `errors.Is` et décident du statut HTTP.
- Heure et identifiants viennent de `s.now()` et `s.newID()` (UTC tronqué à la microseconde, UUIDv7),
  jamais de `time.Now()` en direct.

## Exemple

Référence : [`project_service.go`](project_service.go). Pas-à-pas complet :
[`docs/adding-a-feature.md`](../../docs/adding-a-feature.md). Squelette pour `invoice`
(`service/invoice_service.go`) :

```go
// InvoiceService implements the invoice use cases.
type InvoiceService struct {
	repo  model.InvoiceRepository
	tx    model.Transactor
	audit model.AuditRecorder
	options
}

// NewInvoiceService wires the invoice use cases.
func NewInvoiceService(repo model.InvoiceRepository, tx model.Transactor, audit model.AuditRecorder, opts ...Option) *InvoiceService {
	return &InvoiceService{repo: repo, tx: tx, audit: audit, options: newOptions(opts)}
}

// CreateInvoiceInput carries the fields of a new invoice.
type CreateInvoiceInput struct {
	AmountCents int64
}

// Create adds an invoice for ownerID and audits it in the same transaction.
func (s *InvoiceService) Create(ctx context.Context, ownerID string, in CreateInvoiceInput) (model.Invoice, error) {
	inv, err := model.NewInvoice(s.newID(), ownerID, in.AmountCents, s.now())
	if err != nil {
		return model.Invoice{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, inv); err != nil {
			return err
		}
		return s.audit.Record(ctx, model.AuditEvent{Action: "invoice.create", TargetType: "invoice", TargetID: inv.ID.String()})
	})
	if err != nil {
		return model.Invoice{}, fmt.Errorf("create invoice: %w", err)
	}
	return inv, nil
}
```

## Tester

Tests unitaires sans base, en paquet externe `service_test`, avec les fakes de
[`repository/repositorytest/`](../repository/repositorytest/) : `NewProjectRepository()`, `Tx{}` et
`&AuditRecorder{}` (`Events()` pour vérifier l'audit, `Err` pour simuler une panne).

- [`service_test.go`](service_test.go) contient les aides partagées par tous les services :
  `steppingClock()`, une horloge déterministe qui avance d'une seconde à chaque appel. Réutilisez-la.
- [`project_service_test.go`](project_service_test.go) contient les aides et tests propres à project,
  tous préfixés (`newProjectService`, `TestCreateProject`, `TestUpdateProject`…). Copié pour une
  nouvelle entité, le fichier se renomme en entier (`newInvoiceService`, `TestCreateInvoice`), sans
  conflit de noms.

```go
package service_test

func newInvoiceService(repo *repositorytest.InvoiceRepository, rec *repositorytest.AuditRecorder) *service.InvoiceService {
	return service.NewInvoiceService(repo, repositorytest.Tx{}, rec, steppingClock())
}
```

```bash
go test ./internal/service/...
go test ./internal/service/... -run TestCreateProject
```
