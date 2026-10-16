package database

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes worth translating into domain errors.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
)

// IsNoRows reports whether err means "no row matched".
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// UniqueViolation returns the violated constraint name when err is a unique
// violation. Repositories use it to return domain conflicts, e.g.:
//
//	if c, ok := database.UniqueViolation(err); ok && c == "projects_owner_name_key" {
//		return model.ErrProjectNameTaken
//	}
func UniqueViolation(err error) (constraint string, ok bool) {
	return violation(err, codeUniqueViolation)
}

// ForeignKeyViolation returns the violated constraint name, if any.
func ForeignKeyViolation(err error) (constraint string, ok bool) {
	return violation(err, codeForeignKeyViolation)
}

// CheckViolation returns the violated constraint name, if any.
func CheckViolation(err error) (constraint string, ok bool) {
	return violation(err, codeCheckViolation)
}

func violation(err error, code string) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return pgErr.ConstraintName, true
	}
	return "", false
}
