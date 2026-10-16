package apperr_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

// Business errors are package-level sentinels declared once, next to the
// entity (see internal/model). The Code is a public, stable identifier:
// never rename an existing one.
var errInvoiceNotFound = apperr.NotFound("invoice.not_found", "invoice not found")

func findInvoice(id string) error {
	// Wrap keeps the low-level cause for the logs; clients only ever see
	// the sentinel's Code and Message.
	return errInvoiceNotFound.Wrap(fmt.Errorf("no row for id %s", id))
}

// Example defines a sentinel error and checks it after wrapping, with
// errors.Is (identity: Kind and Code) and KindOf (classification).
func Example() {
	err := fmt.Errorf("paying invoice: %w", findInvoice("42"))

	fmt.Println(errors.Is(err, errInvoiceNotFound))
	fmt.Println(apperr.KindOf(err))
	if ae, ok := apperr.As(err); ok {
		fmt.Println(ae.Code, "|", ae.Message)
	}
	fmt.Println(err)
	// Output:
	// true
	// not_found
	// invoice.not_found | invoice not found
	// paying invoice: invoice not found: no row for id 42
}

// ExampleError_WithFields attaches field-level details to a validation
// sentinel; the copy still matches the sentinel.
func ExampleError_WithFields() {
	errInvoiceInvalid := apperr.Invalid("invoice.invalid", "invoice is invalid")
	err := errInvoiceInvalid.WithFields(apperr.FieldError{Field: "amount", Rule: "min", Message: "must be positive"})

	fmt.Println(errors.Is(err, errInvoiceInvalid), err.Fields[0].Field)
	// Output:
	// true amount
}

// ExampleKindOf classifies any error: context deadlines become
// KindUnavailable (503) and unknown errors KindInternal (opaque 500).
func ExampleKindOf() {
	fmt.Println(apperr.KindOf(apperr.Conflict("invoice.paid", "invoice already paid")))
	fmt.Println(apperr.KindOf(fmt.Errorf("querying: %w", context.DeadlineExceeded)))
	fmt.Println(apperr.KindOf(errors.New("connection reset by peer")))
	// Output:
	// conflict
	// unavailable
	// internal
}
