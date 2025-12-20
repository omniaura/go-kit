package deref

import "time"

// Str dereferences a string pointer.
// If the pointer is nil, it returns an empty string.
func Str[S ~string](s *S) S {
	if s == nil {
		return ""
	}
	return *s
}

// Bool dereferences a bool pointer.
// If the pointer is nil, it returns false.
func Bool[B ~bool](b *B) B {
	if b == nil {
		return false
	}
	return *b
}

// Int64 dereferences an int64 pointer.
// If the pointer is nil, it returns 0.
func Int64[I ~int64 | ~int](i *I) I {
	if i == nil {
		return 0
	}
	return *i
}

// Time dereferences a time.Time pointer.
// If the pointer is nil, it returns a zero time.Time.
func Time(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
