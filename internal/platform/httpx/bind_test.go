package httpx_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
)

type address struct {
	City string `json:"city" validate:"required"`
}

type createThing struct {
	Name    string   `json:"name" validate:"required,max=5"`
	Kind    string   `json:"kind" validate:"omitempty,oneof=a b"`
	Address *address `json:"address" validate:"omitnil"`
}

func post(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindInvalid {
		t.Fatalf("want invalid apperr, got %v", err)
	}
	out := map[string]string{}
	for _, f := range ae.Fields {
		out[f.Field] = f.Rule
	}
	return out
}

func TestBindValid(t *testing.T) {
	got, err := httpx.Bind[createThing](post(`{"name":"abc","kind":"a"}`))
	if err != nil || got.Name != "abc" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestBindValidationFields(t *testing.T) {
	_, err := httpx.Bind[createThing](post(`{"name":"too long","kind":"z","address":{}}`))
	fields := fieldsOf(t, err)
	want := map[string]string{"name": "max", "kind": "oneof", "address.city": "required"}
	for f, rule := range want {
		if fields[f] != rule {
			t.Errorf("field %q: rule %q, want %q (all: %v)", f, fields[f], rule, fields)
		}
	}
}

func TestDecodeJSONRejects(t *testing.T) {
	tests := map[string]string{
		"unknown field": `{"name":"a","title":"unexpected"}`,
		"wrong type":    `{"name":42}`,
		"two values":    `{"name":"a"} {"name":"b"}`,
		"not json":      `name=a`,
		"empty":         ``,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := httpx.DecodeJSON[createThing](post(body))
			if apperr.KindOf(err) != apperr.KindInvalid {
				t.Fatalf("want invalid, got %v", err)
			}
		})
	}
}

func TestDecodeJSONTooLarge(t *testing.T) {
	req := post(`{"name":"` + strings.Repeat("a", 100) + `"}`)
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 10)
	_, err := httpx.DecodeJSON[createThing](req)
	if apperr.KindOf(err) != apperr.KindTooLarge {
		t.Fatalf("want too_large, got %v", err)
	}
}

type listThings struct {
	httpx.PageQuery
	Q      string   `query:"q" validate:"max=3"`
	Active *bool    `query:"active"`
	Min    *float64 `query:"min"`
	Tags   []string `query:"tag"`
	Count  uint8    `query:"count"`
}

func TestBindQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?q=ab&active=true&min=1.5&tag=x,y&tag=z&limit=5&offset=10&count=7", nil)
	var q listThings
	if err := httpx.BindQuery(req, &q); err != nil {
		t.Fatal(err)
	}
	if q.Q != "ab" || q.Active == nil || !*q.Active || q.Min == nil || *q.Min != 1.5 || q.Count != 7 {
		t.Errorf("unexpected %+v", q)
	}
	if strings.Join(q.Tags, ",") != "x,y,z" || q.Limit != 5 || q.Offset != 10 {
		t.Errorf("unexpected %+v", q)
	}
}

func TestBindQueryAbsentPointersStayNil(t *testing.T) {
	var q listThings
	if err := httpx.BindQuery(httptest.NewRequest(http.MethodGet, "/?active=", nil), &q); err != nil {
		t.Fatal(err)
	}
	if q.Active != nil || q.Min != nil {
		t.Errorf("unexpected %+v", q)
	}
}

func TestBindQueryErrors(t *testing.T) {
	var q listThings
	err := httpx.BindQuery(httptest.NewRequest(http.MethodGet, "/?limit=abc&active=maybe&count=300&q=x&q=y", nil), &q)
	fields := fieldsOf(t, err)
	for _, f := range []string{"limit", "active", "count", "q"} {
		if fields[f] != "type" {
			t.Errorf("field %q not reported (all: %v)", f, fields)
		}
	}

	err = httpx.BindQuery(httptest.NewRequest(http.MethodGet, "/?q=toolong&limit=0", nil), &q)
	fields = fieldsOf(t, err)
	if fields["q"] != "max" {
		t.Errorf("want q:max, got %v", fields)
	}
}

func TestBindQueryEmbeddedValidationUsesParamName(t *testing.T) {
	var q listThings
	err := httpx.BindQuery(httptest.NewRequest(http.MethodGet, "/?limit=500", nil), &q)
	if fields := fieldsOf(t, err); fields["limit"] != "max" {
		t.Errorf("want limit:max, got %v", fields)
	}
}

func TestBindQueryRejectsNonStruct(t *testing.T) {
	var n int
	err := httpx.BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), &n)
	if ae, ok := apperr.As(err); !ok || ae.Kind != apperr.KindInternal || errors.Unwrap(err) == nil {
		t.Fatalf("want internal error, got %v", err)
	}
}
