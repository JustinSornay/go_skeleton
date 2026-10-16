# `controller/` — contrôleurs HTTP

Un contrôleur expose les cas d'usage d'une entité en HTTP : il identifie l'utilisateur, décode et
valide la requête dans un type de [`dto/`](../dto/), appelle son service et écrit la réponse. Ses
routes (`Routes() chi.Router`) sont montées par [`app/`](../app/).

## Ce qui va ici

- [`controller.go`](controller.go) : la doc du paquet et l'aide commune `currentUser(r)` (sujet
  authentifié, ou 401 `auth.required`).
- Un fichier par entité : `ProjectController`, `NewProjectController(svc ProjectService)`, `Routes()`
  et les handlers non exportés (`list`, `create`, `get`, `update`, `delete`).
- L'interface `ProjectService`, déclarée ici côté consommateur avec les seules méthodes utilisées ;
  `*service.ProjectService` l'implémente.

## Ce qui n'y va pas

- Règles métier et transactions → [`service/`](../service/) et [`model/`](../model/).
- SQL, repositories, base de données : interdits ici.
- La politique d'accès (`RequireAuth`, `RequireRole`) → déclarée dans `app/router.go`, pas ici.
- Les réponses d'erreur écrites à la main, les `panic` : le handler **renvoie** l'erreur.

## Dépendances

`service/`, `dto/`, `model/`, `utils/*`, `infrastructure/auth` (plus `net/http` et chi). Jamais
`repository/`, `infrastructure/database` ni un kit de test hors `*_test.go` (vérifié par
`internal/archtest`). Seul `app/` l'importe.

## Conventions

- Fichier `<entité>_controller.go`, type `<Entité>Controller`, constructeur `New<Entité>Controller`,
  interface `<Entité>Service`.
- Handlers `func(w http.ResponseWriter, r *http.Request) error`, enveloppés par `httpx.Handle` dans
  `Routes()`. Une `*apperr.Error` devient le bon statut (404, 409, 400…), toute autre erreur un 500
  opaque.
- Entrées : `httpx.Bind[T](r)` (corps JSON + validation), `httpx.BindQuery(r, &req)` puis
  `req.Normalize()` (pagination), `httpx.PathUUID(r, "id")` ; si l'identifiant est illisible, renvoyer
  `model.Err<Entité>NotFound`.
- Sorties : `httpx.JSON(w, status, dto.To<Entité>Response(…))`, `httpx.NoContent(w)`, listes dans
  `httpx.List[T]{Data: …, Page: httpx.Page{…}}`. Un `POST` pose l'en-tête `Location`.
- La ressource d'un autre utilisateur répond 404, jamais 403 : le service renvoie déjà `NotFound`.

## Exemple

Référence : [`project_controller.go`](project_controller.go). Pas-à-pas complet, liste, modification
et suppression comprises : [`docs/adding-a-feature.md`](../../docs/adding-a-feature.md). Squelette pour
`invoice` (`controller/invoice_controller.go`) :

```go
// InvoiceService is what InvoiceController needs (consumer-side interface);
// *service.InvoiceService implements it.
type InvoiceService interface {
	Create(ctx context.Context, ownerID string, in service.CreateInvoiceInput) (model.Invoice, error)
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

Câblage dans `app/router.go`, sur le modèle de `projectModule` :

```go
func invoiceModule(d Deps) *controller.InvoiceController {
	svc := service.NewInvoiceService(repository.NewInvoiceRepository(d.DB), d.DB, repository.NewAuditRepository(d.DB))
	return controller.NewInvoiceController(svc)
}

// in NewRouter, inside the /v1 group protected by mw.RequireAuth:
r.Mount("/invoices", invoiceModule(d).Routes())
```

Puis documenter les routes dans `api/openapi.yaml`, dans le même changement que le `Mount` :
`TestRoutesMatchOpenAPI` échoue sinon.

## Tester

Les tests sont en paquet externe `controller_test` et servent le contrôleur avec le vrai service sur les
fakes de `repositorytest`.

- [`controller_test.go`](controller_test.go) contient les aides partagées par tous les contrôleurs, à
  réutiliser telles quelles : `mount(t, prefix, routes)` monte des routes comme `app/` le fait, avec une
  authentification de test qui lit l'en-tête `X-User` ; `client{t, srv.URL, "alice"}.do(method, path,
  body)` envoie une requête ; `decode[T](t, body)` décode la réponse.
- [`project_controller_test.go`](project_controller_test.go) contient ce qui est propre à project, tout
  préfixé : `newProjectServer`, `projectJSON`, `TestProjectCRUD` (le cycle complet) et
  `TestProjectErrors` (par table, le statut et le `code` de chaque réponse `application/problem+json`).
  Copié pour une nouvelle entité, le fichier se renomme en entier, sans conflit de noms.

```go
package controller_test

func newInvoiceServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := service.NewInvoiceService(repositorytest.NewInvoiceRepository(), repositorytest.Tx{}, &repositorytest.AuditRecorder{})
	return mount(t, "/v1/invoices", controller.NewInvoiceController(svc).Routes())
}

func TestInvoiceCRUD(t *testing.T) {
	alice := client{t, newInvoiceServer(t).URL, "alice"}
	resp, body := alice.do(http.MethodPost, "/v1/invoices", `{"amount_cents":1200}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	// …
}
```

```bash
go test ./internal/controller/...
go test ./internal/app/ -run TestRoutesMatchOpenAPI
```
