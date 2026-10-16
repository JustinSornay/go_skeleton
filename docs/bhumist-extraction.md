# Extraction depuis l'API Bhumist

Ce squelette a été obtenu en lisant l'API Bhumist (`Bhumist/api`, Go 1.26, ~450 fichiers Go) et en
séparant ce qui relève de la plomberie réutilisable de ce qui relève du métier agronomique. Ce document
liste ce qui a été repris, ce qui a été corrigé ou réorganisé, et ce qui a été volontairement laissé de
côté. Les références `fichier:ligne` pointent vers le dépôt Bhumist.

## 1. Repris (principe conservé, mise en œuvre parfois retravaillée)

| Pratique Bhumist | Où dans Bhumist | Où dans le squelette |
|---|---|---|
| Clean architecture : domaine (entités + interfaces de repository), cas d'usage, handlers, infrastructure | `internal/domain`, `internal/usecase`, `internal/handler`, `internal/infrastructure` | Conservée en dossiers par couche : `internal/model`, `internal/service`, `internal/controller` (+ `internal/dto`), `internal/repository`, `internal/infrastructure` |
| Erreurs de domaine sentinelles comparées avec `errors.Is` | `internal/domain/*/entity.go` | Sentinelles `apperr` avec `Kind` et `Code` (`internal/model/project.go`) |
| Injection par constructeur, pas de conteneur ni de service locator | `internal/app/wire*.go` | `internal/app/router.go` (une fonction de construction par fonctionnalité : `projectModule`) |
| Racine de composition séparée de `main`, partagée avec les tests | `app.BuildRouter(app.Deps{…})` | `app.NewRouter(app.Deps{…})`, `app.New` |
| Réponses d'erreur `application/problem+json` | `internal/middleware/error_handler.go` | `internal/utils/httpx/problem.go` (RFC 9457 + `code`, `request_id`, `errors`) |
| Décodage des query params par tags `query` + `go-playground/validator` | `internal/handler/handler_helpers.go` | `httpx.BindQuery` (+ structs embarquées, slices, entiers non signés, erreurs par champ) |
| Décodage JSON générique `decodeAndValidate[T]` | `internal/handler/decode.go` | `httpx.Bind[T]` (+ champs inconnus refusés, 413, valeur unique) |
| DTO distincts des types de domaine, conversions explicites | `internal/handler/dto` | `internal/dto/project_dto.go` |
| Ne jamais journaliser la query string (secrets) | `internal/middleware/logger.go:66-71` (audit P2 #20) | `middleware.Logger` |
| IP client : `X-Forwarded-For` lu de droite à gauche, proxies de confiance en CIDR | `internal/middleware/ratelimit.go:192` | `middleware.ClientIP` |
| Plafond du nombre de clients suivis par le rate limiter | `ratelimit.go` (`max_visitors`) | `middleware.RateLimiter` (`RATE_LIMIT_MAX_CLIENTS`) |
| `RequireAuth`, auth optionnelle à la racine, `RequireRole` | `internal/middleware/auth.go`, `role.go` | `middleware.Authenticate`, `RequireAuth`, `RequireRole` |
| Comparaison en temps constant pour la Basic Auth de la doc | `internal/middleware/basic_auth.go` | `middleware.BasicAuth` (hachage préalable : la longueur ne fuit plus) |
| Garde-fous de production au chargement de la config | `config/config.go:255-273` | `config.Validate()` |
| Secrets uniquement par variables d'environnement ; `Makefile` qui charge `.env` | `config/`, `Makefile:4-5` | `config`, `Makefile` |
| « Qui agit » porté par le contexte (user, cron, système) | `pkg/actor` | `internal/utils/actor` |
| Journal d'audit des actions sensibles | `internal/audit/recorder.go` | `model.AuditRecorder`, implémenté par `internal/repository/audit_repository.go` (désormais transactionnel) |
| Cache Redis JSON + lecture « read-through » + décorateurs ; pannes de cache non bloquantes | `pkg/cache`, `internal/infrastructure/cache` | `internal/infrastructure/cache` (`Store`, `Loader`) |
| Migrations goose avec `Up`/`Down`, règle « ne jamais modifier une migration appliquée » | `migrations/`, `CLAUDE.md` | `migrations/` embarquées + `cmd/migrate` |
| Binaires cron : `--dry-run`, timeout global, acteur `cron`, logique extraite de `main()` | `cmd/cron-retention` | `cmd/retention` |
| Fakes écrits à la main plutôt que mocks générés | `test/fake` | `internal/repository/repositorytest` |
| testcontainers, build tags d'intégration, `TB` utilisable dans `TestMain` | `test/shared` | `internal/infrastructure/database/dbtest` |
| Serveur de test monté sur le vrai routeur | `test/shared/server.go` | `internal/app/app_test.go` |
| Tests « de contrat » sur la structure du code | `test/contract/wiring_test.go`, `docs/arch-doctor-go.md` | `internal/archtest` (automatisé) |
| `docker compose` avec healthchecks, cibles `make` par catégorie | `docker-compose.yml`, `Makefile` | `compose.yaml`, `Makefile` (`make help`) |
| Principes de travail pour Claude Code | `CLAUDE.md` | `CLAUDE.md` |
| Clients typés générés depuis OpenAPI (`oapi-codegen`) | `internal/clients/climate` | Recommandé dans `docs/decisions.md` (§8) |

## 2. Corrigé ou réorganisé

| # | Constat dans Bhumist | Conséquence | Dans le squelette |
|---|---|---|---|
| 1 | Erreurs propagées par `panic` (`renderError`, `handler_helpers.go`) puis `recover` | Flux de contrôle caché, non idiomatique | Handlers `func(w, r) error` + `httpx.Handle` |
| 2 | Statut HTTP déduit de sous-chaînes du message (`error_handler.go:116-121`) | Changer un message change un statut | `apperr.Kind` → statut, dans une seule fonction |
| 3 | `ErrorHandler` importe chaque paquet de domaine (`error_handler.go:22-26`) | Couplage croissant à chaque domaine | Le middleware ne connaît que `apperr` |
| 4 | `http.ListenAndServe` sans timeouts (`cmd/api/main.go:100`) alors que `http.read_timeout`… sont chargés dans la config | Exposé aux clients lents (slowloris), config trompeuse | `server.New` fixe tous les timeouts |
| 5 | Pas d'arrêt gracieux (aucune gestion de signal) | Requêtes coupées à chaque déploiement | `signal.NotifyContext`, drain via `/readyz`, `Shutdown` borné |
| 6 | Pool `database/sql` sans limites (`config.NewPostgresDB`) | Connexions illimitées vers PostgreSQL | `pgxpool` paramétré (`DATABASE_MAX_CONNS`…) |
| 7 | `/health` ne vérifie aucune dépendance | L'orchestrateur envoie du trafic à une instance sans base | `/livez` + `/readyz` (PostgreSQL, Redis, drain) |
| 8 | Logs JSON construits avec `log.Printf` et `"%s"` (`logger.go:54`) ; champ `route` = chemin brut | JSON invalide / injection si le chemin contient `"` ; cardinalité non maîtrisée | `slog` ; `route` = motif chi |
| 9 | `trustedProxies` est une variable globale réécrite à chaque construction (`ratelimit.go:68`) ; goroutine de nettoyage jamais arrêtée (`:96`) ; quand la table est pleine, tout nouveau client reçoit un seau vide et est refusé (`:123`) | État global, fuite de goroutine, déni de service par rotation d'IP | État encapsulé, nettoyage arrêté par contexte, seau partagé de débordement, `x/time/rate` |
| 10 | Jeton accepté en query string `?access_token=` (`auth.go:45-47`) | Jetons dans les logs de proxies/CDN et l'historique | En-tête `Authorization` uniquement |
| 11 | Auth couplée au SDK Firebase via un singleton global ; contournement de test par `X-Test-User-ID` + deux drapeaux | Difficile à tester, dépendances lourdes, fournisseur imposé | `auth.Verifier` : JWKS pour tout fournisseur OIDC, `DevVerifier` refusé hors dev/test |
| 12 | `SecurityHeaders` lit `os.Getenv("APP_ENV")` directement (`security_headers.go:12`) | Configuration dispersée | Option `HTTP_HSTS` passée explicitement |
| 13 | Viper + `config.yaml` + `os.Getenv` épars ; le paquet `config` ouvre aussi les connexions | Source de vérité éclatée | `caarlos0/env`, un seul `Config` validé ; connexions dans `infrastructure/database` et `infrastructure/cache` |
| 14 | `log.Fatalf` dans le câblage (`wire_repos.go:157`) | `defer` ignorés, non testable | Erreurs retournées, ressources fermées dans l'ordre inverse |
| 15 | Branches DevMode/surcharges de test dupliquées dans `wireRepositories` | Câblage difficile à lire | Une seule construction ; les tests fournissent leurs `Deps` |
| 16 | Requêtes SQL dans des middlewares (`role.go`, `tier_resolver.go`), `*sql.DB` dans des handlers (listés dans `arch-doctor-go.md`) | Violation d'inversion de dépendances | Accès aux données derrière des ports ; `RequireRole` s'appuie sur les rôles du jeton |
| 17 | Dossiers par couche à plat, nommés différemment d'une couche à l'autre : un fichier par domaine dans `internal/handler` (49 fichiers), un fichier par cas d'usage dans `internal/usecase` (`compute_projection.go`, `get_top_crops.go`…) ; READMEs de quelques lignes, sans exemple ni liste des imports autorisés | Retrouver les fichiers d'une fonctionnalité demande de fouiller chaque couche ; rien ne dit où ranger un nouveau fichier ni ce qu'il peut importer | Dossiers par couche conservés (choix expliqué dans `docs/decisions.md` §1). Un fichier par entité et par couche, nommé pareil partout (`project.go`, `project_repository.go`, `project_service.go`, `project_dto.go`, `project_controller.go`). Chaque dossier a un README : rôle, règles de dépendances, exemple réel, squelette. Les règles sont vérifiées par `internal/archtest` (voir #24) |
| 18 | swaggo : Swagger 2.0, `docs/docs.go` de 274 Ko versionné, génération qui bloquait (commit `80b9ddf`) | Contrat dérivé de commentaires, maintenance coûteuse | OpenAPI 3.1 écrit à la main + test de concordance avec le routeur |
| 19 | `chimiddleware.RequestID` réutilise n'importe quelle valeur d'en-tête entrante | Contenu arbitraire injecté dans les logs et les réponses | ID entrant validé (longueur, alphabet), sinon UUIDv7 |
| 20 | Corps JSON non bornés (sauf un handler), champs inconnus acceptés, erreurs de validation brutes avec noms de champs Go | Mémoire exposée, fautes de frappe client silencieuses, messages peu exploitables | `BodyLimit` global (413), `DisallowUnknownFields`, erreurs par champ avec noms JSON |
| 21 | Échec d'audit journalisé puis ignoré, hors transaction métier | Action sensible possible sans trace | Audit dans la même transaction que la modification |
| 22 | Read-through sans protection contre l'effet de meute | N requêtes simultanées sur une clé froide = N appels à l'origine | `cache.Loader` avec `singleflight` |
| 23 | Mise en place des bases de test : migrations jusqu'à la 37, insertion de cultures factices, puis suite (`test/shared/db.go`) | Fragile, données partagées entre tests | Template migré + une base clonée par test |
| 24 | Vérifications d'architecture par recherche de chaînes dans les sources et liste de `grep` manuels | Contournables, à lancer à la main | `internal/archtest` analyse les imports à chaque `go test` |
| 25 | Les fakes peuvent diverger des vrais repositories | Tests unitaires verts sur un comportement erroné | Suite de contrat exécutée sur le fake et sur PostgreSQL |
| 26 | CI : `go test ./...` seul, avec `DEV_MODE` | Ni lint, ni race detector, ni intégration, ni audit de dépendances | Lint, tests `-race` avec PostgreSQL, `govulncheck`, build d'image, Dependabot |
| 27 | `lib/pq` (en maintenance), `go-redis/v8`, `google/uuid` | Dépendances datées | `pgx/v5`, `redis/go-redis/v9`, paquet `uuid` standard (Go 1.27) |
| 28 | Artefacts de debug versionnés (`_qa_api_fix.err`, `_qa_apilog.err`, `bash.exe.stackdump`, `proj_test.json`), README de 5 octets | Bruit, informations internes exposées | `.gitignore` strict, README complet |

## 3. Écarté volontairement

Tout ce qui est propre au produit Bhumist : scoring de culture, climat/sol/GAEZ/CMIP6, heatmap,
rapports PDF (Chromium), Stripe et paliers d'abonnement (`free`/`pro`/`team`/`enterprise`), quotas
journaliers, organisations et rôles en base, clés d'API d'entreprise, webhooks sortants signés,
notifications FCM/SendGrid, assistant guidé, scripts Python d'ingestion de données, service climat.

Certains de ces mécanismes sont de bons modèles à reprendre au besoin :

| Mécanisme | Où dans Bhumist | Comment l'ajouter ici |
|---|---|---|
| Quotas journaliers atomiques (script Lua `INCR` + `PEXPIREAT`) | `internal/middleware/usage_quota.go` | Middleware dans `internal/middleware`, qui dépend d'une petite interface de compteur implémentée par un client Redis dans `internal/infrastructure` |
| Rate limiting distribué | `internal/middleware/rate_limit.go` | Idem, mais en script Lua : l'`INCR` puis `EXPIRE` séparés de Bhumist (`:45-53`) ne sont pas atomiques |
| Clés d'API hachées (SHA-256) avec cache court | `internal/middleware/apikey_auth.go` | Une implémentation supplémentaire de `auth.Verifier` |
| Webhooks signés HMAC avec période de grâce lors de la rotation du secret | `internal/infrastructure/webhook/dispatcher.go` | Fonctionnalité dédiée (un fichier `webhook_*` par couche, client d'envoi dans `internal/infrastructure`), secret stocké chiffré |
| Vérification de révocation Firebase / ré-authentification récente | `RequireFreshAuth`, `RequireRecentAuth` | `Principal.AuthTime` est déjà exposé ; voir `docs/decisions.md` §7 |
| Capture Sentry des 5xx | `internal/handler/errors.go` | Point d'accroche unique : `httpx.WriteError` (`internal/utils/httpx/problem.go`) |

## 4. À signaler côté Bhumist

Relevés en lisant le code, hors du périmètre de ce squelette :

- `.env.qa-backup` est **versionné** et contient des variables nommées `STRIPE_SECRET_KEY`,
  `STRIPE_WEBHOOK_SECRET`, `SENDGRID_API_KEY`, `REPORT_SIGNING_SECRET`, `GBIF_PASSWORD`… Leurs valeurs n'ont
  pas été inspectées : si elles sont réelles, les révoquer, les régénérer et retirer le fichier de
  l'historique git.
- Les points 4 à 10 de la section 2 (timeouts, arrêt gracieux, pool de connexions, logs JSON, rate
  limiter, jeton en query string) s'appliquent tels quels à l'API en production.
