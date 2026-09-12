package caseconv

import (
	"fmt"
	"strings"
)

// Case names a naming convention so callers can pick the target at run time.
// The zero value is [Unknown], which leaves input untouched.
type Case uint8

const (
	Unknown        Case = iota
	Snake               // snake_case
	ScreamingSnake      // SCREAMING_SNAKE_CASE
	Kebab               // kebab-case
	ScreamingKebab      // SCREAMING-KEBAB-CASE
	Camel               // CamelCase
	LowerCamel          // lowerCamelCase
	Title               // Title Case
	Sentence            // Sentence case
)

var caseNames = [...]string{
	Unknown:        "unknown",
	Snake:          "snake",
	ScreamingSnake: "screaming_snake",
	Kebab:          "kebab",
	ScreamingKebab: "screaming_kebab",
	Camel:          "camel",
	LowerCamel:     "lower_camel",
	Title:          "title",
	Sentence:       "sentence",
}

// Cases lists every named convention, in declaration order.
func Cases() []Case {
	out := make([]Case, 0, len(caseNames)-1)
	for c := Snake; int(c) < len(caseNames); c++ {
		out = append(out, c)
	}
	return out
}

// ParseCase resolves a convention from its name, accepting the names
// returned by [Case.String] in any letter case and with "-" or " " in place
// of "_" (so "screaming-snake", "Screaming Snake" and "SCREAMING_SNAKE" all
// resolve to [ScreamingSnake]).
func ParseCase(name string) (Case, error) {
	want := ToSnake(strings.TrimSpace(name))
	for c, n := range caseNames {
		if Case(c) != Unknown && n == want {
			return Case(c), nil
		}
	}
	return Unknown, fmt.Errorf("caseconv: unknown case %q", name)
}

// String returns the snake_case name of the convention.
func (c Case) String() string {
	if int(c) < len(caseNames) {
		return caseNames[c]
	}
	return fmt.Sprintf("Case(%d)", uint8(c))
}

// MarshalText implements [encoding.TextMarshaler] using [Case.String].
func (c Case) MarshalText() ([]byte, error) {
	if int(c) >= len(caseNames) {
		return nil, fmt.Errorf("caseconv: invalid case %d", uint8(c))
	}
	return []byte(c.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] using [ParseCase].
func (c *Case) UnmarshalText(text []byte) error {
	parsed, err := ParseCase(string(text))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// Convert rewrites s in the convention c. [Unknown] and out-of-range values
// return s unchanged.
func (c Case) Convert(s string) string {
	return Convert(c, s)
}

// ConvertBytes is [Case.Convert] for byte slices.
func (c Case) ConvertBytes(s []byte) []byte {
	return Convert(c, s)
}

// Convert rewrites s in the convention c. [Unknown] and out-of-range values
// return s unchanged.
func Convert[T Text](c Case, s T) T {
	switch c {
	case Snake:
		return ToSnake(s)
	case ScreamingSnake:
		return ToScreamingSnake(s)
	case Kebab:
		return ToKebab(s)
	case ScreamingKebab:
		return ToScreamingKebab(s)
	case Camel:
		return ToCamel(s)
	case LowerCamel:
		return ToLowerCamel(s)
	case Title:
		return ToTitle(s)
	case Sentence:
		return ToSentence(s)
	}
	return s
}
