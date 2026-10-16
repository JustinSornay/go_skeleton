# go_skeleton

Squelette d'API REST en Go, prêt pour la production, extrait et réorganisé à partir de l'API Bhumist.
Il fournit toute la plomberie transverse : configuration, erreurs, authentification, base de données,
observabilité, sécurité, tests, CI, conteneur. Il contient aussi une fonctionnalité d'exemple
complète (`project`, `/v1/projects`), qui traverse toutes les couches et se copie pour chaque nouvelle
fonctionnalité.

- **Go 1.27**, routeur **chi**, **PostgreSQL 18** via **pgx**, migrations **goose** embarquées dans le binaire
- Code rangé en **dossiers par couche** (`controller`, `service`, `repository`, `model`…), chacun avec
  son README et un exemple réel
- Erreurs typées (`apperr`) rendues en **RFC 9457** (`application/problem+json`), sans `panic`
- Authentification **JWT/OIDC via JWKS** (Firebase, Auth0, Keycloak, Cognito…) ou mode `dev` local
- Logs structurés **slog** corrélés par `X-Request-ID`, métriques **Prometheus**, `pprof`, sondes `/livez` et `/readyz`
- Arrêt gracieux (drain + timeouts), timeouts serveur, limite de taille de corps, rate limiting par IP, en-têtes de sécurité, CORS
- Contrat **OpenAPI 3.1** écrit à la main, servi avec Swagger UI, et **vérifié par un test** contre les routes réelles
- Règles de dépendances entre couches **vérifiées par un test** (`internal/archtest`)
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
internal/              un dossier par couche, chacun avec son README (carte : internal/README.md)
  app/                 racine de composition : câblage, ordre des middlewares, routes, serveurs
  config/              configuration par variables d'environnement + garde-fous de production
  controller/          contrôleurs HTTP : handlers + Routes()
  dto/                 contrat JSON des requêtes et réponses
  service/             logique métier (cas d'usage)
  model/               entités, erreurs métier, interfaces (ports)
  repository/          implémentations PostgreSQL des interfaces de model/ (SQL écrit à la main)
    repositorytest/    fakes en mémoire + contrats communs (kit de test)
  middleware/          request ID, logs, recover, sécurité, CORS, IP client, rate limit, auth…
  infrastructure/      connexions au monde extérieur
    auth/              Verifier (JWT/JWKS, dev), Principal
    cache/             Store (Redis, mémoire), lecture cache-aside avec singleflight
    database/          pool pgx, transactions par contexte, migrations, erreurs PostgreSQL
      dbtest/          bases de test isolées (clonées depuis un template migré)
    health/ metrics/ server/
  utils/               petits utilitaires sans état
    actor/ apperr/ httpx/ logging/ requestid/
  archtest/            test des règles de dépendances
docs/                  architecture, décisions, ajout d'une fonctionnalité, extraction depuis Bhumist
```

## Couches

| Dossier | Rôle | Exemple de référence |
|---|---|---|
| [`internal/controller`](internal/controller/README.md) | Handlers HTTP, `Routes()`, interface du service consommé | `project_controller.go` |
| [`internal/dto`](internal/dto/README.md) | Types JSON des requêtes et réponses, conversions explicites | `project_dto.go` |
| [`internal/service`](internal/service/README.md) | Cas d'usage : transactions, audit, enchaînement des appels | `project_service.go` |
| [`internal/model`](internal/model/README.md) | Entités, invariants, erreurs métier, interfaces (ports) | `project.go` |
| [`internal/repository`](internal/repository/README.md) | SQL PostgreSQL implémentant les interfaces de `model` | `project_repository.go` |
| [`internal/repository/repositorytest`](internal/repository/repositorytest/README.md) | Fakes en mémoire, contrats exécutés sur le fake et sur PostgreSQL | `memory.go`, `contract.go` |
| [`internal/middleware`](internal/middleware/README.md) | Middlewares HTTP transverses | `auth.go`, `ratelimit.go` |
| [`internal/infrastructure`](internal/infrastructure/README.md) | PostgreSQL, Redis, JWKS, sondes, métriques, serveur HTTP | `database/`, `cache/` |
| [`internal/utils`](internal/utils/README.md) | Erreurs, plomberie HTTP, logs, request ID, acteur | `apperr/`, `httpx/` |
| [`internal/config`](internal/config/README.md) | Variables d'environnement, validation au démarrage | `config.go` |
| [`internal/app`](internal/app/README.md) | Racine de composition : construit repository → service → controller | `router.go` (`projectModule`) |

Chaque README donne le rôle du dossier, ce qui y va ou non, les règles de dépendances, les conventions
de nommage, un squelette pour une nouvelle fonctionnalité et la façon de tester. La carte d'ensemble,
le schéma des dépendances et les règles vérifiées par `internal/archtest` sont dans
[`internal/README.md`](internal/README.md).

## Documentation

| Document | Contenu |
|---|---|
| [internal/README.md](internal/README.md) | Carte des dossiers, schéma et règles de dépendances, vie d'une requête |
| [docs/architecture.md](docs/architecture.md) | Couches, règles de dépendances, cycle de vie d'une requête, erreurs, transactions, sécurité, tests |
| [docs/decisions.md](docs/decisions.md) | Choix techniques et leurs raisons (couches, chi, pgx, config env, JWKS, OpenAPI…) |
| [docs/adding-a-feature.md](docs/adding-a-feature.md) | Pas-à-pas pour ajouter une fonctionnalité, couche par couche |
| [docs/bhumist-extraction.md](docs/bhumist-extraction.md) | Ce qui a été repris de Bhumist, ce qui a été amélioré, ce qui a été écarté |
| [CLAUDE.md](CLAUDE.md) | Consignes pour Claude Code sur ce dépôt |

## Configuration

Tout passe par des variables d'environnement, déclarées avec leur type et leur valeur par défaut dans
[`internal/config/config.go`](internal/config/config.go) et documentées dans
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
- `repositorytest.ProjectRepositoryContract` est exécuté contre le fake en mémoire **et** contre
  PostgreSQL : le fake utilisé par les tests unitaires ne peut pas diverger de l'implémentation réelle.
- `internal/app/router_test.go` échoue si une route n'est pas documentée dans `api/openapi.yaml` (ou l'inverse).
- `internal/archtest` échoue si une couche importe ce qu'elle ne doit pas, ou si un paquet est rangé
  dans un dossier inconnu sous `internal/`.

## Partir de ce squelette

1. `make rename MODULE=github.com/acme/mon-api` : renomme le module partout.
2. Ajuster `APP_NAME`, le titre de `api/openapi.yaml` et ce README.
3. Créer les fonctionnalités métier en suivant [docs/adding-a-feature.md](docs/adding-a-feature.md).
4. Quand l'exemple `project` ne sert plus, le supprimer :
   - ses fichiers `project*` dans `model/`, `repository/`, `service/`, `dto/` et `controller/`
     (tests compris ; garder `controller/controller_test.go` et `service/service_test.go`, qui
     contiennent les aides de test partagées) ;
   - ses parties dans `repository/repositorytest/` (garder `Tx`, `AuditRecorder` et la doc du paquet) ;
   - `projectModule` et son `Mount` dans `internal/app/router.go` ;
   - ses routes et schémas dans `api/openapi.yaml` ;
   - le scénario de `TestEndToEnd` (`internal/app/app_test.go`), à adapter à une vraie fonctionnalité ;
   - la migration `00002_projects.sql`, tant qu'elle n'a été appliquée sur aucun environnement partagé.
