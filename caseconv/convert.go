package caseconv

import (
	"unicode"
	"unicode/utf8"
)

// wordCase is how a converter rewrites the letters of one word.
type wordCase uint8

const (
	keepCase  wordCase = iota // leave the word as written
	lowerCase                 // every letter lower-case
	upperCase                 // every letter upper-case
	capCase                   // first letter upper-case, the rest lower-case
	capKeep                   // first letter upper-case, the rest as written
)

// ToSnake converts s to snake_case.
func ToSnake[T Text](s T) T {
	return join(s, "_", lowerCase, lowerCase)
}

// ToScreamingSnake converts s to SCREAMING_SNAKE_CASE.
func ToScreamingSnake[T Text](s T) T {
	return join(s, "_", upperCase, upperCase)
}

// ToKebab converts s to kebab-case.
func ToKebab[T Text](s T) T {
	return join(s, "-", lowerCase, lowerCase)
}

// ToScreamingKebab converts s to SCREAMING-KEBAB-CASE.
func ToScreamingKebab[T Text](s T) T {
	return join(s, "-", upperCase, upperCase)
}

// ToDelimited converts s to lower-case words joined by delimiter, for
// example "any.kind.of.string" with delimiter ".".
func ToDelimited[T Text](s T, delimiter string) T {
	return join(s, delimiter, lowerCase, lowerCase)
}

// ToScreamingDelimited converts s to upper-case words joined by delimiter,
// for example "ANY.KIND.OF.STRING" with delimiter ".".
func ToScreamingDelimited[T Text](s T, delimiter string) T {
	return join(s, delimiter, upperCase, upperCase)
}

// ToCamel converts s to CamelCase (upper camel case, also called
// PascalCase). Acronyms are normalised: "userID" becomes "UserId".
func ToCamel[T Text](s T) T {
	return join(s, "", capCase, capCase)
}

// ToLowerCamel converts s to lowerCamelCase.
func ToLowerCamel[T Text](s T) T {
	return join(s, "", lowerCase, capCase)
}

// ToTitle converts s to Title Case: words separated by single spaces, each
// starting with an upper-case letter. Letters after the first are left as
// written, so acronyms survive ("API-reference" → "API Reference") and
// mixed-case words keep their spelling.
func ToTitle[T Text](s T) T {
	return join(s, " ", capKeep, capKeep)
}

// ToSentence converts s to Sentence case: words separated by single spaces,
// with only the first word capitalised. Letters after the first are left as
// written.
func ToSentence[T Text](s T) T {
	return join(s, " ", capKeep, keepCase)
}

// join rewrites each word of s with the given cases (the first word may
// differ from the rest) and concatenates them with delimiter.
func join[T Text](s T, delimiter string, first, rest wordCase) T {
	if len(s) == 0 {
		return s
	}
	out := make([]byte, 0, len(s)+2*len(delimiter))
	wc := first
	for word := range Split(s) {
		if len(out) > 0 {
			out = append(out, delimiter...)
		}
		out = appendWord(out, word, wc)
		wc = rest
	}
	return T(out)
}

func appendWord[T Text](out []byte, word T, wc wordCase) []byte {
	if wc == keepCase {
		return append(out, word...)
	}
	for i := 0; i < len(word); {
		r, size := decodeRune(word, i)
		switch {
		case wc == upperCase && r == 'ß':
			// ß has no single-rune upper-case form; SpecialCasing maps it
			// to "SS", which unicode.ToUpper cannot express.
			out = append(out, "SS"...)
			i += size
			continue
		case wc == upperCase, i == 0 && (wc == capCase || wc == capKeep):
			r = unicode.ToUpper(r)
		case wc == lowerCase, wc == capCase:
			r = unicode.ToLower(r)
		}
		out = utf8.AppendRune(out, r)
		i += size
	}
	return out
}
