# `repository/repositorytest/` — kit de test des repositories

Des fakes en mémoire des interfaces de [`model/`](../../model/), écrits à la main (pas de mocks
générés), et les **contrats** : la suite de comportements que toute implémentation doit respecter.
Le même contrat tourne sur le fake et sur PostgreSQL, ce qui garantit que les tests unitaires des
services et contrôleurs reposent sur un fake fidèle à la production.

## Ce qui va ici

| Fichier | Contenu |
|---|---|
| [`memory.go`](memory.go) | doc du paquet, fake `ProjectRepository` (`NewProjectRepository()`), `Tx` (implémente `model.Transactor` en appelant `fn` directement) |
| [`contract.go`](contract.go) | `ProjectRepositoryContract(t, newRepo)` |
| [`audit.go`](audit.go) | `AuditRecorder` : garde les événements (`Events()`), échoue sur demande (`Err`) ; sa valeur zéro est prête à l'emploi |
| [`memory_test.go`](memory_test.go) | `TestMemoryProjectRepositoryContract` : le contrat exécuté sur le fake |

## Ce qui n'y va pas

- Les tests eux-mêmes d'un service ou d'un contrôleur → dans leur dossier (`*_test.go`).
- Du SQL : l'exécution du contrat sur PostgreSQL est dans `repository/<entité>_repository_test.go`.

## Dépendances

Les mêmes que [`repository/`](../) : `model/`, `infrastructure/database`, `utils/*`, plus `testing`
pour les contrats. Jamais `net/http` ni chi.

C'est un kit de test (nom en `…test`) : seuls des fichiers `*_test.go` l'importent (services,
contrôleurs, tests d'intégration), jamais du code de production (vérifié par `internal/archtest`).

## Conventions

- Le fake porte le nom de l'interface (`repositorytest.ProjectRepository`, `NewProjectRepository()`),
  est sûr en concurrence (`sync.Mutex`) et expose un champ `Err` renvoyé par toutes les méthodes.
- Il reproduit la sémantique PostgreSQL : filtrage par propriétaire, noms uniques sans casse, tri du
  plus récent au plus ancien. Tout comportement dont un service dépend doit figurer dans le contrat, sinon
  le fake peut dériver sans que rien ne casse.
- Contrat : `<Interface>Contract(t *testing.T, newRepo func(t *testing.T) model.<Interface>)`, un
  `t.Run` par comportement, un `newRepo(t)` neuf par sous-test.
- `Tx{}` n'annule rien : un test qui vérifie un rollback va en intégration (voir
  `TestAuditRecordIsTransactional`).
- Un fake et un contrat par interface de repository, dans `memory.go` et `contract.go` ; découpez par
  entité (`invoice_memory.go`, `invoice_contract.go`) quand un fichier devient long. `AuditRecorder` et
  `Tx` sont de simples doublures de test, sans contrat : ils n'imitent aucune règle de PostgreSQL dont
  un service dépendrait.

## Exemple

Référence : le fake et le contrat de project. Pas-à-pas complet :
[`docs/adding-a-feature.md`](../../../docs/adding-a-feature.md). Squelette pour `invoice` :

```go
// InvoiceRepository is a goroutine-safe fake model.InvoiceRepository.
type InvoiceRepository struct {
	mu       sync.Mutex
	invoices map[uuid.UUID]model.Invoice

	// Err, when set, is returned by every method (failure injection).
	Err error
}

// NewInvoiceRepository returns an empty repository.
func NewInvoiceRepository() *InvoiceRepository {
	return &InvoiceRepository{invoices: make(map[uuid.UUID]model.Invoice)}
}

var _ model.InvoiceRepository = (*InvoiceRepository)(nil)

func (r *InvoiceRepository) Get(_ context.Context, ownerID string, id uuid.UUID) (model.Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return model.Invoice{}, r.Err
	}
	inv, ok := r.invoices[id]
	if !ok || inv.OwnerID != ownerID {
		return model.Invoice{}, model.ErrInvoiceNotFound
	}
	return inv, nil
}

// Create: same pattern (lock, Err, store).

// InvoiceRepositoryContract runs the behaviour every model.InvoiceRepository must have.
func InvoiceRepositoryContract(t *testing.T, newRepo func(t *testing.T) model.InvoiceRepository) {
	ctx := context.Background()
	t.Run("get is scoped by owner", func(t *testing.T) {
		repo := newRepo(t)
		inv, _ := model.NewInvoice(uuid.NewV7(), "alice", 1200, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Get(ctx, "bob", inv.ID); !errors.Is(err, model.ErrInvoiceNotFound) {
			t.Fatalf("want ErrInvoiceNotFound, got %v", err)
		}
	})
}
```

Puis dans `memory_test.go` (paquet externe `repositorytest_test`) :

```go
package repositorytest_test

func TestMemoryInvoiceRepositoryContract(t *testing.T) {
	repositorytest.InvoiceRepositoryContract(t, func(*testing.T) model.InvoiceRepository {
		return repositorytest.NewInvoiceRepository()
	})
}
```

## Tester

```bash
go test ./internal/repository/repositorytest/...    # contrat sur le fake, sans Docker
make test-integration                               # le même contrat sur PostgreSQL
```
