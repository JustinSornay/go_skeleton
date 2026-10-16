package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/JustinSornay/go_skeleton"

type pkg struct {
	ImportPath string
	Imports    []string
}

// listPackages parses the import declarations of every non-test Go file of
// the module. Reading the files directly (instead of running `go list`)
// lets the go test cache notice when an import changes.
func listPackages(t *testing.T) []pkg {
	t.Helper()
	root := "../.."
	byDir := map[string]map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		dir, _ := filepath.Rel(root, filepath.Dir(path))
		if byDir[dir] == nil {
			byDir[dir] = map[string]bool{}
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			byDir[dir][p] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pkgs := make([]pkg, 0, len(byDir))
	for dir, imports := range byDir {
		p := pkg{ImportPath: module + "/" + filepath.ToSlash(dir)}
		if dir == "." {
			p.ImportPath = module
		}
		for imp := range imports {
			p.Imports = append(p.Imports, imp)
		}
		slices.Sort(p.Imports)
		pkgs = append(pkgs, p)
	}
	if len(pkgs) < 10 {
		t.Fatalf("found only %d packages: wrong root?", len(pkgs))
	}
	return pkgs
}

// layer classifies a package path relative to the module.
type layer struct {
	kind   string // "cmd", "app", "platform", "core", "adapter", "testkit", "other"
	module string // business module name for core/adapter/testkit
	sub    string // adapter name ("httpapi", "postgres", …)
}

func classify(path string) layer {
	rel, ok := strings.CutPrefix(path, module+"/")
	if !ok {
		return layer{kind: "external"}
	}
	parts := strings.Split(rel, "/")
	switch {
	case parts[0] == "cmd":
		return layer{kind: "cmd"}
	case parts[0] != "internal" || len(parts) < 2:
		return layer{kind: "other"}
	case parts[1] == "app":
		return layer{kind: "app"}
	case parts[1] == "platform":
		return layer{kind: "platform", sub: strings.Join(parts[2:], "/")}
	case parts[1] == "archtest":
		return layer{kind: "other"}
	case len(parts) == 2:
		return layer{kind: "core", module: parts[1]}
	case strings.HasSuffix(parts[2], "test"):
		return layer{kind: "testkit", module: parts[1], sub: parts[2]}
	default:
		return layer{kind: "adapter", module: parts[1], sub: parts[2]}
	}
}

// Infrastructure packages a module core must never import directly.
var forbiddenInCore = []string{
	"net/http",
	"database/sql",
	"github.com/jackc/pgx",
	"github.com/redis/go-redis",
	"github.com/go-chi/chi",
	module + "/internal/platform/database",
	module + "/internal/platform/httpx",
	module + "/internal/platform/middleware",
}

func hasPrefix(imp string, prefixes []string) bool {
	for _, p := range prefixes {
		if imp == p || strings.HasPrefix(imp, p+"/") {
			return true
		}
	}
	return false
}

func TestDependencyRules(t *testing.T) {
	for _, p := range listPackages(t) {
		from := classify(p.ImportPath)
		for _, imp := range p.Imports {
			to := classify(imp)
			violation := ""
			switch {
			case to.kind == "cmd":
				violation = "nothing may import a command"
			case to.kind == "app" && from.kind != "cmd":
				violation = "only commands may import the composition root (internal/app)"
			case from.kind == "platform" && (to.kind == "core" || to.kind == "adapter" || to.kind == "testkit"):
				violation = "platform packages must not depend on business modules"
			case from.kind == "core" && hasPrefix(imp, forbiddenInCore):
				violation = "a module core (domain + use cases) must not depend on transport or storage"
			case from.kind == "core" && (to.kind == "adapter" || to.kind == "testkit"):
				violation = "a module core must not import adapters"
			case from.kind == "adapter" && to.kind == "adapter":
				violation = "adapters must not import other adapters"
			case from.kind == "adapter" && to.module != "" && to.module != from.module && to.kind != "core":
				violation = "modules may only use each other through their core package"
			case (from.kind == "core" || from.kind == "adapter") && to.kind == "testkit":
				violation = "production code must not import test kits"
			}
			if violation != "" {
				t.Errorf("%s imports %s: %s", strings.TrimPrefix(p.ImportPath, module+"/"), strings.TrimPrefix(imp, module+"/"), violation)
			}
		}
	}
}

func TestClassify(t *testing.T) {
	tests := map[string]string{
		module + "/cmd/api":                      "cmd",
		module + "/internal/app":                 "app",
		module + "/internal/platform/httpx":      "platform",
		module + "/internal/project":             "core",
		module + "/internal/project/postgres":    "adapter",
		module + "/internal/project/httpapi":     "adapter",
		module + "/internal/project/projecttest": "testkit",
		module + "/migrations":                   "other",
		"net/http":                               "external",
	}
	for path, want := range tests {
		if got := classify(path).kind; got != want {
			t.Errorf("classify(%q) = %q, want %q", path, got, want)
		}
	}
}
