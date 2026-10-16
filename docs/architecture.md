# Architecture

## Vue d'ensemble

Le code est organisé **par fonctionnalité** (un module métier = un dossier sous `internal/`), avec une
séparation stricte, *à l'intérieur* de chaque module, entre le cœur métier et ses adaptateurs.
Les briques techniques communes vivent dans `internal/platform`, et `internal/app` est le seul endroit
qui connaît tout le monde.

```
                     cmd/api, cmd/migrate, cmd/<job>
                                  │
                                  ▼
                           internal/app            racine de composition (câblage, routes, serveurs)
                     ┌────────────┼──────────────────────┐
                     ▼            ▼                      ▼
        internal/<module>/httpapi │         internal/<module>/postgres      adaptateurs
                     │            ▼                      │
                     └──────► internal/<module> ◄────────┘                  cœur : domaine + cas d'usage
                                  │
                                  ▼
          internal/platform/{apperr, actor, audit, cache, logging}           noyau partagé stable

      internal/platform/{httpx, middleware, database, auth, …}  ◄── utilisés par app et les adaptateurs
```

### Règles de dépendances

Elles sont vérifiées automatiquement par [`internal/archtest`](../internal/archtest/arch_test.go) :

| Règle | Pourquoi |
|---|---|
| Le cœur d'un module n'importe ni `net/http`, ni `database/sql`, ni pgx, Redis, chi, ni `platform/{database,httpx,middleware}` | Le métier se teste sans base ni serveur et survit à un changement de transport ou de stockage |
| Le cœur n'importe pas ses adaptateurs ni les kits de test | Les dépendances pointent vers le métier, jamais l'inverse |
| Un adaptateur n'importe pas un autre adaptateur | `httpapi` ne fait pas de SQL, `postgres` ne connaît pas HTTP |
| Un module n'utilise un autre module que via son cœur | Les frontières entre modules restent explicites |
| `platform` n'importe aucun module métier | Le socle technique reste réutilisable d'un projet à l'autre |
| Seuls les `cmd` importent `internal/app` ; personne n'importe un `cmd` | Une seule racine de composition |

## Anatomie d'un module

Exemple : [`internal/project`](../internal/project).

| Fichier | Rôle |
|---|---|
| `project.go` | Entité, invariants (`New`, `Apply`), erreurs sentinelles (`ErrNotFound`, `ErrNameTaken`…) |
| `repository.go` | Port de persistance (`Repository`) et ses types de requête |
| `service.go` | Cas d'usage. Dépendances injectées par le constructeur, sous forme d'interfaces (`Repository`, `Transactor`, `audit.Recorder`) |
| `httpapi/` | DTO JSON (contrat public), handlers, `Routes()` |
| `postgres/` | Implémentation SQL du port, traduction des erreurs du driver en erreurs métier |
| `projecttest/` | Faux repository en mémoire et **suite de contrat** exécutée contre les deux implémentations |

Le handler déclare l'interface dont il a besoin (`httpapi.Service`) : c'est le consommateur qui définit
l'interface, selon l'idiome Go.

## Cycle de vie d'une requête

L'ordre des middlewares est défini à un seul endroit, [`internal/app/router.go`](../internal/app/router.go) :

```
RequestID → ClientIP → Logger → Metrics → Recoverer → SecurityHeaders → GetHead
  ├─ /livez, /readyz                       sondes : ni rate limit ni auth
  └─ CORS → RateLimit → BodyLimit → Timeout → Authenticate
       ├─ /openapi.yaml, /docs             Basic Auth si DOCS_USER est défini
       └─ /v1/...                          RequireAuth (ou RequireRole) par groupe
```

- **RequestID** vient en premier : chaque ligne de log et chaque réponse d'erreur porte l'identifiant.
  Un `X-Request-ID` entrant n'est réutilisé que s'il est court et sûr (pas d'injection dans les logs).
- **Logger** englobe **Recoverer** : une panique est journalisée comme un 500 avec sa stack.
  Seul le chemin est logué, jamais la query string (elle peut contenir des secrets).
- **ClientIP** ne fait confiance à `X-Forwarded-For` que si le pair direct est un proxy déclaré
  (`RATE_LIMIT_TRUSTED_PROXIES`), et lit la chaîne de droite à gauche : impossible d'usurper son IP
  pour contourner le rate limiting.
- **Timeout** borne le contexte de la requête ; pgx et les clients HTTP sortants le respectent.
- **Authenticate** laisse passer les requêtes anonymes mais rejette tout jeton présent et invalide.
  Le contrôle d'accès est explicite, par groupe de routes (`RequireAuth`, `RequireRole`).

## Gestion des erreurs

```
domaine/service  ──► *apperr.Error{Kind, Code, Message, Fields, Err}
                         │  (remonte via return, enrichi par fmt.Errorf("…: %w", err))
handler (httpx.HandlerFunc) ──► return err
                         │
httpx.Handle ──► httpx.WriteError ──► Kind → statut HTTP ──► problem+json (RFC 9457)
```

- Les handlers ont la signature `func(w, r) error` : pas de `panic` pour signaler une erreur.
- Le **Kind** (`NotFound`, `Conflict`, `Invalid`…) décide du statut HTTP, dans une seule fonction
  (`httpx.StatusFor`). Le **Code** (`project.name_taken`) est stable et exploitable par les clients.
- Une erreur qui n'est pas un `*apperr.Error` devient un 500 opaque : son message (qui peut contenir des
  détails SQL, des hôtes, des identifiants) est journalisé mais jamais renvoyé.
- `context.DeadlineExceeded` et `context.Canceled` deviennent des 503.
- Un identifiant mal formé ou une ressource d'un autre utilisateur donnent **404** (pas 403) :
  on ne révèle pas l'existence des ressources d'autrui.

## Transactions

`database.DB.WithinTx(ctx, fn)` place la transaction dans le contexte ; les repositories appellent
`db.Q(ctx)` qui renvoie la transaction en cours ou le pool. Un service peut ainsi rendre plusieurs appels
atomiques sans connaître pgx : il dépend seulement de l'interface `Transactor`.

L'audit (`audit.Recorder`) écrit via le même contexte : l'entrée d'audit est validée ou annulée
**avec** la modification qu'elle décrit. Une erreur d'audit fait donc échouer l'opération (choix délibéré :
pas d'action sensible sans trace).

## Configuration

Variables d'environnement uniquement ([`config.go`](../internal/platform/config/config.go)), avec type,
valeur par défaut et validation au démarrage. Garde-fous de production : `AUTH_MODE=dev` interdit hors
`dev`/`test`, CORS `*` interdit, documentation protégée par mot de passe, migrations automatiques
interdites, JWKS en HTTPS. Les binaires de maintenance (`migrate`, `retention`) ne lisent que
`DATABASE_*`.

## Observabilité

| Besoin | Mise en œuvre |
|---|---|
| Logs | `slog` JSON ; logger par requête dans le contexte (`logging.FromContext`), avec `request_id` et `user_id` |
| Métriques | Prometheus sur le port admin : `http_requests_total`, `http_request_duration_seconds` (labels = motif de route chi, cardinalité faible), runtime Go |
| Profilage | `/debug/pprof/*` sur le port admin uniquement |
| Santé | `/livez` (processus vivant), `/readyz` (PostgreSQL, Redis ; 503 pendant le drain) |
| Arrêt | SIGTERM → `/readyz` passe à `draining` → attente `HTTP_SHUTDOWN_DELAY` → `Shutdown` avec `HTTP_SHUTDOWN_TIMEOUT` |

Le port admin (`ADMIN_ADDR`, `127.0.0.1:9090` par défaut) n'est jamais exposé publiquement.

## Sécurité (résumé)

- Timeouts serveur systématiques (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`).
- Corps de requête limité (`HTTP_MAX_BODY_BYTES`, 413), champs JSON inconnus refusés.
- JWT : algorithmes asymétriques uniquement (pas de `none` ni de confusion HMAC), `exp` obligatoire,
  `iss` et `aud` vérifiés, clés JWKS rafraîchies automatiquement.
- Jetons uniquement dans l'en-tête `Authorization` (jamais en query string).
- En-têtes : `nosniff`, `X-Frame-Options: DENY`, CSP restrictive, `Cache-Control: no-store`, HSTS optionnel.
- Rate limiting par IP en mémoire, avec plafond de clients et seau partagé en cas de saturation.
- SQL paramétré partout ; jokers `LIKE` échappés ; images Docker distroless non-root.

## Stratégie de test

| Niveau | Où | Dépendances |
|---|---|---|
| Unitaire domaine/service | `internal/<module>/*_test.go` | Faux en mémoire (`projecttest`), aucun mock généré |
| Handler HTTP | `internal/<module>/httpapi/*_test.go` | Vrai service + faux repository, `httptest` |
| Contrat de repository | `projecttest.RepositoryContract` | Exécuté contre le faux **et** PostgreSQL |
| Intégration SQL | `internal/<module>/postgres`, `platform/database`, `platform/audit` | PostgreSQL réel, tag `integration` |
| Bout en bout | `internal/app/app_test.go` | Routeur complet + PostgreSQL |
| Contrat d'API | `internal/app/router_test.go` | Routes chi ↔ `api/openapi.yaml` |
| Architecture | `internal/archtest` | Analyse des imports |
