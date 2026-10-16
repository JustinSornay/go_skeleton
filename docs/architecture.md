# Architecture

## Vue d'ensemble

Le code est rangé **par couche** : sous `internal/`, un dossier global par rôle classique
(`controller`, `service`, `repository`, `model`…). Chaque dossier a son README, qui donne :

- son rôle ;
- ce qui y va et ce qui n'y va pas ;
- les règles de dépendances qui s'y appliquent ;
- ses conventions de nommage ;
- un exemple réel et un squelette pour une nouvelle fonctionnalité.

L'entité d'exemple **project** (`/v1/projects`) traverse toutes les couches. La carte des dossiers est
dans [`internal/README.md`](../internal/README.md). `internal/app` est le seul paquet qui connaît tous
les autres.

```
                                cmd/*
                                  │      seul cmd/* importe app/
                                  ▼
                                app/     racine de composition : importe tout
         ┌──────────────────┬─────┴──────────────┬──────────────────────┐
         ▼                  ▼                    ▼                      ▼
    middleware/        controller/          repository/          infrastructure/*
         │            │     │     │           │      │           config/
         │            ▼     │     ▼           │      │
         │          dto/    │  service/       │      │
         │            │     │     │           │      │
         │            ▼     ▼     ▼           │      ▼
         │               model/  ◄────────────┘   infrastructure/database
         ▼
    infrastructure/auth   (importé aussi par controller/)

    utils/* (apperr, httpx, actor, logging, requestid) : importable par toutes les couches sauf
    config/ ; model/ se limite à utils/apperr, service/ n'importe pas utils/httpx.
```

`model/` est au centre, et les dépendances pointent vers lui. `repository/` implémente les interfaces
déclarées dans `model/` (inversion de dépendances). Le métier ne connaît donc ni HTTP ni SQL.

## Les couches

| Dossier | Rôle | Exemple de référence |
|---|---|---|
| [`model/`](../internal/model/README.md) | Entités, invariants, erreurs métier, interfaces (ports) | `project.go`, `audit.go`, `transactor.go` |
| [`service/`](../internal/service/README.md) | Cas d'usage : transactions, audit, enchaînement des appels | `project_service.go`, options communes dans `service.go` |
| [`repository/`](../internal/repository/README.md) | Implémentations PostgreSQL des interfaces de `model/`, SQL écrit à la main | `project_repository.go`, `audit_repository.go` |
| [`repository/repositorytest/`](../internal/repository/repositorytest/README.md) | Kit de test : fakes en mémoire, contrats, `Tx`, `AuditRecorder` | `memory.go`, `contract.go`, `audit.go` |
| [`dto/`](../internal/dto/README.md) | Contrat JSON des requêtes et réponses | `project_dto.go` |
| [`controller/`](../internal/controller/README.md) | Handlers HTTP et `Routes()` | `project_controller.go` |
| [`middleware/`](../internal/middleware/README.md) | Middlewares HTTP transverses | `auth.go`, `ratelimit.go` |
| [`infrastructure/`](../internal/infrastructure/README.md) | Connexions au monde extérieur | `auth/`, `cache/`, `database/`, `health/`, `metrics/`, `server/` |
| [`utils/`](../internal/utils/README.md) | Utilitaires sans état | `actor/`, `apperr/`, `httpx/`, `logging/`, `requestid/` |
| [`config/`](../internal/config/README.md) | Configuration par variables d'environnement | `config.go` |
| [`app/`](../internal/app/README.md) | Racine de composition : câblage, middlewares, routes, serveurs | `router.go`, `app.go` |
| [`archtest/`](../internal/archtest/arch_test.go) | Vérification des règles de dépendances | `arch_test.go` |

Conventions communes :

- **Un fichier par entité et par couche** : `model/project.go`, `repository/project_repository.go`,
  `service/project_service.go`, `dto/project_dto.go`, `controller/project_controller.go`.
- **Un préfixe d'entité sur les identifiants**, car les paquets sont partagés par toutes les
  fonctionnalités : `ErrProjectNotFound`, `ProjectRepository`, `NewProjectService`,
  `ToProjectResponse`. Les aides non exportées de `project_repository.go` (`row`, `columns`,
  `mapErr`) font exception : préfixez celles d'une nouvelle entité (`invoiceRow`, `mapInvoiceErr`).
- **Les interfaces vont dans `model/`**, avec une exception : une interface consommée par
  `controller/` ou `middleware/` est déclarée chez son consommateur, selon l'idiome Go. Un contrôleur
  déclare ainsi l'interface du service qu'il consomme (`controller.ProjectService`), et un middleware
  sa petite interface de compteur (il n'importe pas `model/`).

## Règles de dépendances

Elles portent sur les imports des fichiers non-test et sont vérifiées automatiquement par
[`internal/archtest`](../internal/archtest/arch_test.go).

| Dossier | Peut importer dans `internal/` | Interdit en plus | Pourquoi |
|---|---|---|---|
| `utils/*` | `utils/*` | — | Socle réutilisable d'un projet à l'autre |
| `config/` | rien | — | Lisible par n'importe quel binaire sans rien tirer d'autre |
| `model/` | `utils/apperr` | `net/http`, `database/sql`, pgx, go-redis, chi | Le métier se teste sans base ni serveur et survit à un changement de transport ou de stockage |
| `service/` | `model/`, `utils/*` sauf `utils/httpx` | `net/http`, `database/sql`, pgx, go-redis, chi | Mêmes raisons : les cas d'usage ne voient que des interfaces |
| `dto/` | `model/`, `utils/*` | — | Le contrat JSON dépend du modèle, jamais l'inverse |
| `repository/` (et `repositorytest/`) | `model/`, `infrastructure/database`, `utils/*` | `net/http`, chi | La persistance ne connaît pas HTTP |
| `infrastructure/*` | `infrastructure/*`, `utils/*`, paquets du module hors `internal/` (`migrations`) | — | Briques techniques sans métier, réutilisables |
| `middleware/` | `utils/*`, `infrastructure/auth` | — | Transverse : aucune logique métier, aucun accès aux données |
| `controller/` | `service/`, `dto/`, `model/`, `utils/*`, `infrastructure/auth` | — | `controller/` ne fait pas de SQL et ne contourne pas le service |
| `app/` | tout | — | La racine de composition de l'API |
| hors `internal/` et `cmd/` (`migrations/`, `api/`) | rien | — | Sinon `infrastructure/*`, qui les importe, atteindrait `model/` ou `repository/` à travers eux |

Règles globales :

- seuls les `cmd/*` importent `internal/app`, et personne n'importe un `cmd/*` ; une tâche planifiée
  (`cmd/retention`) est la racine de composition de son binaire : elle construit elle-même les
  repositories et services qu'elle utilise ;
- aucun paquet de production n'importe un kit de test (`repositorytest`, `dbtest` : tout paquet dont le
  nom finit par `test`), alors que les fichiers `*_test.go` le peuvent ;
- un dossier inconnu sous `internal/` qui contient des fichiers Go, même uniquement de test, fait
  échouer le test.

Le test analyse les imports avec `go/parser` (`TestDependencyRules`). `TestViolations` vérifie la
matrice elle-même sur des imports représentatifs, pour qu'une erreur dans la table des règles ne
désactive pas une règle en silence. Pour ajouter une couche, mettez à jour `layers` et `rules` dans
`arch_test.go` **et** [`internal/README.md`](../internal/README.md).

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
  Un `X-Request-ID` entrant n'est réutilisé que s'il est court et sûr, pour éviter toute injection dans
  les logs.
- **Logger** englobe **Recoverer** : une panique est journalisée comme un 500 avec sa stack.
  Seul le chemin est journalisé, jamais la query string, qui peut contenir des secrets.
- **ClientIP** ne fait confiance à `X-Forwarded-For` que si le pair direct est un proxy déclaré
  (`RATE_LIMIT_TRUSTED_PROXIES`). Il lit la chaîne de droite à gauche : un client ne peut pas usurper
  son IP pour contourner le rate limiting.
- **Timeout** borne le contexte de la requête ; pgx et les clients HTTP sortants le respectent.
- **Authenticate** laisse passer les requêtes anonymes mais rejette tout jeton présent et invalide.
  Le contrôle d'accès est explicite, par groupe de routes (`RequireAuth`, `RequireRole`).

La requête traverse ensuite les couches. Exemple : `POST /v1/projects`.

```
controller ──► dto ──► service ──► model (interfaces) ──► repository ──► infrastructure/database
```

1. `ProjectController.create` lit l'utilisateur avec `currentUser(r)`, puis décode et valide le corps
   avec `httpx.Bind[dto.CreateProjectRequest]`.
2. `ProjectService.Create` construit l'entité avec `model.NewProject`, qui normalise et valide. Il
   ouvre ensuite une transaction qui enchaîne `repo.Create` et `audit.Record`.
3. `repository.ProjectRepository` exécute le SQL avec `r.db.Q(ctx)` et traduit la violation de
   `projects_owner_name_key` en `model.ErrProjectNameTaken`.
4. Le contrôleur répond avec `httpx.JSON(w, http.StatusCreated, dto.ToProjectResponse(p))` ; une
   erreur remonte jusqu'à `httpx.Handle`.

## Gestion des erreurs

```
model / service / repository ──► *apperr.Error{Kind, Code, Message, Fields, Err}
                         │  (remonte via return, enrichi par fmt.Errorf("…: %w", err))
controller (httpx.HandlerFunc) ──► return err
                         │
httpx.Handle ──► httpx.WriteError ──► Kind → statut HTTP ──► problem+json (RFC 9457)
```

- Les erreurs métier sont des sentinelles `apperr` déclarées dans `model/`
  (`ErrProjectNotFound`, `ErrProjectNameTaken`, `ErrProjectInvalid`) et comparées avec `errors.Is`.
  `repository/` traduit les erreurs du driver en ces sentinelles.
- Les handlers ont la signature `func(w, r) error` : pas de `panic` pour signaler une erreur.
- Le **Kind** (`NotFound`, `Conflict`, `Invalid`…) décide du statut HTTP, dans une seule fonction
  (`httpx.StatusFor`). Le **Code** (`project.name_taken`) est stable et exploitable par les clients.
- Une erreur qui n'est pas un `*apperr.Error` devient un 500 opaque. Son message peut contenir des
  détails SQL, des hôtes ou des identifiants : il est journalisé, mais jamais renvoyé au client.
- `context.DeadlineExceeded` et `context.Canceled` deviennent des 503.
- Un identifiant mal formé ou une ressource d'un autre utilisateur donnent **404** (pas 403) : on ne
  révèle pas l'existence des ressources d'autrui.

## Transactions

`database.DB.WithinTx(ctx, fn)` place la transaction dans le contexte, et les repositories appellent
`r.db.Q(ctx)`, qui renvoie la transaction en cours ou, à défaut, le pool. Un service peut ainsi rendre
plusieurs appels atomiques sans connaître pgx : il dépend seulement de l'interface `model.Transactor`.
Dans `app/router.go`, `d.DB` est passé à la fois aux repositories et, comme `Transactor`, au service.

L'audit passe par `model.AuditRecorder`, implémenté par `repository.AuditRepository`. Il écrit via le
même contexte : l'entrée d'audit est validée ou annulée **avec** la modification qu'elle décrit. Une
erreur d'audit fait donc échouer l'opération, par choix délibéré : pas d'action sensible sans trace.

Dans les tests unitaires, `repositorytest.Tx{}` appelle `fn` directement et n'annule rien. Les
rollbacks se testent en intégration (`TestAuditRecordIsTransactional`, `TestWithinTxRollsBackOnError`).

## Cache et services externes

`service/` et `repository/` n'importent pas `infrastructure/cache` ni un client externe. On procède
ainsi :

1. `model/` déclare l'interface dont le service a besoin, avec des types simples, par exemple
   `City(ctx context.Context, lat, lon float64) (string, error)`.
2. Le client vit dans `infrastructure/<système>/`. Il satisfait cette interface sans importer `model/`,
   grâce au typage structurel de Go.
3. Pour mettre ce client en cache, on l'enveloppe dans un décorateur qui implémente la même interface
   et lit via `cache.Loader`. Le décorateur vit à côté du client, dans `infrastructure/`. Voir
   `Example_decorator` dans
   [`infrastructure/cache/example_test.go`](../internal/infrastructure/cache/example_test.go).
4. `app/` construit le tout et l'injecte dans le service. Le store partagé est `Deps.Cache` : Redis si
   `REDIS_URL` est défini, sinon en mémoire.

L'étape 1 impose des types simples ou standard : `infrastructure/*` n'importe pas `model/`, donc un
client ne peut ni renvoyer ni recevoir un type du modèle. Quand l'interface a besoin de types du modèle
(un client qui renvoie un `model.Forecast`, un cache placé autour d'un `model.ProjectRepository`),
seul `app/` peut importer à la fois `model/` et `infrastructure/*` : l'adaptateur qui traduit les types
du client (`weather.Forecast` → `model.Forecast`), ou le décorateur de cache autour du repository, vit
dans `app/`, dans un fichier nommé d'après lui (`app/forecast_adapter.go`). Il ne contient que de la
traduction de types et de l'appel au cache, jamais de règle métier. Si ces adaptateurs se multiplient,
un dossier dédié serait une nouvelle couche : à décider, puis à déclarer dans `layers` et `rules`
(`internal/archtest`).

`cache.Loader` regroupe les lectures simultanées d'une même clé froide (`singleflight`). Une panne du
cache est journalisée puis contournée : le cache ne doit jamais faire tomber le service.

## Configuration

Tout passe par des variables d'environnement ([`config.go`](../internal/config/config.go)), avec type,
valeur par défaut et validation au démarrage. Garde-fous de production :

- `AUTH_MODE=dev` est interdit hors `dev`/`test` ;
- CORS `*` est interdit ;
- la documentation est protégée par mot de passe ;
- les migrations automatiques sont interdites ;
- le JWKS doit être servi en HTTPS.

`infrastructure/*` ne lit jamais la configuration : `app/` traduit `config.Config` en `Options`
(`database.Options`, `server.Options`). Les binaires de maintenance (`migrate`, `retention`) ne lisent
que `DATABASE_*` (`config.LoadDatabase`).

## Observabilité

| Besoin | Mise en œuvre |
|---|---|
| Logs | `slog` JSON ; logger par requête dans le contexte (`logging.FromContext`, `internal/utils/logging`), avec `request_id` et `user_id` |
| Métriques | Prometheus sur le port admin (`internal/infrastructure/metrics`) : `http_requests_total`, `http_request_duration_seconds` (labels = motif de route chi, cardinalité faible), runtime Go |
| Profilage | `/debug/pprof/*` sur le port admin uniquement |
| Santé | `/livez` (processus vivant), `/readyz` (PostgreSQL, Redis ; 503 pendant le drain), dans `internal/infrastructure/health` |
| Arrêt | SIGTERM → `/readyz` passe à `draining` → attente `HTTP_SHUTDOWN_DELAY` → `Shutdown` avec `HTTP_SHUTDOWN_TIMEOUT` (`internal/infrastructure/server`) |

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
| Cas d'usage | `internal/service/*_test.go` | Fakes de `repositorytest` (`NewProjectRepository()`, `Tx{}`, `AuditRecorder`), aucun mock généré |
| HTTP | `internal/controller/*_test.go` | Vrai service sur les fakes, `httptest` |
| Contrat de repository | `repositorytest.ProjectRepositoryContract` | Exécuté contre le fake (`repositorytest/memory_test.go`) **et** PostgreSQL (`repository/project_repository_test.go`) |
| Intégration SQL | `internal/repository`, `internal/infrastructure/database` | PostgreSQL réel, tag `integration`, `dbtest.New(t)` |
| Infrastructure | `internal/infrastructure/{auth,cache,health,metrics,server}` | miniredis, `httptest`, sans Docker |
| Exemples exécutables | les `example_test.go` (`config`, `middleware`, `infrastructure/cache`, `utils/apperr`, `utils/httpx`) | Sortie vérifiée par `go test` |
| Bout en bout | `internal/app/app_test.go` (`TestEndToEnd`) | Routeur complet + PostgreSQL |
| Contrat d'API | `internal/app/router_test.go` (`TestRoutesMatchOpenAPI`) | Routes chi ↔ `api/openapi.yaml` |
| Architecture | `internal/archtest` | Analyse des imports |
