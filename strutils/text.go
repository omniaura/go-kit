package strutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const previewMaxBytes = 240

// FirstNonBlank returns the first value whose trimmed form is not empty.
// The original, untrimmed value is returned.
func FirstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// FirstTrimmedNonBlank returns the first non-blank value after trimming space.
func FirstTrimmedNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// ContainsFold reports whether items contains needle under Unicode case folding.
func ContainsFold[S ~string](items []S, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(string(item), needle) {
			return true
		}
	}
	return false
}

// QuotedPreview returns a JSON-quoted preview of raw, truncated to a bounded
// byte prefix before quoting.
func QuotedPreview(raw string) string {
	preview := TruncateUTF8(raw, previewMaxBytes, "...")
	data, err := json.Marshal(preview)
	if err != nil {
		return preview
	}
	return string(data)
}

// TypedPreview returns the Go type of v followed by a compact JSON preview.
// If v cannot be marshaled as JSON, only the Go type is returned.
func TypedPreview(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T", v)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return fmt.Sprintf("%T", v)
	}

	return fmt.Sprintf("%T %s", v, TruncateUTF8(compact.String(), previewMaxBytes, "..."))
}

// TruncateEllipsis truncates text to at most maxBytes bytes, using "..." when
// there is room for the full marker.
func TruncateEllipsis(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	if maxBytes <= 0 {
		return ""
	}
	if maxBytes <= 3 {
		return strings.Repeat(".", maxBytes)
	}
	return TruncateUTF8(text, maxBytes-3, "...")
}

// TrimMax trims surrounding whitespace, then truncates the result to at most
// maxBytes bytes while preserving UTF-8 validity.
func TrimMax(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	return TruncateUTF8(value, maxBytes, "")
}

// MiddleTruncateUTF8 truncates s to fit within maxBytes by preserving the
// beginning and end of the string and replacing the middle with a marker.
func MiddleTruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 0 {
		return ""
	}

	approxTruncatedBytes := max(len(s)-maxBytes, 1)
	marker := "[cut]"
	if maxBytes >= len(cutTruncationMarker(approxTruncatedBytes))+4 {
		estimatedCutBytes := len(s) - maxBytes + len(cutTruncationMarker(approxTruncatedBytes))
		marker = cutTruncationMarker(estimatedCutBytes)
	}

	contextBudget := maxBytes - len(marker)
	if contextBudget < 0 {
		if maxBytes >= 3 {
			return "..."
		}
		return strings.Repeat(".", maxBytes)
	}

	prefix, suffix := middleTruncateSegments(s, contextBudget)
	return prefix + marker + suffix
}

func middleTruncateSegments(s string, totalBudget int) (string, string) {
	if totalBudget <= 0 {
		return "", ""
	}

	prefixBudget := totalBudget / 2
	suffixBudget := totalBudget - prefixBudget

	prefix := s[:prefixBudget]
	for len(prefix) > 0 && !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}

	suffixStart := max(len(s)-suffixBudget, len(prefix))
	for suffixStart < len(s) && !utf8.RuneStart(s[suffixStart]) {
		suffixStart++
	}

	return prefix, s[suffixStart:]
}

func formatTruncatedBytes(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%db", n)
	case n < 1000000:
		return formatTruncatedUnit(float64(n)/1000, "kb")
	default:
		return formatTruncatedUnit(float64(n)/1000000, "mb")
	}
}

func formatTruncatedUnit(value float64, unit string) string {
	formatted := fmt.Sprintf("%.1f", value)
	formatted = strings.TrimSuffix(formatted, ".0")
	return formatted + unit
}

func cutTruncationMarker(n int) string {
	return fmt.Sprintf("[%s cut]", formatTruncatedBytes(n))
}

// TruncateUTF8 truncates s to a valid UTF-8 prefix of at most maxBytes bytes,
// then appends suffix when truncation occurs.
func TruncateUTF8(s string, maxBytes int, suffix string) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 0 {
		return suffix
	}

	truncated := s[:maxBytes]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + suffix
}
