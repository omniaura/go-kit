package strutils

import "strings"

// Slugify lowercases s and reduces it to the [a-z0-9-] character set,
// collapsing spaces, dashes, underscores, and dots into single dashes.
func Slugify(s string) string {
	var b strings.Builder
	lastHyphen := false

	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !lastHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}

	return strings.TrimRight(b.String(), "-")
}

// SlugifyMax is Slugify followed by a byte-length cap. A non-positive max
// disables the cap.
func SlugifyMax(s string, maxBytes int) string {
	out := Slugify(s)
	if maxBytes > 0 && len(out) > maxBytes {
		out = strings.Trim(out[:maxBytes], "-")
	}
	return out
}
