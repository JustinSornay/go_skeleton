# Décisions techniques

Chaque décision indique le contexte, le choix et ce qu'il faudrait pour la remettre en cause.
Ajouter une entrée quand un choix structurant change.

## 1. Couches globales (controller/service/repository/model…)

- **Contexte** : l'équipe a choisi une organisation familière, avec un dossier par rôle classique,
  où l'on sait d'emblée où ranger un contrôleur, un service ou un repository. Une première version
  rangeait le code par fonctionnalité : un dossier par module, avec ses adaptateurs HTTP et PostgreSQL en
  sous-paquets. Elle était plus cohésive, mais moins immédiate à parcourir.
- **Choix** : sous `internal/`, un dossier global par couche :
  - `model`, `service`, `repository` (+ `repositorytest`), `dto`, `controller` ;
  - `middleware`, `infrastructure/*`, `utils/*`, `config` ;
  - `app` (racine de composition) et `archtest`.

  Chaque dossier a un README (rôle, ce qui y va, règles, conventions, squelette) et un exemple réel :
  l'entité `project`, qui traverse toutes les couches. La carte est dans
  [`internal/README.md`](../internal/README.md).
- **Compromis** : une fonctionnalité s'étale sur plusieurs dossiers, et les paquets sont partagés par
  toutes les entités. Deux choses compensent :
  - **une convention de nommage** : un fichier par couche nommé d'après l'entité (`project.go`,
    `project_repository.go`, `project_service.go`, `project_dto.go`, `project_controller.go`), et un
    préfixe d'entité sur les identifiants (`ErrProjectNotFound`, `NewProjectService`) ;
  - **des règles de dépendances vérifiées** par `internal/archtest` : `model` sans HTTP ni SQL,
    `service` sans transport ni stockage, `controller` sans accès aux repositories, kits de test
    interdits en production, dossiers inconnus refusés.

  L'inversion de dépendances de la clean architecture est conservée : `repository` implémente les
  interfaces déclarées dans `model`.
- **À reconsidérer si** le nombre d'entités rend les dossiers difficiles à parcourir. On pourrait alors
  découper chaque couche en sous-paquets par domaine, en adaptant `classify` et `rules` dans
  `internal/archtest/arch_test.go`.

## 2. Les handlers retournent des erreurs ; pas de `panic`

- **Contexte** : Bhumist propage les erreurs en paniquant (`renderError` → `panic(err)`), puis un
  middleware `recover` détermine le statut, en partie par recherche de sous-chaînes dans le message
  (`"invalid coordinates"` → 400). Modifier un message d'erreur peut changer silencieusement un statut HTTP,
  et le middleware d'erreurs doit importer chaque paquet de domaine.
- **Choix** : `type HandlerFunc func(w, r) error` + `httpx.Handle`. Les erreurs métier sont des
  `*apperr.Error` porteurs d'un `Kind` ; le mapping `Kind → statut` tient dans une fonction.
  `panic` est réservé aux bugs (le `Recoverer` les journalise et renvoie un 500).

## 3. Format d'erreur RFC 9457

RFC 9457 remplace RFC 7807 (même format `application/problem+json`). On y ajoute `code` (identifiant
stable), `request_id` et `errors` (erreurs par champ). `type` vaut `about:blank` : le `code` suffit aux
clients. Si des pages de documentation par erreur sont publiées un jour, `type` pourra pointer dessus.

## 4. Routeur chi conservé

Depuis Go 1.22, `http.ServeMux` gère méthodes et paramètres de chemin. chi reste utile pour les groupes
et sous-routeurs avec middlewares (`Group`, `Route`, `Mount`), les gestionnaires 404/405 personnalisés et
le motif de route (`RoutePattern`) utilisé comme label de métriques. chi est 100 % compatible `net/http`
et sans dépendance : en sortir plus tard reste simple.

## 5. pgx et SQL écrit à la main

- `jackc/pgx/v5` remplace `lib/pq` (en maintenance) : pool natif, meilleures performances, erreurs
  PostgreSQL typées, `CollectRows`/`RowToStructByName`.
- Pas d'ORM : le SQL reste lisible et revu comme du code (choix déjà fait dans Bhumist).
- Si le volume de requêtes grossit, [sqlc](https://sqlc.dev) peut générer le code de mapping à partir de
  fichiers `.sql` sans changer l'architecture (il produirait le code de mapping de `internal/repository`).

## 6. Configuration 100 % variables d'environnement

- **Contexte** : Bhumist combine Viper, un `config.yaml` et des `os.Getenv` dispersés (y compris dans des
  middlewares), avec des valeurs lues mais jamais utilisées (les timeouts HTTP).
- **Choix** : `caarlos0/env` (sans dépendance) ; chaque champ déclare sa variable, son type et sa valeur
  par défaut ; `Validate()` centralise les garde-fous de production. Les tests passent une `map` à
  `LoadFrom` au lieu de modifier l'environnement du processus.

## 7. Authentification indépendante du fournisseur (JWKS)

- **Contexte** : Bhumist dépend du SDK Firebase Admin (et de ses nombreuses dépendances GCP) et d'un
  singleton global ; le contournement de test repose sur un en-tête dédié et deux drapeaux.
- **Choix** : interface `auth.Verifier`. `JWTVerifier` valide n'importe quel JWT OIDC via son JWKS
  (Firebase compris, cf. `.env.example`). `DevVerifier` sert au local et aux tests, et la configuration
  le refuse hors `dev`/`test`.
- **Limite** : la vérification de révocation de Firebase (`VerifyIDTokenAndCheckRevoked`) n'est pas
  couverte par JWKS. Si elle est requise, ajouter une implémentation de `Verifier` utilisant le SDK pour
  les seules routes sensibles.

## 8. Contrat OpenAPI écrit à la main et testé

- **Contexte** : swaggo génère du Swagger 2.0 à partir de commentaires, produit un `docs.go` de plus de
  270 Ko, et la génération était lente ou bloquait.
- **Choix** : `api/openapi.yaml` (OpenAPI 3.1) écrit à la main, embarqué et servi via Swagger UI (ressources
  CDN épinglées avec Subresource Integrity). `TestRoutesMatchOpenAPI` échoue si une route existe sans être
  documentée ou inversement.
- Pour consommer des API externes, générer des clients typés avec `oapi-codegen` (comme le client climat
  de Bhumist) reste une bonne pratique.

## 9. Migrations embarquées

Les fichiers SQL sont embarqués (`embed.FS`) et appliqués par `cmd/migrate` (API Provider de goose).
Le binaire et son schéma voyagent ensemble ; aucun outil à installer en production. En production,
`migrate up` s'exécute comme étape de release (job, init container), jamais au démarrage de l'API.

## 10. Fakes écrits à la main + suite de contrat

Pas de mocks générés (choix de Bhumist conservé). Nouveauté : la suite
`repositorytest.ProjectRepositoryContract` est exécutée contre le fake **et** contre PostgreSQL, ce qui
garantit que le fake reproduit fidèlement le comportement réel (filtrage par propriétaire, unicité
insensible à la casse, ordre, pagination). Les fakes et les contrats vivent dans
`internal/repository/repositorytest`.

## 11. Bases de test clonées depuis un template

Un serveur PostgreSQL par binaire de test (testcontainers ou `TEST_DATABASE_URL`), un template migré une
fois, puis `CREATE DATABASE … TEMPLATE` par test : isolation totale en quelques millisecondes, plus besoin
de données de départ partagées ni de bouts de code pour contourner l'ordre des migrations.

## 12. UUIDv7 de la bibliothèque standard

Go 1.27 fournit le paquet `uuid`. Les identifiants sont des UUIDv7 générés par l'application : ordonnés
dans le temps (insertion locale dans l'index de clé primaire) et connus avant l'écriture en base.

## 13. Observabilité : Prometheus et slog par défaut, pas d'OpenTelemetry

Les métriques RED et les logs corrélés couvrent l'essentiel sans infrastructure supplémentaire. Pour le
tracing distribué, ajouter le SDK OpenTelemetry dans un paquet `internal/infrastructure/tracing`
(initialisation dans `app.New`, middleware `otelhttp` dans le routeur, `otelpgx` branché dans
`internal/infrastructure/database`). Pour Sentry, brancher la capture des 5xx dans `httpx.WriteError`
(`internal/utils/httpx/problem.go`), seul point par lequel passent toutes les erreurs serveur.

## 14. Rate limiting en mémoire

Suffisant pour protéger une instance contre les rafales. Avec plusieurs réplicas, la limite effective est
multipliée par leur nombre. Pour une limite globale exacte ou des quotas par utilisateur (comme les
quotas journaliers de Bhumist), ajouter un limiteur Redis atomique (script Lua `INCR` + `PEXPIRE`).
Le middleware, dans `internal/middleware`, dépendrait alors d'une petite interface de compteur.
Celle-ci serait implémentée dans `internal/infrastructure` et branchée par `internal/app`.
