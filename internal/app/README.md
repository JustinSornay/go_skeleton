# `app/` — racine de composition

Le seul paquet qui connaît tous les autres. Il ouvre l'infrastructure, construit repositories →
services → controllers par simple injection par constructeur (ni conteneur, ni variable globale), monte
les middlewares et les routes, puis lance les serveurs. Carte des dossiers :
[`internal/README.md`](../README.md).

## Ce qui va ici

| Fichier | Contenu |
|---|---|
| [`app.go`](app.go) | `New(ctx, cfg, log)` ouvre PostgreSQL, applique les migrations si `DATABASE_AUTO_MIGRATE`, choisit le cache (Redis si `REDIS_URL`, sinon mémoire), le `auth.Verifier` et le rate limiter, puis appelle `NewRouter`. `Run(ctx)` sert l'API et le port admin (`/metrics`, `/debug/pprof`) avec arrêt gracieux ; `Close()` libère tout dans l'ordre inverse. |
| [`router.go`](router.go) | `Deps`, `NewRouter(Deps)` (ordre des middlewares, routes, politique d'accès) et une fonction de construction par fonctionnalité (`projectModule`). |
| [`router_test.go`](router_test.go) | `TestRoutesMatchOpenAPI` : routes chi ↔ `api/openapi.yaml`. |
| [`app_test.go`](app_test.go) | `TestEndToEnd` : routeur complet + PostgreSQL réel (tag `integration`). |

Autrement dit : le câblage, l'ordre des middlewares, la politique d'accès (`RequireAuth`,
`RequireRole`) par groupe de routes, la traduction de `config.Config` en `Options` de l'infrastructure
et le cycle de vie (ouverture, fermeture, drain).

S'y ajoutent, au besoin, les adaptateurs entre un client d'`infrastructure/` et une interface de
`model/` qui utilise des types du modèle (`forecast_adapter.go`, ou un décorateur de cache autour d'un
repository) : `app/` est le seul paquet qui peut importer les deux. Un adaptateur ne fait que traduire
des types et appeler le cache, sans règle métier (voir
[architecture.md](../../docs/architecture.md#cache-et-services-externes)).

## Ce qui n'y va pas

- Une règle métier → [`service/`](../service/), [`model/`](../model/).
- Un handler, un DTO → [`controller/`](../controller/), [`dto/`](../dto/).
- Du SQL → [`repository/`](../repository/).

## Dépendances

`app/` peut importer tout `internal/`. Seuls les `cmd/*` importent `app`, et personne n'importe un
`cmd`. Comme partout, le code non-test n'importe aucun kit de test (`dbtest`, `repositorytest`).
Vérifié par `internal/archtest`.

## Brancher une nouvelle fonctionnalité

1. Suivre les étapes 1 à 6 de [`docs/adding-a-feature.md`](../../docs/adding-a-feature.md) : `model/`,
   migration et `repository/`, fake et contrat, `service/`, `dto/`, `controller/`.
2. Dans `router.go`, ajouter **une fonction de construction par fonctionnalité**, sur le modèle de
   `projectModule`. `d.DB` sert à la fois de connexion pour les repositories et de `model.Transactor`
   pour le service :

   ```go
   func invoiceModule(d Deps) *controller.InvoiceController {
   	repo := repository.NewInvoiceRepository(d.DB)
   	svc := service.NewInvoiceService(repo, d.DB, repository.NewAuditRepository(d.DB))
   	return controller.NewInvoiceController(svc)
   }
   ```

3. Monter ses routes dans `NewRouter`, dans le groupe qui porte la bonne politique d'accès :

   ```go
   r.Route("/v1", func(r chi.Router) {
   	r.Group(func(r chi.Router) {
   		r.Use(mw.RequireAuth)
   		r.Mount("/projects", projectModule(d).Routes())
   		r.Mount("/invoices", invoiceModule(d).Routes())
   	})
   })
   ```

4. Documenter les routes dans `api/openapi.yaml` dans le même changement que le `Mount` :
   `TestRoutesMatchOpenAPI` échoue tant que les deux ne correspondent pas.
5. Nouvelle dépendance d'infrastructure (client externe…) : l'ouvrir dans `New`, enregistrer sa
   fermeture dans `a.closers` et, si elle est indispensable, sa sonde avec `a.health.Add`, puis
   l'ajouter à `Deps`.

## Ordre des middlewares

Défini dans `NewRouter`, du plus externe au plus interne (avec `r.Use(a, b)`, `a` s'exécute en premier) :

```
RequestID → ClientIP → Logger → Metrics → Recoverer → SecurityHeaders → GetHead
  ├─ /livez, /readyz           sondes : ni CORS, ni rate limit, ni auth
  └─ CORS → RateLimit → BodyLimit → Timeout → Authenticate
       ├─ /openapi.yaml, /docs Basic Auth si DOCS_USER est défini
       └─ /v1/...              RequireAuth (ou RequireRole) par groupe
```

- `RequestID` en premier : chaque ligne de log et chaque réponse d'erreur porte l'identifiant.
- `Logger` englobe `Recoverer` : une panique est journalisée comme un 500.
- `Authenticate` laisse passer les requêtes anonymes ; c'est `RequireAuth` ou `RequireRole`, posé sur
  un groupe, qui protège les routes.
- `Metrics` et `RateLimit` sont optionnels (`Deps.Metrics`, `Deps.Limiter` à `nil`).

## `Deps` dans les tests

Les tests construisent le routeur sans `New`, en remplissant `Deps` directement (d'après
`TestEndToEnd`) :

```go
router, err := app.NewRouter(app.Deps{
	Config:   testConfig(t),        // config.LoadFrom(map[string]string{…})
	Logger:   logging.Discard(),
	DB:       dbtest.New(t),        // base PostgreSQL isolée (tag integration)
	Cache:    cache.NewMemory(),
	Verifier: auth.DevVerifier{},   // « Authorization: Bearer alice »
	Health:   health.New(time.Second),
})
```

Un test qui ne sert aucune requête peut omettre `DB` et `Cache` (cf. `TestRoutesMatchOpenAPI`).

## Tester

```bash
go test ./internal/app/...   # TestRoutesMatchOpenAPI, sans Docker
make test-integration        # + TestEndToEnd (PostgreSQL via testcontainers ou TEST_DATABASE_URL)
```
