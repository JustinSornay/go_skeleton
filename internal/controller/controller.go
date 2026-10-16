// Package controller exposes the use cases over HTTP: each controller parses
// and validates the request (into internal/dto types), calls its service and
// writes the response. Handlers return errors, wrapped with httpx.Handle;
// they never write error responses by hand. Routes are mounted by
// internal/app, which also declares the access policy.
package controller

import (
	"net/http"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

// currentUser returns the authenticated subject. RequireAuth guarantees it
// in production; the check keeps a controller safe if it is ever mounted
// without that middleware.
func currentUser(r *http.Request) (string, error) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		return "", apperr.Unauthenticated("auth.required", "authentication is required")
	}
	return p.Subject, nil
}
