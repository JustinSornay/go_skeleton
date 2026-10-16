# Décisions techniques

Chaque décision indique le contexte, le choix et ce qu'il faudrait pour la remettre en cause.
Ajouter une entrée quand un choix structurant change.

## 1. Organisation par fonctionnalité, cœur séparé des adaptateurs

- **Contexte** : dans Bhumist, l'organisation par couches à plat (`handler/` ≈ 50 fichiers, `usecase/`,
  `persistence/`, DTO dans un paquet séparé) disperse une même fonctionnalité dans 5 dossiers, et la
  documentation interne prévoyait déjà de découper `handler/` au-delà de 55 fichiers.
- **Choix** : un dossier par module (`internal/project`), avec le cœur (domaine + cas d'usage) à la racine
  du module et les adaptateurs en sous-paquets (`httpapi`, `postgres`). La séparation des couches de la
  clean architecture est conservée, mais regroupée par fonctionnalité.
- **Garde-fou** : `internal/archtest` vérifie les règles d'imports.

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
  fichiers `.sql` sans changer l'architecture (il produirait le contenu des paquets `postgres`).

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

## 10. Faux écrits à la main + suite de contrat

Pas de mocks générés (choix de Bhumist conservé). Nouveauté : la suite `RepositoryContract` est exécutée
contre le faux **et** contre PostgreSQL, ce qui garantit que le faux reproduit fidèlement le comportement
réel (portée par propriétaire, unicité insensible à la casse, ordre, pagination).

## 11. Bases de test clonées depuis un template

Un serveur PostgreSQL par binaire de test (testcontainers ou `TEST_DATABASE_URL`), un template migré une
fois, puis `CREATE DATABASE … TEMPLATE` par test : isolation totale en quelques millisecondes, plus besoin
de données de départ partagées ni de bouts de code pour contourner l'ordre des migrations.

## 12. UUIDv7 de la bibliothèque standard

Go 1.27 fournit le paquet `uuid`. Les identifiants sont des UUIDv7 générés par l'application : ordonnés
dans le temps (insertion locale dans l'index de clé primaire) et connus avant l'écriture en base.

## 13. Observabilité : Prometheus et slog par défaut, pas d'OpenTelemetry

Les métriques RED et les logs corrélés couvrent l'essentiel sans infrastructure supplémentaire. Pour le
tracing distribué, ajouter le SDK OpenTelemetry dans `internal/platform` (initialisation dans
`app.New`, middleware `otelhttp` dans le routeur, `otelpgx` pour les requêtes SQL). Pour Sentry, brancher
la capture des 5xx dans `httpx.WriteError`, seul point par lequel passent toutes les erreurs serveur.

## 14. Rate limiting en mémoire

Suffisant pour protéger une instance contre les rafales. Avec plusieurs réplicas, la limite effective est
multipliée par leur nombre. Pour une limite globale exacte ou des quotas par utilisateur (comme les
quotas journaliers de Bhumist), ajouter un limiteur Redis atomique (script Lua `INCR` + `PEXPIRE`).
