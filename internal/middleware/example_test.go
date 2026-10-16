package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
	mw "github.com/JustinSornay/go_skeleton/internal/middleware"
	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
	"github.com/JustinSornay/go_skeleton/internal/utils/requestid"
)

type tenantKey struct{}

var errTenantRequired = apperr.Invalid("tenant.required", "the X-Tenant-ID header is required")

// requireTenant is a custom middleware with the usual shape: it reads a
// header, stores the value in the context (typed private key) and rejects
// the request with an RFC 9457 problem through httpx.WriteError, never with
// a panic or a hand-written error body.
func requireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-Tenant-ID")
		if tenant == "" {
			httpx.WriteError(w, r, errTenantRequired)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantKey{}, tenant)))
	})
}

// Example writes a custom middleware and chains it after RequestID.
// chi.Chain behaves like r.Use in internal/app/router.go: the first
// middleware listed is the outermost one, so RequestID runs first and the
// problem returned by requireTenant already carries the request ID.
func Example() {
	hello := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "tenant=%s request_id=%s", r.Context().Value(tenantKey{}), requestid.From(r.Context()))
	})
	h := chi.Chain(mw.RequestID, requireTenant).Handler(hello)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	req.Header.Set(requestid.Header, "req-1")
	req.Header.Set("X-Tenant-ID", "acme")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	fmt.Println(rec.Code, rec.Body.String())

	req.Header.Del("X-Tenant-ID")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var p httpx.Problem
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	fmt.Println(rec.Code, rec.Header().Get("Content-Type"), p.Code, p.RequestID)
	// Output:
	// 200 tenant=acme request_id=req-1
	// 400 application/problem+json tenant.required req-1
}

// ExampleRequireRole tests an access policy without any token: the
// principal that Authenticate would store is put in the context directly.
func ExampleRequireRole() {
	h := mw.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, roles := range [][]string{{"admin"}, {"editor"}} {
		ctx := auth.WithPrincipal(context.Background(), auth.Principal{Subject: "alice", Roles: roles})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodDelete, "/v1/projects/1", nil))
		fmt.Println(roles, rec.Code)
	}
	// Output:
	// [admin] 204
	// [editor] 403
}
