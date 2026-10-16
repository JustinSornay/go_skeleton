// Package apperr defines the application error model shared by every layer.
//
// Domain and service code return *Error values (usually package-level
// sentinels) that carry a Kind. Only the HTTP layer (utils/httpx) maps a
// Kind to a status code, so business code never imports net/http and error
// classification never depends on message text.
package apperr

import (
	"context"
	"errors"
)

// Kind classifies an error independently of any transport.
type Kind uint8

const (
	KindInternal        Kind = iota // unexpected failure; details are never exposed
	KindInvalid                     // the input is malformed or breaks a rule
	KindUnauthenticated             // no valid credentials
	KindForbidden                   // authenticated but not allowed
	KindNotFound                    // the resource does not exist (or is not visible)
	KindConflict                    // the request conflicts with the current state
	KindTooLarge                    // the request body exceeds the allowed size
	KindRateLimited                 // too many requests
	KindUnavailable                 // a dependency is down or timed out; retry later
)

// String returns a stable, human-readable name for the kind.
func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindTooLarge:
		return "too_large"
	case KindRateLimited:
		return "rate_limited"
	case KindUnavailable:
		return "unavailable"
	default:
		return "internal"
	}
}

// FieldError describes a validation failure on a single input field.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Error is the application error type.
//
// Code and Message are safe to return to clients. Err is the underlying cause:
// it is logged but never serialized.
type Error struct {
	Kind    Kind
	Code    string // stable machine-readable identifier, e.g. "project.not_found"
	Message string // human-readable, client-safe
	Fields  []FieldError
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// Is matches two *Error values by Kind and Code so that a sentinel still
// matches after being copied with Wrap or WithFields.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Kind == t.Kind && e.Code == t.Code
}

// New returns an error of the given kind.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Wrap returns a copy of e that records cause as the underlying error.
// errors.Is(result, e) and errors.Is(result, cause) both hold.
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.Err = cause
	return &c
}

// WithFields returns a copy of e carrying field-level validation details.
func (e *Error) WithFields(fields ...FieldError) *Error {
	c := *e
	c.Fields = append(append([]FieldError(nil), e.Fields...), fields...)
	return &c
}

// Convenience constructors for the most common kinds.

func Invalid(code, message string, fields ...FieldError) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: message, Fields: fields}
}

func NotFound(code, message string) *Error { return New(KindNotFound, code, message) }
func Conflict(code, message string) *Error { return New(KindConflict, code, message) }
func Forbidden(code, message string) *Error {
	return New(KindForbidden, code, message)
}

func Unauthenticated(code, message string) *Error {
	return New(KindUnauthenticated, code, message)
}

func Unavailable(code, message string, cause error) *Error {
	return &Error{Kind: KindUnavailable, Code: code, Message: message, Err: cause}
}

// Internal wraps an unexpected error. The cause is kept for logs only.
func Internal(cause error) *Error {
	return &Error{Kind: KindInternal, Code: "internal", Message: "internal error", Err: cause}
}

// As returns the first *Error in err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// KindOf classifies any error. Context cancellation and deadlines are mapped
// to KindUnavailable; anything unknown is KindInternal.
func KindOf(err error) Kind {
	if err == nil {
		return KindInternal
	}
	if e, ok := As(err); ok {
		return e.Kind
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return KindUnavailable
	}
	return KindInternal
}
