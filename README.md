# go_skeleton

Squelette d'API REST en Go, prêt pour la production, extrait et réorganisé à partir de l'API Bhumist.
Il fournit toute la plomberie transverse (configuration, erreurs, authentification, base de données,
observabilité, sécurité, tests, CI, conteneur) et un module d'exemple complet (`project`) à copier
pour chaque nouvelle fonctionnalité.

- **Go 1.27**, routeur **chi**, **PostgreSQL 18** via **pgx**, migrations **goose** embarquées dans le binaire
- Erreurs typées (`apperr`) rendues en **RFC 9457** (`application/problem+json`), sans `panic`
- Authentification **JWT/OIDC via JWKS** (Firebase, Auth0, Keycloak, Cognito…) ou mode `dev` local
- Logs structurés **slog** corrélés par `X-Request-ID`, métriques **Prometheus**, `pprof`, sondes `/livez` et `/readyz`
- Arrêt gracieux (drain + timeouts), timeouts serveur, limite de taille de corps, rate limiting par IP, en-têtes de sécurité, CORS
- Contrat **OpenAPI 3.1** écrit à la main, servi avec Swagger UI, et **vérifié par un test** contre les routes réelles
- Règles d'architecture **vérifiées par un test** (imports interdits entre couches)
- Tests unitaires sans base, tests d'intégration sur un vrai PostgreSQL (testcontainers ou `TEST_DATABASE_URL`)
- `Makefile`, `compose.yaml`, `Dockerfile` distroless non-root, CI GitHub Actions (lint, tests `-race`, govulncheck, build image)

## Démarrage rapide

Prérequis : Go 1.27+, Docker.

```bash
cp .env.example .env     # configuration locale (AUTH_MODE=dev)
make up                  # PostgreSQL dans Docker
make migrate             # applique les migrations
make run                 # API sur :8080, admin (métriques, pprof) sur 127.0.0.1:9090
```

En mode `dev`, le jeton Bearer **est** l'identifiant utilisateur (`alice`, ou `alice:admin` pour ajouter des rôles) :

```bash
curl -s -X POST localhost:8080/v1/projects \
  -H 'Authorization: Bearer alice' -d '{"name":"Ferme du Nord"}'
curl -s localhost:8080/v1/projects -H 'Authorization: Bearer alice'
curl -s localhost:8080/v1/projects          # 401 problem+json
open http://localhost:8080/docs             # Swagger UI
```

Exemple d'erreur :

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "request validation failed",
  "instance": "/v1/projects",
  "code": "request.invalid",
  "request_id": "01a1451b-2389-7d54-b2c5-f3c0fa74632a",
  "errors": [{ "field": "name", "rule": "required", "message": "is required" }]
}
```

`make help` liste toutes les commandes (base de données, tests, lint, build, image…).

## Structure

```
cmd/
  api/                 binaire HTTP : main() mince, run() testable, -healthcheck pour Docker
  migrate/             up | down | status | version | create <name> (migrations embarquées)
  retention/           exemple de tâche planifiée (purge de l'audit, --dry-run, lots)
api/                   openapi.yaml (contrat) + Swagger UI
migrations/            fichiers SQL goose, embarqués dans les binaires
internal/
  app/                 racine de composition : câblage, ordre des middlewares, serveurs
  platform/            briques techniques réutilisables, sans métier
    apperr/            modèle d'erreur (Kind, Code, champs) indépendant du transport
    httpx/             handlers qui retournent error, RFC 9457, bind/validation JSON et query, pagination
    middleware/        request ID, logs, recover, sécurité, CORS, IP client, rate limit, auth…
    auth/              Verifier (JWT/JWKS, dev), Principal
    config/            configuration par variables d'environnement + garde-fous de production
    database/          pool pgx, transactions par contexte, migrations, erreurs PostgreSQL
      dbtest/          bases de test isolées (clonées depuis un template migré)
    cache/             Store (Redis, mémoire), lecture cache-aside avec singleflight
    audit/ actor/      journal d'audit transactionnel, « qui agit » porté par le contexte
    health/ metrics/ logging/ server/ requestid/
  project/             module d'exemple (domaine + cas d'usage, sans HTTP ni SQL)
    httpapi/           adaptateur HTTP : DTO, handlers, routes
    postgres/          adaptateur PostgreSQL : SQL écrit à la main
    projecttest/       faux repository en mémoire + suite de contrat commune
  archtest/            test des règles de dépendances
docs/                  architecture, décisions, ajout d'un module, extraction depuis Bhumist
```

## Documentation

| Document | Contenu |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Couches, règles de dépendances, cycle de vie d'une requête, erreurs, transactions, sécurité, tests |
| [docs/decisions.md](docs/decisions.md) | Choix techniques et leurs raisons (chi, pgx, config env, JWKS, OpenAPI…) |
| [docs/adding-a-module.md](docs/adding-a-module.md) | Pas-à-pas pour ajouter une fonctionnalité |
| [docs/bhumist-extraction.md](docs/bhumist-extraction.md) | Ce qui a été repris de Bhumist, ce qui a été amélioré, ce qui a été écarté |
| [CLAUDE.md](CLAUDE.md) | Consignes pour Claude Code sur ce dépôt |

## Configuration

Tout passe par des variables d'environnement, déclarées avec leur type et leur valeur par défaut dans
[`internal/platform/config/config.go`](internal/platform/config/config.go) et documentées dans
[`.env.example`](.env.example). Le chargement échoue au démarrage si une valeur est invalide ou dangereuse
en production (`AUTH_MODE=dev`, CORS `*`, docs sans mot de passe, migrations automatiques…).

Authentification Firebase (exemple) :

```bash
AUTH_MODE=jwt
AUTH_JWKS_URL=https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com
AUTH_ISSUER=https://securetoken.google.com/<project-id>
AUTH_AUDIENCE=<project-id>
```

## Tests

```bash
make test               # unitaires : aucun Docker nécessaire
make test-integration   # + PostgreSQL : testcontainers, ou TEST_DATABASE_URL si défini
make lint               # golangci-lint v2
make check              # tout ce que la CI exécute
```

- Les tests d'intégration portent le build tag `integration`. Chaque test obtient sa propre base,
  clonée en quelques millisecondes depuis un template déjà migré : isolation complète, tests parallélisables.
- `projecttest.RepositoryContract` est exécuté contre le faux en mémoire **et** contre PostgreSQL :
  le faux utilisé par les tests unitaires ne peut pas diverger de l'implémentation réelle.
- `internal/app/router_test.go` échoue si une route n'est pas documentée dans `api/openapi.yaml` (ou l'inverse).
- `internal/archtest` échoue si une couche importe ce qu'elle ne doit pas.

## Partir de ce squelette

1. `make rename MODULE=github.com/acme/mon-api` : renomme le module partout.
2. Ajuster `APP_NAME`, le titre de `api/openapi.yaml` et ce README.
3. Créer les modules métier en suivant [docs/adding-a-module.md](docs/adding-a-module.md) ;
   supprimer ensuite `internal/project`, ses routes et sa migration `00002_projects.sql` (tant qu'elle
   n'a été appliquée sur aucun environnement partagé) quand ils ne servent plus d'exemple.
