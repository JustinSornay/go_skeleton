// Package httpx contains the HTTP plumbing shared by every controller: an
// error-returning handler signature, RFC 9457 problem responses, request
// binding/validation and JSON rendering.
//
// Handlers return errors instead of panicking or writing error responses
// themselves:
//
//	func (c *ProjectController) get(w http.ResponseWriter, r *http.Request) error {
//		owner, err := currentUser(r)
//		if err != nil {
//			return err
//		}
//		id, err := httpx.PathUUID(r, "id")
//		if err != nil {
//			return model.ErrProjectNotFound
//		}
//		p, err := c.svc.Get(r.Context(), owner, id)
//		if err != nil {
//			return err // *apperr.Error kinds become 4xx, anything else a logged 500
//		}
//		return httpx.JSON(w, http.StatusOK, dto.ToProjectResponse(p))
//	}
package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"uuid"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

// HandlerFunc is an http.HandlerFunc that reports failures by returning them.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts h to the standard library: a returned error is rendered as a
// problem response by WriteError.
func Handle(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

// JSON writes v as a JSON response with the given status. v is encoded
// before anything is written, so an encoding failure can still become a
// proper 500 problem response.
func JSON(w http.ResponseWriter, status int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("httpx: encoding response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n')) // a write error only means the client went away
	return nil
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// PathUUID parses a UUID path parameter. An unparsable value is reported as
// "not found": the resource cannot exist, and it avoids leaking ID formats.
func PathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.UUID{}, apperr.NotFound("resource.not_found", "resource not found")
	}
	return id, nil
}
