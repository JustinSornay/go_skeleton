package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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

// listPackages returns every folder of the module under root that holds Go
// files, with the imports of its non-test files. A folder holding only test
// files is listed with no imports, so the unknown-folder check still sees it;
// test files themselves may import test kits and are not checked. Reading
// the files directly (instead of running `go list`) lets the go test cache
// notice when an import changes.
func listPackages(t *testing.T, root string) []pkg {
	t.Helper()
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
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		dir, _ := filepath.Rel(root, filepath.Dir(path))
		if byDir[dir] == nil {
			byDir[dir] = map[string]bool{}
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
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
	return pkgs
}

// layer classifies a package by the folder it lives in.
type layer struct {
	// name is the folder under internal/ ("model", "service", "utils", …),
	// or "cmd", "outside" (module packages outside internal/ and cmd/, e.g.
	// migrations), "external" (stdlib and third-party) or "unknown" (a folder
	// under internal/ that is not a known layer).
	name string
	// testkit is set for test-support packages (repositorytest, dbtest):
	// their name ends in "test", like the standard library's httptest.
	testkit bool
}

// layers are the folders allowed directly under internal/.
var layers = []string{
	"app", "archtest", "config", "controller", "dto", "infrastructure",
	"middleware", "model", "repository", "service", "utils",
}

func classify(path string) layer {
	rel, ok := strings.CutPrefix(path, module+"/")
	if !ok {
		if path == module {
			return layer{name: "outside"}
		}
		return layer{name: "external"}
	}
	parts := strings.Split(rel, "/")
	switch {
	case parts[0] == "cmd":
		return layer{name: "cmd"}
	case parts[0] != "internal":
		return layer{name: "outside"}
	case len(parts) < 2 || !slices.Contains(layers, parts[1]):
		return layer{name: "unknown"}
	}
	last := parts[len(parts)-1]
	return layer{
		name:    parts[1],
		testkit: parts[1] != "archtest" && strings.HasSuffix(last, "test"),
	}
}

// Transport and storage packages the business layers must never import.
var (
	transport = []string{"net/http", "github.com/go-chi/chi"}
	storage   = []string{"database/sql", "github.com/jackc/pgx", "github.com/redis/go-redis"}
)

// rule restricts what a layer's non-test files may import.
type rule struct {
	// allow lists the module packages (path prefixes relative to the module)
	// the layer may import. Any other module package is forbidden.
	allow []string
	// outside also allows module packages outside internal/ (migrations).
	outside bool
	// deny lists import path prefixes that are forbidden even when allow or
	// the external rule would let them through.
	deny []string
}

// rules is the dependency matrix, documented in internal/README.md. Layers
// without an entry (app, archtest, cmd) are only subject to the global rules
// of TestDependencyRules.
var rules = map[string]rule{
	// Module packages outside internal/ and cmd/ (migrations, api) stay
	// independent of internal/: otherwise a layer allowed to import them
	// (infrastructure) could reach a forbidden one through them.
	"outside": {outside: true},
	"utils":   {allow: []string{"internal/utils"}},
	"config":  {},
	"model": {
		allow: []string{"internal/utils/apperr"},
		deny:  slices.Concat(transport, storage),
	},
	"service": {
		allow: []string{"internal/model", "internal/utils"},
		deny:  slices.Concat(transport, storage, []string{module + "/internal/utils/httpx"}),
	},
	"dto": {allow: []string{"internal/model", "internal/utils"}},
	"repository": {
		allow: []string{"internal/model", "internal/infrastructure/database", "internal/utils"},
		deny:  transport,
	},
	"infrastructure": {
		allow:   []string{"internal/infrastructure", "internal/utils"},
		outside: true,
	},
	"middleware": {allow: []string{"internal/utils", "internal/infrastructure/auth"}},
	"controller": {
		allow: []string{"internal/service", "internal/dto", "internal/model", "internal/utils", "internal/infrastructure/auth"},
	},
}

func hasPrefix(imp string, prefixes []string) bool {
	for _, p := range prefixes {
		if imp == p || strings.HasPrefix(imp, p+"/") {
			return true
		}
	}
	return false
}

// violation returns why a package of layer from may not import imp, or ""
// when the import is allowed.
func violation(from layer, imp string) string {
	to := classify(imp)
	switch {
	case to.name == "cmd":
		return "nothing may import a command"
	case to.name == "app" && from.name != "cmd":
		return "only commands may import the composition root (internal/app)"
	case to.testkit && !from.testkit:
		return "production code must not import test kits"
	}
	r, ok := rules[from.name]
	if !ok {
		return ""
	}
	if hasPrefix(imp, r.deny) {
		return from.name + " must not depend on " + strings.TrimPrefix(imp, module+"/")
	}
	switch to.name {
	case "external":
		return ""
	case "outside":
		if r.outside {
			return ""
		}
	default:
		if hasPrefix(strings.TrimPrefix(imp, module+"/"), r.allow) {
			return ""
		}
	}
	if len(r.allow) == 0 && !r.outside {
		return from.name + " must not import other packages of the module"
	}
	allowed := slices.Clone(r.allow)
	if r.outside {
		allowed = append(allowed, "module packages outside internal/")
	}
	return from.name + " may only import " + strings.Join(allowed, ", ")
}

func TestDependencyRules(t *testing.T) {
	pkgs := listPackages(t, "../..")
	if len(pkgs) < 10 {
		t.Fatalf("found only %d packages: wrong root?", len(pkgs))
	}
	for _, p := range pkgs {
		from := classify(p.ImportPath)
		name := strings.TrimPrefix(p.ImportPath, module+"/")
		if from.name == "unknown" {
			t.Errorf("%s: unknown folder; packages under internal/ must live in one of %v", name, layers)
			continue
		}
		for _, imp := range p.Imports {
			if why := violation(from, imp); why != "" {
				t.Errorf("%s imports %s: %s", name, strings.TrimPrefix(imp, module+"/"), why)
			}
		}
	}
}

// TestViolations checks the matrix itself on representative imports, so a
// mistake in the rules cannot silently disable one of them.
func TestViolations(t *testing.T) {
	m := func(rel string) string { return module + "/" + rel }
	tests := []struct {
		from, imp string
		allowed   bool
	}{
		{m("internal/model"), m("internal/utils/apperr"), true},
		{m("internal/model"), "net/http", false},
		{m("internal/model"), "github.com/jackc/pgx/v5", false},
		{m("internal/model"), m("internal/utils/httpx"), false},
		{m("internal/model"), m("internal/service"), false},
		{m("internal/service"), m("internal/model"), true},
		{m("internal/service"), m("internal/utils/actor"), true},
		{m("internal/service"), m("internal/utils/httpx"), false},
		{m("internal/service"), "github.com/go-chi/chi/v5", false},
		{m("internal/service"), m("internal/repository"), false},
		{m("internal/dto"), m("internal/utils/httpx"), true},
		{m("internal/dto"), m("internal/service"), false},
		{m("internal/repository"), m("internal/infrastructure/database"), true},
		{m("internal/repository"), m("internal/infrastructure/cache"), false},
		{m("internal/repository"), "net/http", false},
		{m("internal/repository/repositorytest"), m("internal/model"), true},
		{m("internal/repository/repositorytest"), "github.com/go-chi/chi/v5", false},
		{m("internal/infrastructure/database/dbtest"), m("migrations"), true},
		{m("internal/infrastructure/cache"), m("internal/utils/logging"), true},
		{m("internal/infrastructure/auth"), m("internal/model"), false},
		{m("internal/infrastructure/auth"), m("internal/config"), false},
		{m("internal/middleware"), m("internal/infrastructure/auth"), true},
		{m("internal/middleware"), m("internal/infrastructure/database"), false},
		{m("internal/middleware"), m("internal/service"), false},
		{m("internal/controller"), m("internal/service"), true},
		{m("internal/controller"), m("internal/repository"), false},
		{m("internal/controller"), m("internal/infrastructure/database"), false},
		{m("internal/controller"), m("internal/repository/repositorytest"), false},
		{m("internal/utils/httpx"), m("internal/utils/apperr"), true},
		{m("internal/utils/httpx"), m("internal/model"), false},
		{m("internal/config"), m("internal/utils/logging"), false},
		{m("internal/app"), m("internal/repository"), true},
		{m("internal/app"), m("internal/infrastructure/database/dbtest"), false},
		{m("internal/service"), m("internal/app"), false},
		{m("cmd/api"), m("internal/app"), true},
		{m("internal/app"), m("cmd/api"), false},
		{m("migrations"), m("internal/model"), false},
		{m("migrations"), m("internal/repository"), false},
		{m("api"), m("internal/service"), false},
		{module, m("internal/utils/apperr"), false},
		{m("api"), m("migrations"), true},
	}
	for _, tt := range tests {
		why := violation(classify(tt.from), tt.imp)
		if allowed := why == ""; allowed != tt.allowed {
			t.Errorf("%s -> %s: allowed = %v (%q), want %v", tt.from, tt.imp, allowed, why, tt.allowed)
		}
	}
}

// TestListPackagesSeesTestOnlyFolders guards the unknown-folder check: a
// leftover folder holding only test files must still be listed.
func TestListPackagesSeesTestOnlyFolders(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"internal/model/model.go":                     "package model\n\nimport \"errors\"\n\nvar _ = errors.New\n",
		"internal/platform/leftover/leftover_test.go": "package leftover_test\n\nimport \"net/http\"\n\nvar _ = http.MethodGet\n",
	}
	for rel, src := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string][]string{}
	for _, p := range listPackages(t, root) {
		got[p.ImportPath] = p.Imports
	}
	if imps, ok := got[module+"/internal/model"]; !ok || !slices.Equal(imps, []string{"errors"}) {
		t.Errorf("internal/model: got %v (listed: %v), want imports [errors]", imps, ok)
	}
	leftover := module + "/internal/platform/leftover"
	if imps, ok := got[leftover]; !ok || len(imps) != 0 {
		t.Errorf("test-only folder: listed=%v imports=%v, want listed with no imports", ok, imps)
	}
	if classify(leftover).name != "unknown" {
		t.Errorf("%s must be classified as unknown", leftover)
	}
}

func TestClassify(t *testing.T) {
	tests := map[string]layer{
		module + "/cmd/api":                                 {name: "cmd"},
		module + "/internal/app":                            {name: "app"},
		module + "/internal/config":                         {name: "config"},
		module + "/internal/controller":                     {name: "controller"},
		module + "/internal/dto":                            {name: "dto"},
		module + "/internal/middleware":                     {name: "middleware"},
		module + "/internal/model":                          {name: "model"},
		module + "/internal/repository":                     {name: "repository"},
		module + "/internal/repository/repositorytest":      {name: "repository", testkit: true},
		module + "/internal/service":                        {name: "service"},
		module + "/internal/infrastructure/database":        {name: "infrastructure"},
		module + "/internal/infrastructure/database/dbtest": {name: "infrastructure", testkit: true},
		module + "/internal/utils/httpx":                    {name: "utils"},
		module + "/internal/archtest":                       {name: "archtest"},
		module + "/internal/platform/httpx":                 {name: "unknown"},
		module + "/migrations":                              {name: "outside"},
		module:                                              {name: "outside"},
		"net/http":                                          {name: "external"},
	}
	for path, want := range tests {
		if got := classify(path); got != want {
			t.Errorf("classify(%q) = %+v, want %+v", path, got, want)
		}
	}
}
