package strutils_test

import (
	"testing"

	"github.com/omniaura/go-kit/strutils"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "Hello World", want: "hello-world"},
		{name: "collapses separators", input: " hello---world__again.now ", want: "hello-world-again-now"},
		{name: "drops unsupported runes", input: "Caf\u00e9 \u2603", want: "caf"},
		{name: "trims dashes", input: "---hello---", want: "hello"},
		{name: "empty", input: " \u2603 ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strutils.Slugify(tt.input); got != tt.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlugifyMax(t *testing.T) {
	if got := strutils.SlugifyMax("Hello Big World", 9); got != "hello-big" {
		t.Fatalf("SlugifyMax() = %q", got)
	}
	if got := strutils.SlugifyMax("Hello Big World", 0); got != "hello-big-world" {
		t.Fatalf("SlugifyMax(no cap) = %q", got)
	}
	if got := strutils.SlugifyMax("Hello Big World", 6); got != "hello" {
		t.Fatalf("SlugifyMax(trim dash) = %q", got)
	}
}
