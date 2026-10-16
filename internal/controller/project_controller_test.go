package controller_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/controller"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
	"github.com/JustinSornay/go_skeleton/internal/service"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
)

// newProjectServer serves the project controller with the real service on
// the in-memory fakes (see mount in controller_test.go).
func newProjectServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := service.NewProjectService(repositorytest.NewProjectRepository(), repositorytest.Tx{}, &repositorytest.AuditRecorder{})
	return mount(t, "/v1/projects", controller.NewProjectController(svc).Routes())
}

type projectJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func TestProjectCRUD(t *testing.T) {
	srv := newProjectServer(t)
	alice := client{t, srv.URL, "alice"}
	bob := client{t, srv.URL, "bob"}

	resp, body := alice.do(http.MethodPost, "/v1/projects", `{"name":"Orchard","description":"apples"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := decode[projectJSON](t, body)
	if resp.Header.Get("Location") != "/v1/projects/"+created.ID {
		t.Errorf("Location = %q", resp.Header.Get("Location"))
	}

	resp, body = alice.do(http.MethodGet, "/v1/projects/"+created.ID, "")
	if resp.StatusCode != http.StatusOK || decode[projectJSON](t, body).Name != "Orchard" {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}

	if resp, _ := bob.do(http.MethodGet, "/v1/projects/"+created.ID, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("another owner must get 404, got %d", resp.StatusCode)
	}

	resp, body = alice.do(http.MethodPatch, "/v1/projects/"+created.ID, `{"description":"pears"}`)
	if got := decode[projectJSON](t, body); resp.StatusCode != http.StatusOK || got.Name != "Orchard" || got.Description != "pears" {
		t.Fatalf("patch: %d %s", resp.StatusCode, body)
	}

	resp, body = alice.do(http.MethodGet, "/v1/projects?limit=10", "")
	list := decode[httpx.List[projectJSON]](t, body)
	if resp.StatusCode != http.StatusOK || len(list.Data) != 1 || list.Page.Total != 1 || list.Page.Limit != 10 {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}

	if resp, _ := alice.do(http.MethodDelete, "/v1/projects/"+created.ID, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := alice.do(http.MethodDelete, "/v1/projects/"+created.ID, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: %d", resp.StatusCode)
	}
}

func TestProjectErrors(t *testing.T) {
	srv := newProjectServer(t)
	alice := client{t, srv.URL, "alice"}
	alice.do(http.MethodPost, "/v1/projects", `{"name":"Orchard"}`)

	tests := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"anonymous", http.MethodGet, "/v1/projects", "", 401, "auth.required"},
		{"missing name", http.MethodPost, "/v1/projects", `{"description":"x"}`, 400, "request.invalid"},
		{"unknown field", http.MethodPost, "/v1/projects", `{"name":"x","color":"red"}`, 400, "request.malformed_body"},
		{"duplicate", http.MethodPost, "/v1/projects", `{"name":"orchard"}`, 409, "project.name_taken"},
		{"bad id", http.MethodGet, "/v1/projects/42", "", 404, "project.not_found"},
		{"bad limit", http.MethodGet, "/v1/projects?limit=-1", "", 400, "request.invalid"},
		{"empty patch name", http.MethodPatch, "/v1/projects/01890a5d-ac96-774b-bcce-b302099a8057", `{"name":""}`, 400, "request.invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := alice
			if tt.name == "anonymous" {
				c.user = ""
			}
			resp, body := c.do(tt.method, tt.path, tt.body)
			p := decode[httpx.Problem](t, body)
			if resp.StatusCode != tt.status || p.Code != tt.code {
				t.Fatalf("got %d %q, want %d %q (%s)", resp.StatusCode, p.Code, tt.status, tt.code, body)
			}
			if resp.Header.Get("Content-Type") != httpx.ProblemContentType {
				t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
			}
		})
	}
}
