package caseconv

import (
	"iter"
	"unicode"
	"unicode/utf8"
)

// Text is the set of types the converters accept: strings, byte slices and
// named types of either.
type Text interface {
	~string | ~[]byte
}

// runeClass is the coarse classification a word boundary decision needs.
type runeClass uint8

const (
	classSep runeClass = iota
	classLower
	classUpper
	classDigit
	classOther // letters without case (CJK, ...) and joiners such as apostrophes
)

func classify(r rune) runeClass {
	switch {
	case r < utf8.RuneSelf:
		switch {
		case 'a' <= r && r <= 'z':
			return classLower
		case 'A' <= r && r <= 'Z':
			return classUpper
		case '0' <= r && r <= '9':
			return classDigit
		case r == '\'':
			return classOther
		}
		return classSep
	case unicode.IsLower(r):
		if unicode.ToUpper(r) == r {
			// Letters such as ß that are lower-case but have no single-rune
			// upper-case form never start or end a word, so a conversion
			// that cannot change them stays idempotent.
			return classOther
		}
		return classLower
	case unicode.IsUpper(r) || unicode.IsTitle(r):
		if unicode.ToLower(r) == r {
			// The mirror case: upper-case letters such as ϓ with no
			// lower-case form.
			return classOther
		}
		return classUpper
	case unicode.IsDigit(r):
		return classDigit
	case unicode.IsLetter(r) || r == '’': // right single quotation mark
		return classOther
	}
	return classSep
}

// decodeRune decodes the rune starting at s[i] without allocating, whatever
// the concrete type of s.
func decodeRune[T Text](s T, i int) (rune, int) {
	b0 := s[i]
	if b0 < utf8.RuneSelf {
		return rune(b0), 1
	}
	var buf [utf8.UTFMax]byte
	n := copy(buf[:], s[i:min(i+utf8.UTFMax, len(s))])
	return utf8.DecodeRune(buf[:n])
}

// Words splits s into its words. Boundaries are placed at separators, at
// lower→upper transitions, at letter↔digit transitions and after an acronym
// when it is followed by a capitalised word. Separator runs are dropped, so
// the result never contains an empty word.
//
//	Words("JSONData v2.1") // ["JSON", "Data", "v", "2", "1"]
func Words[T Text](s T) []T {
	var out []T
	for word := range Split(s) {
		out = append(out, word)
	}
	return out
}

// Split is the streaming form of [Words]: it yields each word of s in order
// as a sub-slice of s, without allocating.
func Split[T Text](s T) iter.Seq[T] {
	return func(yield func(T) bool) {
		start := -1
		prev := classSep
		for i := 0; i < len(s); {
			r, size := decodeRune(s, i)
			cur := classify(r)
			if cur == classSep {
				if start >= 0 && !yield(s[start:i]) {
					return
				}
				start = -1
				prev = classSep
				i += size
				continue
			}
			if start < 0 {
				start = i
			} else if boundary(prev, cur) || (prev == classUpper && cur == classUpper && startsLowerAfter(s, i+size)) {
				if !yield(s[start:i]) {
					return
				}
				start = i
			}
			prev = cur
			i += size
		}
		if start >= 0 {
			yield(s[start:])
		}
	}
}

// boundary reports whether a word ends between two adjacent runes of classes
// prev and cur, ignoring the acronym rule (which needs one more rune of
// lookahead and is handled by the caller).
func boundary(prev, cur runeClass) bool {
	switch {
	case prev == classLower && cur == classUpper:
		return true
	case prev == classDigit && cur != classDigit:
		return true
	case cur == classDigit && prev != classDigit:
		return true
	}
	return false
}

// startsLowerAfter reports whether the rune at s[i] is lower-case. It is the
// lookahead behind the acronym rule: in "JSONData" the "D" begins a new word
// because it is an upper-case rune followed by a lower-case one.
func startsLowerAfter[T Text](s T, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := decodeRune(s, i)
	return classify(r) == classLower
}
