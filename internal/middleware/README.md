# `middleware/` — middlewares HTTP transverses

Les middlewares s'appliquent à toutes les routes ou à un groupe de routes : identifiant de requête,
IP client, logs, récupération des paniques, en-têtes de sécurité, CORS, limitation de débit, taille du
corps, timeout, authentification et contrôle d'accès. Carte des dossiers : [`internal/README.md`](../README.md).

## Ce qui va ici

Un `func(http.Handler) http.Handler` sans logique métier, réutilisable par n'importe quelle route : lire
un en-tête, placer une valeur dans le contexte, rejeter tôt une requête.

| Fichier | Fournit |
|---|---|
| `requestid.go` | `RequestID` |
| `clientip.go` | `ClientIP(trustedProxies)`, `ClientIPFromContext` |
| `logger.go` | `Logger(base, quiet...)` |
| `recoverer.go` | `Recoverer` |
| `security.go` | `SecurityHeaders(hsts)` |
| `cors.go` | `CORS(allowedOrigins)` |
| `ratelimit.go` | `NewRateLimiter(ctx, rps, burst, maxClients)`, `(*RateLimiter).Middleware` |
| `limits.go` | `BodyLimit(maxBytes)`, `Timeout(d)` |
| `auth.go` | `Authenticate(verifier)`, `RequireAuth`, `RequireRole(roles...)` |
| `basicauth.go` | `BasicAuth(realm, user, password)` |

## Ce qui n'y va pas

- Une règle métier ou un accès à la base → [`service/`](../service/), [`repository/`](../repository/).
- Un traitement propre à un seul endpoint → [`controller/`](../controller/).
- L'ordre de montage → `NewRouter` dans [`app/router.go`](../app/router.go), seul endroit qui le décide
  (voir [`app/README.md`](../app/README.md)).

## Dépendances

`utils/*` et `infrastructure/auth`, plus la bibliothèque standard et les paquets tiers (chi, `rs/cors`,
`x/time/rate`). Tout le reste de `internal/` est interdit : `model`, `service`, `repository`, `dto`,
`controller`, `app` et les autres paquets d'`infrastructure` (`database`, `cache`…). Vérifié par
`internal/archtest`.

## Conventions

- Un fichier par middleware (ou par famille), nommé d'après lui.
- Sans paramètre : `func X(next http.Handler) http.Handler` (`RequestID`). Avec paramètres : une
  fonction qui renvoie le middleware (`Timeout(d)`, `RequireRole("admin")`). Si la construction peut
  échouer, renvoyer `(middleware, error)` comme `ClientIP`.
- Rejet d'une requête : `httpx.WriteError(w, r, err)` avec une sentinelle `apperr` non exportée
  (`errRateLimited`, `errAuthRequired`). Jamais de `panic`, jamais de corps d'erreur écrit à la main.
  Seule exception : `BasicAuth`, qui protège des pages HTML (`/docs`) et répond en texte.
- Valeur dans le contexte : clé privée typée (`type clientIPKey struct{}`) et accesseur exporté
  (`ClientIPFromContext`).
- Un middleware qui dépend d'un service (un compteur de quota dans Redis, par exemple) déclare ici sa
  petite interface, côté consommateur, avec des types simples : `middleware/` n'importe ni `model/` ni
  `infrastructure/cache`. Le client d'`infrastructure/` la satisfait et `app/` le branche.
- Ne jamais journaliser la query string ni un secret.

## Exemple

Références : [`limits.go`](limits.go) (`Timeout`, le middleware paramétré le plus simple) et
[`auth.go`](auth.go) (rejet par `httpx.WriteError`, valeur placée dans le contexte).
[`example_test.go`](example_test.go), exécuté par `go test` : `Example` écrit un middleware
`requireTenant` et le chaîne après `RequestID` ; `ExampleRequireRole` teste une politique d'accès sans
jeton.

Squelette (`middleware/readonly.go`) :

```go
package middleware

import (
	"net/http"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
)

var errReadOnly = apperr.Unavailable("service.read_only", "the API is read-only during maintenance", nil)

// ReadOnly rejects every write with 503 while enabled reports true.
func ReadOnly(enabled func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if enabled() && r.Method != http.MethodGet && r.Method != http.MethodHead {
				httpx.WriteError(w, r, errReadOnly)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

Puis le monter dans `NewRouter` (`r.Use(mw.ReadOnly(...))`) à la bonne place de la chaîne.

## Tester

```bash
go test ./internal/middleware/...
```

- Tests unitaires dans [`middleware_test.go`](middleware_test.go) : `httptest.NewRecorder()`, puis
  `h.ServeHTTP(rec, req)`, puis décodage du corps en `httpx.Problem` pour vérifier `Code`.
- Authentification : `auth.WithPrincipal` dans le contexte de la requête, ou le `stubVerifier` du
  fichier de test.
- L'enchaînement réel (ordre, routes) est couvert par `TestEndToEnd` dans `internal/app`
  (`make test-integration`).
