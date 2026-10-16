# `dto/` — contrat JSON de l'API

Les types des requêtes et des réponses HTTP : le contrat public, documenté dans `api/openapi.yaml`.
Ils sont distincts des types de [`model/`](../model/) pour que le modèle puisse évoluer sans casser
les clients.

## Ce qui va ici

- Les corps de requête avec tags `json` et `validate` : `CreateProjectRequest`,
  `UpdateProjectRequest` (champs pointeurs : absent = inchangé, `validate:"omitnil,…"`).
- Les paramètres de query string avec tags `query` : `ListProjectsRequest`, qui embarque
  `httpx.PageQuery` (`limit`, `offset`).
- Les réponses (`ProjectResponse`) et leur fonction de conversion explicite depuis le modèle
  (`ToProjectResponse`).

## Ce qui n'y va pas

- Les invariants métier : les tags `validate` ne vérifient que la forme, le modèle revalide tout
  (`model.NewProject`).
- Le décodage, les appels au service, l'écriture de la réponse → [`controller/`](../controller/).
- Un type du modèle sérialisé tel quel, ou un champ interne exposé (`OwnerID` n'apparaît pas dans
  `ProjectResponse`).

## Dépendances

`model/` et `utils/*` (pour `httpx.PageQuery`). Ni `service/`, ni `controller/`, ni `repository/`
(vérifié par `internal/archtest`). Importé par `controller/`.

## Conventions

- Fichier `<entité>_dto.go` ; types `<Verbe><Entité>Request`, `List<Entités>Request`,
  `<Entité>Response` ; conversion `To<Entité>Response(model.<Entité>) <Entité>Response`.
- Noms JSON en snake_case (`created_at`), identifiants en `string`, dates en `time.Time` (RFC 3339).
- Les limites des tags reprennent celles du modèle (`max=100` ↔ `model.ProjectMaxNameLength`) : les
  garder synchronisées à la main.
- `httpx.Bind` refuse les champs JSON inconnus (`request.malformed_body`) ; un échec de validation
  renvoie `request.invalid` avec la liste des champs, nommés d'après leurs tags `json`/`query`.
- Changer un DTO, c'est changer le contrat public : mettez à jour les schémas d'`api/openapi.yaml`
  (`TestRoutesMatchOpenAPI` ne contrôle que les routes, pas les schémas).

## Exemple

Référence : [`project_dto.go`](project_dto.go). Pas-à-pas complet :
[`docs/adding-a-feature.md`](../../docs/adding-a-feature.md). Squelette pour `invoice`
(`dto/invoice_dto.go`) :

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

## Tester

Les DTO sont couverts par les tests du contrôleur, qui envoient du vrai JSON (`TestProjectErrors`
dans `controller/project_controller_test.go` : champ manquant, champ inconnu, `limit` invalide). Une
conversion qui contient de la logique mérite un `dto/<entité>_dto_test.go`.

```bash
go test ./internal/controller/...
```
