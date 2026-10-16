package apperr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
)

var errThing = apperr.NotFound("thing.not_found", "thing not found")

func TestSentinelSurvivesWrapping(t *testing.T) {
	cause := errors.New("driver: no rows")
	err := fmt.Errorf("loading: %w", errThing.Wrap(cause))

	if !errors.Is(err, errThing) {
		t.Error("errors.Is(err, sentinel) = false")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is(err, cause) = false")
	}
	if got := apperr.KindOf(err); got != apperr.KindNotFound {
		t.Errorf("KindOf = %v, want not_found", got)
	}
	if errThing.Err != nil {
		t.Error("Wrap mutated the sentinel")
	}
}

func TestWithFieldsCopies(t *testing.T) {
	base := apperr.Invalid("x.invalid", "invalid")
	withF := base.WithFields(apperr.FieldError{Field: "name", Rule: "required"})
	if len(base.Fields) != 0 || len(withF.Fields) != 1 {
		t.Fatalf("base=%v withF=%v", base.Fields, withF.Fields)
	}
	if !errors.Is(withF, base) {
		t.Error("copy with fields must still match the sentinel")
	}
}

func TestDistinctCodesDoNotMatch(t *testing.T) {
	other := apperr.NotFound("other.not_found", "other")
	if errors.Is(errThing, other) {
		t.Error("errors with different codes must not match")
	}
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		err  error
		want apperr.Kind
	}{
		{errors.New("boom"), apperr.KindInternal},
		{context.DeadlineExceeded, apperr.KindUnavailable},
		{fmt.Errorf("query: %w", context.Canceled), apperr.KindUnavailable},
		{apperr.Conflict("c", "c"), apperr.KindConflict},
		{nil, apperr.KindInternal},
	}
	for _, tt := range tests {
		if got := apperr.KindOf(tt.err); got != tt.want {
			t.Errorf("KindOf(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestErrorMessageIncludesCause(t *testing.T) {
	err := apperr.Internal(errors.New("disk full"))
	if err.Error() != "internal error: disk full" {
		t.Errorf("Error() = %q", err.Error())
	}
}
