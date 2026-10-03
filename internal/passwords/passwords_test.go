package passwords

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	for _, c := range []struct{ in, want int }{{16, 16}, {1, MinLength}, {-5, MinLength}, {500, MaxLength}} {
		pw, err := Generate(c.in, false)
		if err != nil || len(pw) != c.want {
			t.Errorf("Generate(%d) = %q (len %d), %v; want len %d", c.in, pw, len(pw), err, c.want)
		}
		if strings.ContainsAny(pw, symbols) {
			t.Errorf("Generate(%d, false) contained symbols: %q", c.in, pw)
		}
	}

	// With symbols on, a long password should practically always include one.
	pw, _ := Generate(MaxLength, true)
	if !strings.ContainsAny(pw, symbols) {
		t.Errorf("Generate(%d, true) had no symbols: %q", MaxLength, pw)
	}
}
