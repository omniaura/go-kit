package strutils_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/omniaura/go-kit/strutils"
)

func TestFirstNonBlank(t *testing.T) {
	if got := strutils.FirstNonBlank("", " \t", "  value  ", "fallback"); got != "  value  " {
		t.Fatalf("FirstNonBlank() = %q", got)
	}
	if got := strutils.FirstNonBlank("", " "); got != "" {
		t.Fatalf("FirstNonBlank(all blank) = %q", got)
	}
}

func TestFirstTrimmedNonBlank(t *testing.T) {
	if got := strutils.FirstTrimmedNonBlank("", " \t", "  value  ", "fallback"); got != "value" {
		t.Fatalf("FirstTrimmedNonBlank() = %q", got)
	}
}

func TestContainsFold(t *testing.T) {
	type label string
	items := []label{"Alpha", "beta"}
	if !strutils.ContainsFold(items, "ALPHA") {
		t.Fatal("ContainsFold() did not match case-insensitively")
	}
	if strutils.ContainsFold(items, "gamma") {
		t.Fatal("ContainsFold() matched missing item")
	}
}

func TestQuotedPreview(t *testing.T) {
	got := strutils.QuotedPreview("hello\nworld")
	want := `"hello\nworld"`
	if got != want {
		t.Fatalf("QuotedPreview() = %q, want %q", got, want)
	}

	long := strings.Repeat("a", 241)
	got = strutils.QuotedPreview(long)
	want = `"` + strings.Repeat("a", 240) + `..."`
	if got != want {
		t.Fatalf("QuotedPreview(long) = %q, want %q", got, want)
	}
}

func TestTypedPreview(t *testing.T) {
	got := strutils.TypedPreview(map[string]any{"name": "ditto", "ok": true})
	if !strings.HasPrefix(got, "map[string]interface {} ") {
		t.Fatalf("TypedPreview() type prefix = %q", got)
	}
	if !strings.Contains(got, `"name":"ditto"`) || !strings.Contains(got, `"ok":true`) {
		t.Fatalf("TypedPreview() compact JSON = %q", got)
	}

	unmarshalable := func() {}
	if got := strutils.TypedPreview(unmarshalable); got != "func()" {
		t.Fatalf("TypedPreview(unmarshalable) = %q", got)
	}
}

func TestTruncateEllipsis(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{name: "no truncation", input: "hello", maxBytes: 10, want: "hello"},
		{name: "ascii", input: "hello world", maxBytes: 8, want: "hello..."},
		{name: "tiny", input: "hello", maxBytes: 2, want: ".."},
		{name: "zero", input: "hello", maxBytes: 0, want: ""},
		{name: "utf8", input: "a\u00e9bcd", maxBytes: 5, want: "a..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strutils.TruncateEllipsis(tt.input, tt.maxBytes)
			if got != tt.want {
				t.Fatalf("TruncateEllipsis(%q, %d) = %q, want %q", tt.input, tt.maxBytes, got, tt.want)
			}
			if len(got) > tt.maxBytes {
				t.Fatalf("TruncateEllipsis() length = %d, max = %d", len(got), tt.maxBytes)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateEllipsis() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func TestTrimMax(t *testing.T) {
	got := strutils.TrimMax("  a\u00e9b  ", 2)
	if got != "a" {
		t.Fatalf("TrimMax() = %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("TrimMax() returned invalid UTF-8: %q", got)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		suffix   string
		want     string
	}{
		{name: "no truncation", input: "hello", maxBytes: 10, suffix: "...", want: "hello"},
		{name: "exact size", input: "hello", maxBytes: 5, suffix: "...", want: "hello"},
		{name: "ascii with suffix", input: "hello world", maxBytes: 5, suffix: "...", want: "hello..."},
		{name: "utf8 boundary", input: "a\u00e9b", maxBytes: 2, suffix: "", want: "a"},
		{name: "utf8 exact", input: "a\u00e9b", maxBytes: 3, suffix: "", want: "a\u00e9"},
		{name: "zero returns suffix", input: "hello", maxBytes: 0, suffix: "...", want: "..."},
		{name: "negative returns suffix", input: "hello", maxBytes: -1, suffix: "...", want: "..."},
		{name: "cjk", input: "\u4e16\u754c", maxBytes: 3, suffix: "", want: "\u4e16"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strutils.TruncateUTF8(tt.input, tt.maxBytes, tt.suffix)
			if got != tt.want {
				t.Fatalf("TruncateUTF8(%q, %d, %q) = %q, want %q", tt.input, tt.maxBytes, tt.suffix, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateUTF8() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func TestMiddleTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{name: "no truncation", input: "hello", maxBytes: 10, want: "hello"},
		{name: "exact size", input: "hello", maxBytes: 5, want: "hello"},
		{name: "uses byte marker", input: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", maxBytes: 32, want: "abcdefghijk[29b cut]OPQRSTUVWXYZ"},
		{name: "middle marker", input: "abcdefghijklmnopqrstuvwxyz", maxBytes: 20, want: "abcde[14b cut]uvwxyz"},
		{name: "small budget", input: "abcdefghij", maxBytes: 8, want: "a[cut]ij"},
		{name: "zero", input: "abcdefghij", maxBytes: 0, want: ""},
		{name: "two byte budget", input: "abcdefghij", maxBytes: 2, want: ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strutils.MiddleTruncateUTF8(tt.input, tt.maxBytes)
			if got != tt.want {
				t.Fatalf("MiddleTruncateUTF8(%q, %d) = %q, want %q", tt.input, tt.maxBytes, got, tt.want)
			}
			if len(got) > tt.maxBytes {
				t.Fatalf("MiddleTruncateUTF8() length = %d, max = %d", len(got), tt.maxBytes)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("MiddleTruncateUTF8() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func TestMiddleTruncateUTF8PreservesUTF8Boundaries(t *testing.T) {
	input := "你好，this is a long UTF-8 string，再见"
	got := strutils.MiddleTruncateUTF8(input, 24)

	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8, got %q", got)
	}
	if len(got) > 24 {
		t.Fatalf("MiddleTruncateUTF8() length = %d, max = 24", len(got))
	}
	if !strings.Contains(got, "[") || !strings.Contains(got, "cut") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func TestTypedPreviewTruncates(t *testing.T) {
	got := strutils.TypedPreview([]string{strings.Repeat("x", 300)})
	if !strings.Contains(got, "...") {
		t.Fatalf("TypedPreview() did not truncate: %q", got)
	}
}
