package requestid_test

import (
	"strings"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"":                                     false,
		"abc-123":                              true,
		"01890a5d-ac96-774b-bcce-b302099a8057": true,
		"trace:span.1_x":                       true,
		"has space":                            false,
		"line\nbreak":                          false,
		"<script>":                             false,
		strings.Repeat("a", 129):               false,
	}
	for in, want := range cases {
		if got := requestid.Valid(in); got != want {
			t.Errorf("Valid(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewIsValidAndUnique(t *testing.T) {
	a, b := requestid.New(), requestid.New()
	if !requestid.Valid(a) || a == b {
		t.Fatalf("unexpected ids %q %q", a, b)
	}
}
