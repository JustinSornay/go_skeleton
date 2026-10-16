# Ajouter une fonctionnalité

Exemple : une fonctionnalité `invoice` (les factures d'un utilisateur). Les étapes 1 à 9 construisent
`POST /v1/invoices` et `GET /v1/invoices/{id}` ; la section
[« Lister, modifier, supprimer »](#lister-modifier-supprimer) complète le CRUD. Le code est rangé par
couche : une fonctionnalité, c'est **un fichier par couche**, nommé d'après l'entité. Le plus rapide
est de copier les fichiers `project_*`. Ce guide donne l'ordre, le contenu minimal de chaque fichier et
les règles de dépendances qui s'appliquent.
Le README de chaque dossier détaille ses conventions, et la carte des dossiers est dans
[`internal/README.md`](../internal/README.md).

| Étape | Fichier(s) | Détails |
|---|---|---|
| 1 | `internal/model/invoice.go` | [model](../internal/model/README.md) |
| 2 | `migrations/0000N_invoices.sql`, `internal/repository/invoice_repository.go` | [repository](../internal/repository/README.md) |
| 3 | `internal/repository/repositorytest/` (fake + contrat), `internal/repository/invoice_repository_test.go` | [repositorytest](../internal/repository/repositorytest/README.md) |
| 4 | `internal/service/invoice_service.go` | [service](../internal/service/README.md) |
| 5 | `internal/dto/invoice_dto.go` | [dto](../internal/dto/README.md) |
| 6 | `internal/controller/invoice_controller.go` | [controller](../internal/controller/README.md) |
| 7 | `internal/app/router.go` | [app](../internal/app/README.md) |
| 8 | `api/openapi.yaml` | `TestRoutesMatchOpenAPI` |
| 9 | les `*_test.go`, puis `make check` | ci-dessous |

Les paquets sont partagés par toutes les fonctionnalités. Il faut donc préfixer chaque identifiant
par l'entité, y compris les non exportés : `ErrInvoiceNotFound`, `InvoiceRepository`, `invoiceRow`,
`mapInvoiceErr`. Les tests suivent la même règle : les aides communes (`mount`, `client`, `decode`
dans `controller/controller_test.go`, `steppingClock` dans `service/service_test.go`) se réutilisent,
et tout ce qui est propre à project est préfixé (`newProjectServer`, `TestCreateProject`). Un fichier
`project_*_test.go` copié se renomme donc en entier, sans conflit de noms.

Après chaque étape, `make test` doit passer, avec deux nuances :

- le SQL de l'étape 2 ne s'exécute qu'avec `make test-integration` (tag `integration`) ;
- les étapes 7 et 8 vont ensemble : dès qu'une route est montée sans être documentée dans
  `api/openapi.yaml` (ou l'inverse), `TestRoutesMatchOpenAPI` échoue.

Si l'échec vient de `internal/archtest`, c'est qu'un import viole les règles de dépendances : corrigez
la conception, pas le test. Le message indique le paquet, l'import et la règle enfreinte.

## 1. Modèle : `internal/model/invoice.go`

Ce fichier contient l'entité, ses invariants, ses erreurs et l'interface de persistance. Dans
`internal/`, il n'importe que `utils/apperr` ; ailleurs, la bibliothèque standard et des bibliothèques
tierces sans transport ni stockage, jamais `net/http`, SQL, pgx, go-redis ni chi.

```go
// Invoice errors. Codes are public and stable: never rename them.
var (
	ErrInvoiceNotFound = apperr.NotFound("invoice.not_found", "invoice not found")
	ErrInvoiceInvalid  = apperr.Invalid("invoice.invalid", "invoice is invalid")
)

// Invoice is owned by a single user, identified by the auth subject.
type Invoice struct {
	ID          uuid.UUID
	OwnerID     string
	AmountCents int64
	CreatedAt   time.Time
}

// NewInvoice builds a valid Invoice.
func NewInvoice(id uuid.UUID, ownerID string, amountCents int64, now time.Time) (Invoice, error) {
	var fields []apperr.FieldError
	if ownerID == "" {
		fields = append(fields, apperr.FieldError{Field: "owner_id", Rule: "required", Message: "is required"})
	}
	if amountCents <= 0 {
		fields = append(fields, apperr.FieldError{Field: "amount_cents", Rule: "gt", Message: "must be positive"})
	}
	if len(fields) > 0 {
		return Invoice{}, ErrInvoiceInvalid.WithFields(fields...)
	}
	return Invoice{ID: id, OwnerID: ownerID, AmountCents: amountCents, CreatedAt: now}, nil
}

// InvoiceRepository is the persistence port, implemented by
// repository.InvoiceRepository. Reads are scoped by owner.
type InvoiceRepository interface {
	Create(ctx context.Context, inv Invoice) error
	// Get returns ErrInvoiceNotFound if the invoice does not exist for this owner.
	Get(ctx context.Context, ownerID string, id uuid.UUID) (Invoice, error)
}
```

- Les codes d'erreur suivent la forme `<entité>.<raison>` et ne changent jamais : les clients s'en
  servent.
- Les invariants vivent dans le constructeur, pas seulement dans les tags `validate` du DTO : un cron
  ou un autre service qui crée une facture est protégé de la même façon.
- La ressource d'un autre utilisateur renvoie `ErrInvoiceNotFound` (404), jamais « interdit ».

## 2. Persistance : migration et `internal/repository/invoice_repository.go`

```bash
make migration name=invoices     # crée migrations/0000N_invoices.sql (numéro suivant)
```

```sql
-- +goose Up
CREATE TABLE invoices (
    id           uuid        PRIMARY KEY,
    owner_id     text        NOT NULL,
    amount_cents bigint      NOT NULL CHECK (amount_cents > 0),
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Serves the listing order: newest first, id as tie-breaker.
CREATE INDEX invoices_owner_created_idx ON invoices (owner_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE invoices;
```

Écrivez `Up` **et** `Down`. Ne modifiez jamais une migration déjà appliquée quelque part : ajoutez-en
une nouvelle.

Le repository implémente `model.InvoiceRepository`. Il n'importe que `model`,
`infrastructure/database`, `utils/*` et pgx, jamais `net/http` ni chi.

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

- Utilisez toujours `r.db.Q(ctx)` : la requête rejoint alors la transaction ouverte par le service.
- Chaque requête filtre sur `owner_id`.
- Traduisez les erreurs du driver en erreurs du modèle : `database.IsNoRows`, ou
  `database.UniqueViolation(err)` comparé au nom de la contrainte. Toute autre erreur est enveloppée
  et deviendra un 500.

## 3. Fake et contrat : `internal/repository/repositorytest/`

Le contrat décrit le comportement attendu de **toute** implémentation. Il est exécuté contre le fake
en mémoire et contre PostgreSQL, si bien que le fake utilisé par les tests unitaires ne peut pas
diverger de la base réelle.

- Le fake `InvoiceRepository` et `NewInvoiceRepository()` se calquent sur `ProjectRepository` (dans
  `memory.go`) : un `sync.Mutex`, un champ `Err` pour injecter une panne, et la même sémantique que
  PostgreSQL. Le squelette complet est dans le
  [README du dossier](../internal/repository/repositorytest/README.md).
- Le contrat `InvoiceRepositoryContract`, à placer dans `contract.go` (ou dans `invoice_memory.go` et
  `invoice_contract.go` quand les fichiers s'allongent) :

```go
// InvoiceRepositoryContract runs the behaviour every model.InvoiceRepository
// must have, against the fake and against PostgreSQL.
func InvoiceRepositoryContract(t *testing.T, newRepo func(t *testing.T) model.InvoiceRepository) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("create and get, scoped by owner", func(t *testing.T) {
		repo := newRepo(t)
		inv, err := model.NewInvoice(uuid.NewV7(), "alice", 1200, t0)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatal(err)
		}
		if got, err := repo.Get(ctx, "alice", inv.ID); err != nil || got != inv {
			t.Fatalf("got %+v, %v; want %+v", got, err, inv)
		}
		if _, err := repo.Get(ctx, "bob", inv.ID); !errors.Is(err, model.ErrInvoiceNotFound) {
			t.Fatalf("want ErrInvoiceNotFound, got %v", err)
		}
	})
}
```

Les tests sont en paquet externe (`<couche>_test`), comme partout dans le dépôt. Exécutez le contrat sur
le fake, dans `repositorytest/memory_test.go` (paquet `repositorytest_test`, déjà présent) :

```go
func TestMemoryInvoiceRepositoryContract(t *testing.T) {
	repositorytest.InvoiceRepositoryContract(t, func(*testing.T) model.InvoiceRepository {
		return repositorytest.NewInvoiceRepository()
	})
}
```

Puis sur PostgreSQL, dans `internal/repository/invoice_repository_test.go`. N'ajoutez pas de
`TestMain` : le paquet n'en a qu'un, dans `audit_repository_test.go`.

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

## 4. Service : `internal/service/invoice_service.go`

Le service contient les cas d'usage. Il n'importe que `model` et `utils/*` (sauf `utils/httpx`) :
jamais `net/http`, SQL, pgx, go-redis ni chi. Ses dépendances sont les interfaces de `model`, passées
au constructeur. Il embarque les options communes déclarées dans `service.go` (`Option`, `WithClock`,
`WithIDGenerator`).

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

// Get returns one of ownerID's invoices.
func (s *InvoiceService) Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Invoice, error) {
	return s.repo.Get(ctx, ownerID, id)
}
```

- Une mutation sensible et son `model.AuditEvent` s'écrivent dans le même `s.tx.WithinTx`. L'erreur
  de `Record` est renvoyée, jamais ignorée.
- L'heure vient de `s.now()` et les identifiants de `s.newID()`, jamais de `time.Now()` en direct.
- Pour un cache ou un client externe, déclarez l'interface dans `model` avec des types simples ou
  standard, pour que le client d'`infrastructure/` la satisfasse sans importer `model`, et laissez
  `app` brancher l'implémentation. Si l'interface a besoin de types du modèle, l'adaptateur vit dans
  `app/` (voir [architecture.md](architecture.md#cache-et-services-externes)).

## 5. DTO : `internal/dto/invoice_dto.go`

Ce fichier définit le contrat JSON public. Il n'importe que `model` et `utils/*`.

```go
// CreateInvoiceRequest is the body of POST /v1/invoices.
type CreateInvoiceRequest struct {
	AmountCents int64 `json:"amount_cents" validate:"required,gt=0"`
}

// InvoiceResponse is the JSON representation of an invoice.
type InvoiceResponse struct {
	ID          string    `json:"id"`
	AmountCents int64     `json:"amount_cents"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToInvoiceResponse maps a model.Invoice to its JSON representation.
func ToInvoiceResponse(inv model.Invoice) InvoiceResponse {
	return InvoiceResponse{ID: inv.ID.String(), AmountCents: inv.AmountCents, CreatedAt: inv.CreatedAt}
}
```

Les tags `validate` ne vérifient que la forme de la requête, le modèle revalide le reste. Ne
sérialisez jamais un type du modèle directement : `OwnerID` n'apparaît pas dans la réponse.

## 6. Contrôleur : `internal/controller/invoice_controller.go`

Le contrôleur n'importe que `service`, `dto`, `model`, `utils/*` et `infrastructure/auth`, jamais
`repository` ni `infrastructure/database`. Il déclare lui-même l'interface dont il a besoin (côté
consommateur), et `*service.InvoiceService` la satisfait.

```go
// InvoiceService is what InvoiceController needs (consumer-side interface);
// *service.InvoiceService implements it.
type InvoiceService interface {
	Create(ctx context.Context, ownerID string, in service.CreateInvoiceInput) (model.Invoice, error)
	Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Invoice, error)
}

// InvoiceController serves /v1/invoices.
type InvoiceController struct {
	svc InvoiceService
}

// NewInvoiceController returns an invoice controller.
func NewInvoiceController(svc InvoiceService) *InvoiceController {
	return &InvoiceController{svc: svc}
}

// Routes returns the controller's router, to be mounted by internal/app.
func (c *InvoiceController) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", httpx.Handle(c.create))
	r.Get("/{id}", httpx.Handle(c.get))
	return r
}

func (c *InvoiceController) create(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	req, err := httpx.Bind[dto.CreateInvoiceRequest](r)
	if err != nil {
		return err
	}
	inv, err := c.svc.Create(r.Context(), owner, service.CreateInvoiceInput{AmountCents: req.AmountCents})
	if err != nil {
		return err
	}
	w.Header().Set("Location", r.URL.Path+"/"+inv.ID.String())
	return httpx.JSON(w, http.StatusCreated, dto.ToInvoiceResponse(inv))
}
```

`get` suit le même schéma que `ProjectController.get`. Quand `httpx.PathUUID(r, "id")` échoue, il
renvoie `model.ErrInvoiceNotFound`. Les handlers **renvoient** leurs erreurs : jamais de `panic`, ni de
réponse d'erreur écrite à la main.

## 7. Câblage : `internal/app/router.go`

Ajoutez une fonction de construction par fonctionnalité, sur le modèle de `projectModule`. `d.DB` sert
à la fois de connexion aux repositories et de `model.Transactor` au service.

```go
func invoiceModule(d Deps) *controller.InvoiceController {
	repo := repository.NewInvoiceRepository(d.DB)
	svc := service.NewInvoiceService(repo, d.DB, repository.NewAuditRepository(d.DB))
	return controller.NewInvoiceController(svc)
}

// in NewRouter, inside the /v1 group protected by mw.RequireAuth:
r.Mount("/invoices", invoiceModule(d).Routes())
// or, restricted to a role:
r.With(mw.RequireRole("billing")).Mount("/invoices", invoiceModule(d).Routes())
```

La politique d'accès reste visible dans `router.go`, au même endroit pour toute l'API. Faites l'étape 8
dans le même changement : tant que les routes montées ne sont pas documentées, `make test` échoue sur
`TestRoutesMatchOpenAPI`.

## 8. Contrat d'API : `api/openapi.yaml`

Documentez chaque nouvelle route, ses schémas et ses réponses d'erreur, en réutilisant
`#/components/responses/*`. `TestRoutesMatchOpenAPI` échoue tant que les routes et la documentation ne
correspondent pas. Ce test ne contrôle pas les schémas : à vous de les tenir à jour.

Le fichier existe déjà : **ne dupliquez pas** les clés `tags:`, `paths:` et `components:` (le YAML
deviendrait invalide). Ajoutez :

- `- name: Invoices` à la liste `tags:` en tête du fichier ;
- les deux chemins ci-dessous sous le `paths:` existant ;
- les schémas sous le `components.schemas:` existant.

```yaml
# under the existing `tags:`
  - name: Invoices

# under the existing `paths:`
  /v1/invoices:
    post:
      tags: [Invoices]
      summary: Create an invoice
      operationId: createInvoice
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/CreateInvoiceRequest" }
      responses:
        "201":
          description: Created.
          headers:
            Location:
              description: URL of the new invoice.
              schema: { type: string }
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Invoice" }
        "400": { $ref: "#/components/responses/BadRequest" }
        "401": { $ref: "#/components/responses/Unauthorized" }
  /v1/invoices/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: string, format: uuid } }
    get:
      tags: [Invoices]
      summary: Get an invoice
      operationId: getInvoice
      responses:
        "200":
          description: The invoice.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Invoice" }
        "401": { $ref: "#/components/responses/Unauthorized" }
        "404": { $ref: "#/components/responses/NotFound" }

# under the existing `components:` → `schemas:`
    Invoice:
      type: object
      required: [id, amount_cents, created_at]
      properties:
        id: { type: string, format: uuid }
        amount_cents: { type: integer, format: int64, minimum: 1 }
        created_at: { type: string, format: date-time }
    CreateInvoiceRequest:
      type: object
      required: [amount_cents]
      additionalProperties: false
      properties:
        amount_cents: { type: integer, format: int64, minimum: 1 }
```

## 9. Tests

Les tests sont en paquet externe (`package service_test`, `package controller_test`…) et réutilisent
les aides communes de leur dossier. Ne copiez pas ces aides : elles existent déjà, et une seconde
déclaration ne compile pas.

| Quoi | Fichier | Avec |
|---|---|---|
| Cas d'usage | `internal/service/invoice_service_test.go` | fakes : `repositorytest.NewInvoiceRepository()`, `repositorytest.Tx{}`, `&repositorytest.AuditRecorder{}` ; horloge `steppingClock()` (`service_test.go`) |
| HTTP | `internal/controller/invoice_controller_test.go` | vrai service sur les fakes ; `mount`, `client`, `decode` (`controller_test.go`) |
| Contrat sur le fake | `internal/repository/repositorytest/memory_test.go` | aucune base |
| Contrat sur PostgreSQL | `internal/repository/invoice_repository_test.go` | tag `integration`, `dbtest.New(t)` |
| Routes ↔ OpenAPI | `internal/app/router_test.go` (existant) | rien à écrire |
| Bout en bout (facultatif) | `internal/app/app_test.go` | tag `integration`, routeur complet |

`internal/service/invoice_service_test.go` :

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
	"github.com/JustinSornay/go_skeleton/internal/service"
)

func newInvoiceService(repo *repositorytest.InvoiceRepository, rec *repositorytest.AuditRecorder) *service.InvoiceService {
	return service.NewInvoiceService(repo, repositorytest.Tx{}, rec, steppingClock())
}

func TestCreateInvoice(t *testing.T) {
	rec := &repositorytest.AuditRecorder{}
	svc := newInvoiceService(repositorytest.NewInvoiceRepository(), rec)

	inv, err := svc.Create(context.Background(), "alice", service.CreateInvoiceInput{AmountCents: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if events := rec.Events(); len(events) != 1 || events[0].Action != "invoice.create" || events[0].TargetID != inv.ID.String() {
		t.Fatalf("audit events: %+v", events)
	}
	if _, err := svc.Create(context.Background(), "alice", service.CreateInvoiceInput{AmountCents: 0}); !errors.Is(err, model.ErrInvoiceInvalid) {
		t.Fatalf("want ErrInvoiceInvalid, got %v", err)
	}
}
```

`internal/controller/invoice_controller_test.go` :

```go
package controller_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/controller"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
	"github.com/JustinSornay/go_skeleton/internal/service"
)

func newInvoiceServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := service.NewInvoiceService(repositorytest.NewInvoiceRepository(), repositorytest.Tx{}, &repositorytest.AuditRecorder{})
	return mount(t, "/v1/invoices", controller.NewInvoiceController(svc).Routes())
}

type invoiceJSON struct {
	ID          string `json:"id"`
	AmountCents int64  `json:"amount_cents"`
}

func TestInvoiceCRUD(t *testing.T) {
	srv := newInvoiceServer(t)
	alice, bob := client{t, srv.URL, "alice"}, client{t, srv.URL, "bob"}

	resp, body := alice.do(http.MethodPost, "/v1/invoices", `{"amount_cents":1200}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := decode[invoiceJSON](t, body)
	if resp, _ := bob.do(http.MethodGet, "/v1/invoices/"+created.ID, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("another owner must get 404, got %d", resp.StatusCode)
	}
}
```

```bash
go test ./internal/service/... -run TestCreateInvoice
make test               # unitaires + règles d'architecture + TestRoutesMatchOpenAPI
make test-integration   # + contrat sur PostgreSQL et bout en bout
make check              # avant de pousser : lint + intégration -race + govulncheck
```

## Lister, modifier, supprimer

Le reste du CRUD suit les fichiers de project, couche par couche. Voici ce qui n'est pas évident.

**Modèle.** Une modification partielle passe par un patch dont les champs `nil` restent inchangés, et
par `Apply`, qui revalide l'entité. Sortez pour cela les contrôles de `NewInvoice` dans une méthode
`validate()`, comme `Project.validate`.

```go
// InvoicePatch is a partial update: nil fields are left unchanged.
type InvoicePatch struct {
	AmountCents *int64
}

// Empty reports whether the patch changes nothing.
func (pt InvoicePatch) Empty() bool { return pt.AmountCents == nil }

// Apply returns a copy of inv with the patch applied and validated.
func (inv Invoice) Apply(pt InvoicePatch) (Invoice, error) {
	if pt.AmountCents != nil {
		inv.AmountCents = *pt.AmountCents
	}
	return inv, inv.validate()
}

// InvoiceListQuery paginates InvoiceRepository.List.
type InvoiceListQuery struct {
	OwnerID string
	Limit   int
	Offset  int
}

// InvoiceRepository gains:
	// List returns one page of the owner's invoices, newest first, and the
	// total number of the owner's invoices.
	List(ctx context.Context, q InvoiceListQuery) ([]Invoice, int, error)
	// Update persists amount_cents. Returns ErrInvoiceNotFound.
	Update(ctx context.Context, inv Invoice) error
	// Delete returns ErrInvoiceNotFound if nothing was deleted.
	Delete(ctx context.Context, ownerID string, id uuid.UUID) error
```

**Repository.** `List` renvoie la page et le total en une requête, avec `count(*) OVER ()`. Deux pièges,
traités dans `ProjectRepository.List` :

- au-delà de la dernière page, aucune ligne ne porte le total : sans la seconde requête `count(*)`, le
  total renvoyé serait 0 ;
- l'ordre `created_at DESC, id DESC` départage deux factures créées au même instant ; il correspond à
  l'index de la migration, et le fake doit le reproduire.

```go
func (r *InvoiceRepository) List(ctx context.Context, q model.InvoiceListQuery) ([]model.Invoice, int, error) {
	rows, _ := r.db.Q(ctx).Query(ctx, `
		SELECT `+invoiceColumns+`, count(*) OVER () AS total
		FROM invoices
		WHERE owner_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`,
		q.OwnerID, q.Limit, q.Offset)
	type listRow struct {
		invoiceRow
		Total int `db:"total"`
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, 0, mapInvoiceErr(err, "list invoices")
	}
	if len(got) == 0 {
		if q.Offset == 0 {
			return []model.Invoice{}, 0, nil
		}
		// Past the last page: no row carries the window count, ask directly.
		var total int
		err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM invoices WHERE owner_id = $1`, q.OwnerID).Scan(&total)
		return []model.Invoice{}, total, mapInvoiceErr(err, "count invoices")
	}
	out := make([]model.Invoice, len(got))
	for i, g := range got {
		out[i] = g.toDomain() // the conversion of Get, moved to a method like row.toDomain
	}
	return out, got[0].Total, nil
}
```

`Update` et `Delete` filtrent sur `id` **et** `owner_id`, et traduisent « 0 ligne touchée »
(`tag.RowsAffected() == 0`) en `model.ErrInvoiceNotFound` : voir `ProjectRepository.Update` et
`Delete`.

**Fake et contrat.** Le `List` du fake trie comme le SQL :

```go
slices.SortFunc(all, func(a, b model.Invoice) int {
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	return cmp.Compare(b.ID.String(), a.ID.String()) // id DESC, as in SQL
})
```

Ajoutez au contrat un sous-test par comportement, sur le modèle des sous-tests `update`, `delete` et
`list filters, orders and paginates` de `ProjectRepositoryContract`. Pour la liste, vérifiez au moins
l'ordre et le total au-delà de la dernière page :

```go
t.Run("list pages newest first and counts past the last page", func(t *testing.T) {
	repo := newRepo(t)
	for i := range 3 {
		inv, _ := model.NewInvoice(uuid.NewV7(), "alice", 100, t0.Add(time.Duration(i)*time.Hour))
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := repo.List(ctx, model.InvoiceListQuery{OwnerID: "alice", Limit: 2})
	if err != nil || total != 3 || len(page) != 2 || !page[0].CreatedAt.After(page[1].CreatedAt) {
		t.Fatalf("first page: %+v total=%d err=%v", page, total, err)
	}
	page, total, err = repo.List(ctx, model.InvoiceListQuery{OwnerID: "alice", Limit: 2, Offset: 10})
	if err != nil || total != 3 || len(page) != 0 {
		t.Fatalf("past the last page: %+v total=%d err=%v", page, total, err)
	}
})
```

**Service.** `List` délègue au repository. `Update` relit l'entité dans la transaction ; un patch vide
la renvoie sans écrire ni auditer ; sinon `Apply`, `repo.Update` et l'audit `invoice.update`. `Delete`
enchaîne `repo.Delete` et l'audit `invoice.delete`. Voir `ProjectService.Update` et `Delete`.

```go
// Update applies a partial update and audits it atomically.
func (s *InvoiceService) Update(ctx context.Context, ownerID string, id uuid.UUID, pt model.InvoicePatch) (model.Invoice, error) {
	var updated model.Invoice
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		cur, err := s.repo.Get(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if pt.Empty() {
			updated = cur // nothing to write, nothing to audit
			return nil
		}
		if updated, err = cur.Apply(pt); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, updated); err != nil {
			return err
		}
		return s.audit.Record(ctx, model.AuditEvent{Action: "invoice.update", TargetType: "invoice", TargetID: id.String()})
	})
	if err != nil {
		return model.Invoice{}, fmt.Errorf("update invoice: %w", err)
	}
	return updated, nil
}
```

**DTO.** Le corps de `PATCH` utilise des pointeurs (absent = inchangé) ; la query string de la liste
embarque `httpx.PageQuery` (`limit`, `offset`).

```go
// UpdateInvoiceRequest is a partial update: absent fields are left unchanged.
type UpdateInvoiceRequest struct {
	AmountCents *int64 `json:"amount_cents" validate:"omitnil,gt=0"`
}

// ListInvoicesRequest is the query string of GET /v1/invoices.
type ListInvoicesRequest struct {
	httpx.PageQuery
}
```

**Contrôleur.** Ajoutez `List`, `Update` et `Delete` à l'interface `InvoiceService`, puis les routes
`r.Get("/", …list)`, `r.Patch("/{id}", …update)` et `r.Delete("/{id}", …delete)`. `update` et
`delete` suivent `ProjectController.update` et `delete` (`httpx.NoContent(w)` pour un 204). La liste :

```go
func (c *InvoiceController) list(w http.ResponseWriter, r *http.Request) error {
	owner, err := currentUser(r)
	if err != nil {
		return err
	}
	var req dto.ListInvoicesRequest
	if err := httpx.BindQuery(r, &req); err != nil {
		return err
	}
	page := req.Normalize() // limit 20 by default, 100 at most
	items, total, err := c.svc.List(r.Context(), model.InvoiceListQuery{OwnerID: owner, Limit: page.Limit, Offset: page.Offset})
	if err != nil {
		return err
	}
	out := httpx.List[dto.InvoiceResponse]{
		Data: make([]dto.InvoiceResponse, len(items)),
		Page: httpx.Page{Limit: page.Limit, Offset: page.Offset, Total: total},
	}
	for i, inv := range items {
		out.Data[i] = dto.ToInvoiceResponse(inv)
	}
	return httpx.JSON(w, http.StatusOK, out)
}
```

**OpenAPI.** Sur le modèle de `listProjects`, `updateProject` et `deleteProject` :

```yaml
# under the existing /v1/invoices path
    get:
      tags: [Invoices]
      summary: List my invoices
      operationId: listInvoices
      parameters:
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: One page of invoices, newest first.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/InvoiceList" }
        "400": { $ref: "#/components/responses/BadRequest" }
        "401": { $ref: "#/components/responses/Unauthorized" }

# under the existing /v1/invoices/{id} path
    patch:
      tags: [Invoices]
      summary: Update an invoice
      operationId: updateInvoice
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/UpdateInvoiceRequest" }
      responses:
        "200":
          description: The updated invoice.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Invoice" }
        "400": { $ref: "#/components/responses/BadRequest" }
        "401": { $ref: "#/components/responses/Unauthorized" }
        "404": { $ref: "#/components/responses/NotFound" }
    delete:
      tags: [Invoices]
      summary: Delete an invoice
      operationId: deleteInvoice
      responses:
        "204": { description: Deleted. }
        "401": { $ref: "#/components/responses/Unauthorized" }
        "404": { $ref: "#/components/responses/NotFound" }

# under the existing `components:` → `schemas:`
    InvoiceList:
      type: object
      required: [data, page]
      properties:
        data:
          type: array
          items: { $ref: "#/components/schemas/Invoice" }
        page: { $ref: "#/components/schemas/Page" }
    UpdateInvoiceRequest:
      type: object
      additionalProperties: false
      properties:
        amount_cents: { type: integer, format: int64, minimum: 1 }
```

## Tâches planifiées

Une tâche planifiée est un binaire `cmd/<job>` construit sur le modèle de `cmd/retention` :

- `main()` minimal et `run()` testable ;
- `--dry-run`, traitement par lots et délai global ;
- un acteur `actor.Cron("<job>")` dans le contexte, pour la traçabilité.

C'est la racine de composition de son binaire, au même titre que `app/` pour l'API (`internal/archtest`
le permet). Elle ne lit que la configuration dont elle a besoin (`config.LoadDatabase`), ouvre la base,
puis construit elle-même les repositories et services qu'elle utilise, comme `app/router.go`, au lieu
de démarrer toute l'application. Le SQL reste dans `internal/repository` : `cmd/retention` appelle
`AuditRepository.CountOlderThan` et `DeleteOlderThan`, et ne garde que la boucle par lots.
