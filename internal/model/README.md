# `model/` — entités, erreurs métier, interfaces

Le cœur du métier : les entités et leurs invariants, les erreurs métier, et les interfaces (ports)
dont dépendent les services. Aucune implémentation technique ici : ce paquet se teste sans base ni
serveur.

## Ce qui va ici

- Les entités (`Project`) et leur constructeur qui normalise et valide (`NewProject`), les
  modifications validées (`Project.Apply` avec un `ProjectPatch`).
- Les erreurs métier : des sentinelles `apperr` (`ErrProjectNotFound`, `ErrProjectNameTaken`,
  `ErrProjectInvalid`), à comparer avec `errors.Is`.
- Les limites partagées avec la base (`ProjectMaxNameLength`, `ProjectMaxDescriptionLength`).
- Les interfaces de persistance et leurs types de requête (`ProjectRepository`, `ProjectListQuery`).
- Les interfaces transverses : `Transactor` (`transactor.go`), `AuditRecorder` et `AuditEvent`
  (`audit.go`).

## Ce qui n'y va pas

- Le SQL et les implémentations des interfaces → [`repository/`](../repository/) (et les fakes →
  [`repository/repositorytest/`](../repository/repositorytest/)).
- Les tags `json`, `validate` ou `db` : le JSON vit dans [`dto/`](../dto/), les colonnes dans
  `repository/`.
- L'orchestration (transactions, audit, enchaînement d'appels) → [`service/`](../service/).
- L'interface qu'un contrôleur attend d'un service → `controller/` (côté consommateur).

## Dépendances

Dans `internal/`, `model/` n'importe que `utils/apperr`. Hors du module : la bibliothèque standard et
des bibliothèques tierces sans transport ni stockage (un type décimal, par exemple), jamais `net/http`,
`database/sql`, pgx, go-redis ni chi (vérifié par `internal/archtest`). Tous les autres dossiers métier
l'importent.

## Conventions

- Un fichier par entité (`project.go`), un fichier par interface transverse (`audit.go`,
  `transactor.go`).
- Le paquet est partagé par toutes les entités : préfixez chaque nom par l'entité (`ErrProjectNotFound`,
  `ProjectMaxNameLength`, `ProjectListQuery`).
- Codes d'erreur `<entité>.<raison>` en snake_case (`project.name_taken`). Ils sont publics et stables :
  ne jamais en renommer un.
- Les invariants vivent dans le constructeur, pas seulement dans les tags `validate` du DTO : un cron ou
  un autre service qui crée l'entité est protégé de la même façon. Une violation renvoie
  `Err<Entité>Invalid.WithFields(apperr.FieldError{…})`.
- Les méthodes des interfaces filtrent par propriétaire et documentent leurs erreurs : une ressource
  d'un autre utilisateur est `Err<Entité>NotFound`, jamais « interdit ».

## Exemple

Référence : [`project.go`](project.go). Pas-à-pas complet :
[`docs/adding-a-feature.md`](../../docs/adding-a-feature.md). Squelette pour une entité `invoice`
(`model/invoice.go`) :

```go
package model

import (
	"context"
	"time"
	"uuid"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

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

// InvoiceRepository is the persistence port, implemented by repository.InvoiceRepository.
type InvoiceRepository interface {
	Create(ctx context.Context, inv Invoice) error
	// Get returns ErrInvoiceNotFound if the invoice does not exist for this owner.
	Get(ctx context.Context, ownerID string, id uuid.UUID) (Invoice, error)
}
```

## Tester

Les invariants sont testés à travers le service (`TestCreateProjectRejectsInvalidInput` dans
`service/project_service_test.go`). Une règle plus riche mérite son propre `model/<entité>_test.go`
(paquet `model_test`, sans base ni fake).

```bash
go test ./internal/model/... ./internal/service/...
```
