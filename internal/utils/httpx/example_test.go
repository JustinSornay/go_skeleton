package httpx_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
)

var errProjectArchived = apperr.Conflict("project.archived", "the project is archived")

// ExampleHandle shows a handler that reports a failure by returning an
// *apperr.Error. Handle renders it as an RFC 9457 problem whose status comes
// from the error's Kind (KindConflict: 409); the wrapping context added with
// fmt.Errorf is never sent to the client.
func ExampleHandle() {
	h := httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		if r.URL.Query().Get("archived") == "true" {
			return fmt.Errorf("renaming project: %w", errProjectArchived)
		}
		return httpx.JSON(w, http.StatusOK, map[string]string{"name": "Orchard"})
	})

	for _, target := range []string{"/v1/projects/1", "/v1/projects/1?archived=true"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, target, nil))
		fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
		fmt.Print(rec.Body.String())
	}
	// Output:
	// 200 application/json
	// {"name":"Orchard"}
	// 409 application/problem+json
	// {"type":"about:blank","title":"Conflict","status":409,"detail":"the project is archived","instance":"/v1/projects/1","code":"project.archived"}
}

type createProjectRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

// ExampleBind decodes and validates a JSON body. Every failing field is
// listed in the problem's "errors" member, under its JSON name.
func ExampleBind() {
	h := httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		req, err := httpx.Bind[createProjectRequest](r)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, req)
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"name":""}`)))
	fmt.Println(rec.Code)
	fmt.Print(rec.Body.String())
	// Output:
	// 400
	// {"type":"about:blank","title":"Bad Request","status":400,"detail":"request validation failed","instance":"/v1/projects","code":"request.invalid","errors":[{"field":"name","rule":"required","message":"is required"}]}
}

type listProjectsRequest struct {
	httpx.PageQuery
	Search string `query:"q" validate:"max=100"`
}

// ExampleBindQuery is the shape of every list endpoint: the request embeds
// PageQuery, Normalize applies the defaults, and the response is a List.
func ExampleBindQuery() {
	h := httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var req listProjectsRequest
		if err := httpx.BindQuery(r, &req); err != nil {
			return err
		}
		page := req.Normalize()
		names := []string{"Orchard"} // would come from the service, filtered by req.Search
		return httpx.JSON(w, http.StatusOK, httpx.List[string]{
			Data: names,
			Page: httpx.Page{Limit: page.Limit, Offset: page.Offset, Total: len(names)},
		})
	})

	for _, target := range []string{"/v1/projects?q=orch", "/v1/projects?limit=500"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		fmt.Print(rec.Code, " ", rec.Body.String())
	}
	// Output:
	// 200 {"data":["Orchard"],"page":{"limit":20,"offset":0,"total":1}}
	// 400 {"type":"about:blank","title":"Bad Request","status":400,"detail":"request validation failed","instance":"/v1/projects","code":"request.invalid","errors":[{"field":"limit","rule":"max","message":"must be at most 100"}]}
}
