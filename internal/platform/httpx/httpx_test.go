package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

func serve(t *testing.T, h httpx.HandlerFunc, req *http.Request) (*httptest.ResponseRecorder, httpx.Problem) {
	t.Helper()
	rec := httptest.NewRecorder()
	httpx.Handle(h).ServeHTTP(rec, req)
	var p httpx.Problem
	if strings.HasPrefix(rec.Header().Get("Content-Type"), httpx.ProblemContentType) {
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatalf("decoding problem: %v", err)
		}
	}
	return rec, p
}

func TestWriteErrorMapping(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{apperr.NotFound("x.not_found", "x not found"), 404, "x.not_found", "x not found"},
		{fmt.Errorf("wrapped: %w", apperr.Conflict("x.conflict", "taken")), 409, "x.conflict", "taken"},
		{apperr.Invalid("x.invalid", "bad"), 400, "x.invalid", "bad"},
		{apperr.Forbidden("x.forbidden", "no"), 403, "x.forbidden", "no"},
		{context.DeadlineExceeded, 503, "unavailable", "the request could not be completed in time"},
		// Unknown errors never leak their message.
		{errors.New("pq: password authentication failed for user admin"), 500, "internal", "internal error"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/things/1?secret=x", nil)
		req = req.WithContext(requestid.With(req.Context(), "req-1"))
		rec, p := serve(t, func(http.ResponseWriter, *http.Request) error { return tt.err }, req)

		if rec.Code != tt.wantStatus || p.Status != tt.wantStatus {
			t.Errorf("%v: status = %d/%d, want %d", tt.err, rec.Code, p.Status, tt.wantStatus)
		}
		if p.Code != tt.wantCode || p.Detail != tt.wantDetail {
			t.Errorf("%v: code/detail = %q/%q", tt.err, p.Code, p.Detail)
		}
		if p.RequestID != "req-1" || p.Instance != "/things/1" || p.Type != "about:blank" {
			t.Errorf("%v: unexpected problem %+v", tt.err, p)
		}
	}
}

func TestUnauthenticatedSetsChallenge(t *testing.T) {
	rec, _ := serve(t, func(http.ResponseWriter, *http.Request) error {
		return apperr.Unauthenticated("auth.required", "login")
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("status %d, challenge %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

func TestJSONSuccess(t *testing.T) {
	rec, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) error {
		return httpx.JSON(w, http.StatusCreated, map[string]string{"ok": "yes"})
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "application/json" || !strings.Contains(rec.Body.String(), `"ok":"yes"`) {
		t.Fatalf("got %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestRouterMisses(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.MethodNotAllowed(rec, httptest.NewRequest(http.MethodPut, "/x", nil))
	if rec.Code != 405 || rec.Header().Get("Content-Type") != httpx.ProblemContentType {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	httpx.NotFound(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != 404 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestPageQueryNormalize(t *testing.T) {
	tests := []struct{ in, want httpx.PageQuery }{
		{httpx.PageQuery{}, httpx.PageQuery{Limit: 20}},
		{httpx.PageQuery{Limit: 500, Offset: -3}, httpx.PageQuery{Limit: 100}},
		{httpx.PageQuery{Limit: 5, Offset: 10}, httpx.PageQuery{Limit: 5, Offset: 10}},
	}
	for _, tt := range tests {
		if got := tt.in.Normalize(); got != tt.want {
			t.Errorf("Normalize(%+v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestJSONEncodingFailureBecomesProblem(t *testing.T) {
	rec, p := serve(t, func(w http.ResponseWriter, _ *http.Request) error {
		return httpx.JSON(w, http.StatusOK, map[string]any{"bad": make(chan int)})
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError || p.Code != "internal" {
		t.Fatalf("got %d %+v", rec.Code, p)
	}
}
