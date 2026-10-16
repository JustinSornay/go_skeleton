# `utils/` — utilitaires sans état

Petits paquets techniques sans état métier, partagés par les couches : modèle d'erreur, plomberie
HTTP, logs, identifiant de requête, auteur d'une action. Carte des dossiers :
[`internal/README.md`](../README.md).

## Ce qui va ici

Du code générique, réutilisable tel quel dans un autre projet, sans connexion ni cycle de vie. Un sujet
par paquet, nommé d'après lui.

| Paquet | Rôle | Identifiants principaux |
|---|---|---|
| [`apperr/`](apperr/apperr.go) | Erreurs applicatives indépendantes du transport : `Kind`, `Code` public stable, détails par champ | `Error`, `Kind…`, `New`, `NotFound`, `Conflict`, `Invalid`, `Wrap`, `WithFields`, `As`, `KindOf` |
| [`httpx/`](httpx/httpx.go) | Handlers qui retournent `error`, réponses RFC 9457, binding et validation, pagination | `Handle`, `HandlerFunc`, `JSON`, `NoContent`, `PathUUID`, `WriteError`, `StatusFor`, `Bind`, `BindQuery`, `PageQuery`, `List` |
| [`logging/`](logging/logging.go) | `slog` configuré, logger propre à la requête porté par le contexte | `New`, `Options`, `FromContext`, `WithContext`, `Discard` |
| [`requestid/`](requestid/requestid.go) | Identifiant de corrélation (`X-Request-ID`) | `Header`, `With`, `From`, `New`, `Valid` |
| [`actor/`](actor/actor.go) | Qui agit (utilisateur, cron, système), porté par le contexte | `Actor`, `User`, `Cron`, `System`, `With`, `From` |

## Ce qui n'y va pas

- Un concept métier (entité, règle, erreur métier) → [`model/`](../model/).
- Ce qui ouvre une connexion ou a un cycle de vie (pool, client, serveur) →
  [`infrastructure/`](../infrastructure/).
- Un helper utilisé par une seule couche : il reste dans cette couche (ex. `currentUser` dans
  `controller/`).
- Un paquet fourre-tout (`utils/common`, `utils/helpers`).

## Dépendances

Un paquet de `utils/` n'importe que d'autres paquets de `utils/` (plus la bibliothèque standard et les
paquets tiers). Tout le monde peut importer `utils/`, sauf `config/`, avec deux restrictions : `model/`
n'importe que `utils/apperr`, et `service/` n'importe jamais `utils/httpx` (le métier ne connaît pas
HTTP). Vérifié par `internal/archtest`.

## Conventions

- Nom de paquet court, au singulier, sans suffixe `util` : `requestid`, pas `requestidutils`.
- Valeur dans le contexte : clé privée `type ctxKey struct{}`, accesseurs `With(ctx, v)` et
  `From(ctx)` ; `From` renvoie une valeur par défaut sûre quand rien n'a été posé (`actor.From` →
  `actor.System`, `requestid.From` → `""`). Variante acceptée quand le nom du paquet ne suffit pas :
  `logging.WithContext` / `logging.FromContext` (qui renvoie `slog.Default()`).
- Tests en paquet externe (`package actor_test`), sans Docker.

## Exemple

- Référence : [`actor/actor.go`](actor/actor.go), un paquet complet en quelques lignes (constantes,
  valeur dans le contexte, valeur par défaut).
- [`apperr/example_test.go`](apperr/example_test.go) : déclarer une sentinelle, la retrouver avec
  `errors.Is` après `Wrap` et `fmt.Errorf("…: %w")`, `KindOf`, `WithFields`.
- [`httpx/example_test.go`](httpx/example_test.go) : un handler qui retourne une erreur `apperr`
  rendue en RFC 9457 par `httpx.Handle` (`ExampleHandle`), la validation d'un corps JSON
  (`ExampleBind`), un endpoint de liste avec `PageQuery` et `List` (`ExampleBindQuery`).

Squelette d'un nouveau paquet (`utils/locale/locale.go`) :

```go
// Package locale carries the caller's preferred language in the context.
package locale

import "context"

// Default is returned when no locale was set.
const Default = "en"

type ctxKey struct{}

// With returns a copy of ctx carrying tag.
func With(ctx context.Context, tag string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tag)
}

// From returns the locale stored in ctx, or Default.
func From(ctx context.Context) string {
	if tag, ok := ctx.Value(ctxKey{}).(string); ok {
		return tag
	}
	return Default
}
```

Rien à déclarer ailleurs : `internal/archtest` range automatiquement tout sous-dossier de
`internal/utils` dans la couche `utils`.

## Tester

```bash
go test ./internal/utils/...
```

Les exemples (`func ExampleXxx` avec un commentaire `// Output:`) font partie des tests : si la sortie
change, `go test` échoue.
