# `infrastructure/` — connexions au monde extérieur

Les briques techniques qui ont un cycle de vie (pool, client, serveur) : PostgreSQL, Redis,
vérification des jetons, sondes, métriques, serveur HTTP. Rien de métier. Carte des dossiers :
[`internal/README.md`](../README.md).

## Ce qui va ici

Un sous-dossier par système externe ou brique technique (client d'une API tierce, stockage objet, envoi
d'e-mails, file de messages…), avec ses propres types et interfaces, réutilisable tel quel dans un autre
projet.

| Paquet | Rôle | Points d'entrée |
|---|---|---|
| [`auth/`](auth/auth.go) | Vérification des bearer tokens (JWT/JWKS, mode dev) | `Verifier`, `NewJWTVerifier`, `DevVerifier`, `Principal`, `WithPrincipal`, `FromContext`, `BearerToken` |
| [`cache/`](cache/cache.go) | Cache clé/valeur Redis ou mémoire, lecture cache-aside | `Store`, `OpenRedis`, `NewMemory`, `NewLoader`, `GetJSON`, `SetJSON`, `ErrMiss` |
| [`database/`](database/database.go) | Pool pgx, transactions portées par le contexte, migrations, erreurs PostgreSQL | `Open`, `DB.Q`, `DB.WithinTx`, `NewMigrator`, `IsNoRows`, `UniqueViolation` |
| [`database/dbtest/`](database/dbtest/dbtest.go) | Kit de test : une base PostgreSQL isolée par test | `Main`, `New`, `URL` |
| [`health/`](health/health.go) | Sondes `/livez` et `/readyz`, état de drain | `New`, `Checker.Add`, `Checker.SetDraining` |
| [`metrics/`](metrics/metrics.go) | Prometheus : registre et middleware HTTP | `NewRegistry`, `NewHTTP`, `HTTP.Middleware` |
| [`server/`](server/server.go) | Serveurs HTTP avec timeouts et arrêt gracieux | `New`, `Run`, `Options`, `Shutdown` |

## Ce qui n'y va pas

- Le SQL d'une entité → [`repository/`](../repository/), qui s'appuie sur `infrastructure/database`.
- Une entité, une règle ou une erreur métier → [`model/`](../model/), [`service/`](../service/).
- Un helper sans connexion ni état → [`utils/`](../utils/). Un middleware générique →
  [`middleware/`](../middleware/) (celui de `metrics` reste à côté des métriques qu'il alimente).

## Dépendances

Les autres paquets d'`infrastructure/`, `utils/*` et les paquets du module hors `internal/`
(`migrations`). Jamais `model`, `service`, `repository`, `dto`, `controller`, `middleware`, `app`, ni
`config`. Seuls des tests et des kits de test importent `dbtest`. Vérifié par `internal/archtest`.

Conséquences :

- l'infrastructure ne lit pas la configuration : elle reçoit une struct `Options` (`database.Options`,
  `server.Options`) que [`app/`](../app/) remplit à partir de `config.Config` ;
- pour qu'un service utilise un client, `model/` déclare l'interface avec des types simples (par
  exemple `City(ctx context.Context, lat, lon float64) (string, error)`) ; le client la satisfait sans
  importer `model` (typage structurel de Go) et `app/` le branche ;
- si l'interface a besoin de types du modèle, le client garde ses propres types
  (`weather.Forecast`) et un adaptateur dans `app/` les traduit (`model.Forecast`) ; de même, un cache
  autour d'un repository est un décorateur placé dans `app/` (voir
  [architecture.md](../../docs/architecture.md#cache-et-services-externes)).

## Conventions

- Nom de paquet = nom du système ou de la brique (`cache`, `database`), pas `redisclient`.
- `Open…(ctx, …)` ouvre une connexion et la vérifie (`database.Open`, `cache.OpenRedis`) ; `New…`
  construit sans ouvrir de connexion durable, ou enveloppe un objet existant (`geocoding.New` ci-dessous,
  `database.New`, `cache.NewRedis`). Exception : `auth.NewJWTVerifier` télécharge le JWKS au démarrage.
- Une brique qui tient des connexions expose `Close()` et `Ping(ctx)` : `app/` gère la fermeture et
  branche la sonde `/readyz` (`a.health.Add("redis", rs.Ping)`). Un client HTTP sans état comme
  `geocoding` n'en a pas besoin.
- Erreurs enveloppées avec le nom du paquet : `fmt.Errorf("cache: redis ping: %w", err)`.
- Toujours un `context.Context` et un timeout sur les appels sortants.
- La panne d'une dépendance optionnelle (le cache) ne doit jamais faire tomber le service.

## Exemple

Référence : [`cache/`](cache/cache.go), une interface (`Store`), deux implémentations (`Redis`,
`Memory`) et un test de contrat commun (`TestStoreContract` dans [`cache_test.go`](cache/cache_test.go)).
[`cache/example_test.go`](cache/example_test.go), exécuté par `go test` : `ExampleLoader` montre que le
second appel est servi par le cache ; `Example_decorator` ajoute un cache à un client sans le modifier.

Squelette d'un client externe (`infrastructure/geocoding/geocoding.go`) :

```go
// Package geocoding resolves coordinates to a city name with an external API.
package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Options is filled by internal/app from config.Config.
type Options struct {
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	http    *http.Client
	baseURL string
}

func New(o Options) *Client {
	return &Client{http: &http.Client{Timeout: o.Timeout}, baseURL: o.BaseURL}
}

// City calls GET {BaseURL}/reverse?lat=…&lon=….
func (c *Client) City(ctx context.Context, lat, lon float64) (string, error) {
	url := fmt.Sprintf("%s/reverse?lat=%f&lon=%f", c.baseURL, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("geocoding: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("geocoding: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geocoding: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		City string `json:"city"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("geocoding: decoding response: %w", err)
	}
	return body.City, nil
}
```

## Tester

```bash
go test ./internal/infrastructure/...   # unitaires : miniredis, httptest, aucun Docker
make test-integration                   # + database (tag integration, PostgreSQL via dbtest)
```

- Un client HTTP se teste contre un serveur de test `httptest.NewServer`.
- Un test qui a besoin de PostgreSQL porte `//go:build integration`, déclare
  `func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }` et obtient sa base avec `dbtest.New(t)`.
