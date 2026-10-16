# `internal/` — carte du code

Un dossier par rôle classique. Chaque dossier a son README : rôle, ce qui y va, règles de dépendances,
exemple de référence et squelette pour une nouvelle fonctionnalité. L'exemple qui traverse toutes les
couches est l'entité **project** (`/v1/projects`) : copiez ses fichiers `project_*` pour démarrer.

## Rôle classique → dossier

| Rôle classique | Dossier | Exemple de référence |
|---|---|---|
| Contrôleurs (handlers HTTP + routes) | [`controller/`](controller/) | `project_controller.go` |
| DTO (contrat JSON des requêtes/réponses) | [`dto/`](dto/) | `project_dto.go` |
| Services (logique métier, cas d'usage) | [`service/`](service/) | `project_service.go` |
| Repositories (SQL PostgreSQL) | [`repository/`](repository/) | `project_repository.go`, `audit_repository.go` |
| Fakes en mémoire et contrats de test | [`repository/repositorytest/`](repository/repositorytest/) | `memory.go`, `contract.go`, `audit.go` |
| Modèles / entités, erreurs métier | [`model/`](model/) | `project.go` |
| Interfaces (ports) | [`model/`](model/) | `ProjectRepository`, `AuditRecorder`, `Transactor` |
| Middlewares HTTP | [`middleware/`](middleware/) | `auth.go`, `ratelimit.go` |
| Utilitaires sans état | [`utils/`](utils/) | `actor`, `apperr`, `httpx`, `logging`, `requestid` |
| Infrastructure (connexions au monde extérieur) | [`infrastructure/`](infrastructure/) | `auth`, `cache`, `database`, `health`, `metrics`, `server` |
| Configuration | [`config/`](config/) | `config.go` |
| Câblage (racine de composition) | [`app/`](app/) | `router.go` (`projectModule`), `app.go` |
| Règles d'architecture | [`archtest/`](archtest/) | `arch_test.go` |

Exception à « les interfaces vont dans `model/` » : une interface consommée par `controller/` ou
`middleware/` est déclarée chez son consommateur, avec les seules méthodes utilisées. C'est le cas de
`controller.ProjectService` ; un middleware de quota déclarerait de même sa petite interface de
compteur, puisque `middleware/` n'importe pas `model/`.

## Dépendances

Une flèche se lit « importe ».

```
                     cmd/*     seul cmd/* importe app/
                       │
                       ▼
                     app/      racine de composition : importe tout
                       │
      ┌────────────────┼───────────────────────┐
      ▼                ▼                       ▼
 middleware/      controller/             repository/
      │            │   │   │               │       │
      │            │   │   └──► dto/ ──┐   │       │
      │            │   ▼               ▼   │       │
      │            │  service/ ──► model/ ◄┘       │
      │            │                 ▲             │
      │            ├─────────────────┘             │
      ▼            ▼                               ▼
 infrastructure/auth                    infrastructure/database
```

Hors du schéma :

- `utils/*` est importable par tous les dossiers sauf `config/` (qui n'importe rien d'interne), avec
  deux restrictions : `model/` n'importe que `utils/apperr`, et `service/` jamais `utils/httpx` ;
- `infrastructure/*` n'importe que `infrastructure/*`, `utils/*` et les paquets du module hors
  `internal/` (`migrations/`) ;
- ces paquets hors `internal/` et `cmd/` (`migrations/`, `api/`) n'importent rien d'`internal/` : sinon,
  `infrastructure/*` pourrait atteindre `model/` ou `repository/` à travers eux.

## Règles

Elles sont vérifiées par [`archtest/arch_test.go`](archtest/arch_test.go) (`TestDependencyRules`),
exécuté par `make test` : un import interdit fait échouer la CI. Seuls les imports des fichiers non-test
sont contrôlés. La bibliothèque standard et les bibliothèques externes sont libres, sauf la dernière
colonne.

| Dossier | Peut importer dans `internal/` | Interdit en plus |
|---|---|---|
| `utils/*` | `utils/*` | — |
| `config/` | rien | — |
| `model/` | `utils/apperr` | `net/http`, `database/sql`, pgx, go-redis, chi |
| `service/` | `model/`, `utils/*` sauf `utils/httpx` | `net/http`, `database/sql`, pgx, go-redis, chi |
| `dto/` | `model/`, `utils/*` | — |
| `repository/` (et `repositorytest/`) | `model/`, `infrastructure/database`, `utils/*` | `net/http`, chi |
| `infrastructure/*` | `infrastructure/*`, `utils/*` (+ paquets hors `internal/` : `migrations/`) | — |
| `middleware/` | `utils/*`, `infrastructure/auth` | — |
| `controller/` | `service/`, `dto/`, `model/`, `utils/*`, `infrastructure/auth` | — |
| `app/` | tout | — |
| hors `internal/` et `cmd/` (`migrations/`, `api/`) | rien (seulement d'autres paquets hors `internal/`) | — |

Règles globales :

- seuls les `cmd/*` importent `app/`, et personne n'importe un `cmd/*` ; un `cmd/*` n'a pas d'autre
  règle : une tâche planifiée (`cmd/retention`) construit elle-même les repositories et services qu'elle
  utilise, comme `app/` ;
- aucun paquet de production n'importe un kit de test (`repositorytest`, `dbtest` : tout paquet dont le
  nom finit par `test`) ; les fichiers `*_test.go`, eux, le peuvent ;
- un dossier inconnu sous `internal/` fait échouer le test dès qu'il contient un fichier Go, même de
  test : un nouveau rôle s'ajoute dans `layers` et `rules` (`archtest/arch_test.go`) **et** dans ce
  README.

## Vie d'une requête

```
route → middleware → controller → dto → service → model (interfaces) → repository → database
```

Exemple : `POST /v1/projects` avec `{"name":"Orchard"}`.

1. **Route** — `app/router.go` : `NewRouter` monte `/v1/projects` sur `projectModule(d).Routes()`, dans
   le groupe `/v1` protégé par `RequireAuth`.
2. **Middlewares** — `RequestID → ClientIP → Logger → Metrics → Recoverer → SecurityHeaders → GetHead`,
   puis `CORS → RateLimit → BodyLimit → Timeout → Authenticate`, puis `RequireAuth`. `Authenticate`
   place un `auth.Principal` dans le contexte.
3. **Controller** — `ProjectController.create` lit le propriétaire avec `currentUser(r)`.
4. **DTO** — `httpx.Bind[dto.CreateProjectRequest](r)` décode le JSON (champs inconnus refusés) et
   applique les tags `validate` ; le contrôleur le convertit en `service.CreateProjectInput`.
5. **Service** — `ProjectService.Create` : `model.NewProject` normalise et valide, puis `tx.WithinTx`
   enchaîne `repo.Create` et `audit.Record` dans une seule transaction.
6. **Model → repository** — le service ne connaît que `model.ProjectRepository` et
   `model.AuditRecorder` ; `app/` y a branché `repository.ProjectRepository` et
   `repository.AuditRepository`, qui écrivent avec `r.db.Q(ctx)` (la transaction du contexte).
7. **Database** — `database.DB` (pool pgx) exécute le SQL ; une violation de `projects_owner_name_key`
   est traduite en `model.ErrProjectNameTaken`.
8. **Réponse** — `httpx.JSON(w, http.StatusCreated, dto.ToProjectResponse(p))`, avec l'en-tête
   `Location`. Une erreur remonte telle quelle jusqu'à `httpx.Handle`, qui écrit un
   `application/problem+json` dont le statut dépend du `Kind` (`project.name_taken` → 409) ; une
   erreur sans `*apperr.Error` dans sa chaîne devient un 500 opaque, journalisé (503 sur délai dépassé).

## Ajouter une fonctionnalité (ex. `invoice`)

Pas-à-pas complet, avec le code de chaque fichier : [`docs/adding-a-feature.md`](../docs/adding-a-feature.md).
L'ordre : `model/` → migration et `repository/` → fake et contrat dans `repository/repositorytest/` →
`service/` → `dto/` → `controller/` → `app/router.go` et `api/openapi.yaml` (ensemble, sinon
`TestRoutesMatchOpenAPI` échoue) → tests.

## Tester

```bash
make test                       # tests unitaires + règles d'architecture, sans Docker
go test ./internal/archtest/    # règles de dépendances seules
make test-integration           # + PostgreSQL (testcontainers, ou TEST_DATABASE_URL)
make check                      # ce que lance la CI : lint + intégration -race + govulncheck
```
