# `config/` — configuration par variables d'environnement

Chargement de la configuration depuis l'environnement (twelve-factor) : chaque variable est déclarée
avec son type et sa valeur par défaut, et l'ensemble est validé au démarrage. Tout tient dans
[`config.go`](config.go). Carte des dossiers : [`internal/README.md`](../README.md).

## Ce qui va ici

- La déclaration de chaque variable : une struct par préfixe (`HTTP` ↔ `HTTP_…`).
- Les règles de cohérence et les garde-fous de production (`Config.Validate`).
- Les chargements partiels des binaires de maintenance (`LoadDatabase`).

| Fonction | Usage |
|---|---|
| `Load()` | `cmd/api` : lit l'environnement du processus |
| `LoadFrom(vars)` | Tests : lit une `map[string]string`, sans toucher à l'environnement réel |
| `LoadDatabase()` | `cmd/migrate`, `cmd/retention` : ne lit que `DATABASE_*` |
| `Config.Validate()` | Appelée par `LoadFrom` (et utilise `IsProduction()`) ; toutes les erreurs sont réunies avec `errors.Join` |

## Ce qui n'y va pas

- L'ouverture des connexions → [`infrastructure/`](../infrastructure/), appelée par [`app/`](../app/).
- Un `os.Getenv` ailleurs dans le code.
- Un `if cfg.App.Env == "production"` dans le code métier : exposer plutôt un réglage explicite.

## Dépendances

Aucun paquet de `internal/` : seulement la bibliothèque standard et `github.com/caarlos0/env/v11`. Seuls
`app/` et `cmd/*` importent `config` ; les autres couches reçoivent des valeurs simples ou une struct
`Options` (`database.Options`, `server.Options`). Vérifié par `internal/archtest`.

## Conventions

- Nom de variable = préfixe de la struct (`envPrefix:"RATE_LIMIT_"`) + tag `env` (`RPS`) →
  `RATE_LIMIT_RPS`.
- Valeur par défaut avec `envDefault`, variable obligatoire avec `required`, secret avec `unset`
  (retiré de l'environnement une fois lu, comme `DATABASE_URL`).
- Durées en `time.Duration` (`"20s"`), listes en `[]string` avec `envSeparator:","`.
- Toute nouvelle variable est aussi ajoutée, commentée, à [`.env.example`](../../.env.example).
- Une règle propre à la production va dans le bloc `if c.IsProduction()` de `Validate`.

## Exemple

Référence : la struct `RateLimit` de [`config.go`](config.go) (préfixe, valeurs par défaut,
commentaires, liste). [`example_test.go`](example_test.go), exécuté par `go test` : `ExampleLoadFrom`
(valeurs par défaut, listes nettoyées) et `ExampleLoadFrom_productionSafety` (toutes les erreurs de
production d'un coup).

Squelette d'une nouvelle section (`MAIL_FROM`, `MAIL_API_KEY`, `MAIL_TIMEOUT`) dans `config.go` :

```go
type Config struct {
	// … existing sections
	Mail Mail `envPrefix:"MAIL_"`
}

type Mail struct {
	From    string        `env:"FROM" envDefault:"no-reply@example.com"`
	APIKey  string        `env:"API_KEY,unset"` // secret: removed from the environment once read
	Timeout time.Duration `env:"TIMEOUT" envDefault:"10s"`
}

// In Validate, inside the `if c.IsProduction()` block:
if c.Mail.APIKey == "" {
	add("MAIL_API_KEY is required in production")
}
```

Ensuite : documenter les variables dans `.env.example`, puis passer `cfg.Mail` au client concerné dans
`app/`.

## Tester

```bash
go test ./internal/config/...
```

Les tests appellent `config.LoadFrom` avec les helpers `baseEnv()` et `with(...)` de
[`config_test.go`](config_test.go). Toute nouvelle règle de `Validate` ajoute un cas à `TestInvalid`
(la valeur refusée et un extrait du message attendu). Une nouvelle variable obligatoire en production
doit aussi être ajoutée à `TestValidProduction` et à la sortie attendue de
`ExampleLoadFrom_productionSafety` : ces deux tests échouent sinon.
