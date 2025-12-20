package slicer

// Last returns the last element of a slice.
// If the slice is empty, it returns a zero value of the type.
func Last[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	return slice[len(slice)-1]
}

// LastPtr returns a pointer to the last element of a slice.
// If the slice is empty, it returns a pointer to a zero value of the type.
func LastPtr[T any](slice []T) *T {
	if len(slice) == 0 {
		var zero T
		return &zero
	}
	return &slice[len(slice)-1]
}

// First returns the first element of a slice.
// If the slice is empty, it returns a zero value of the type.
func First[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	return slice[0]
}

// FirstPtr returns a pointer to the first element of a slice.
// If the slice is empty, it returns a pointer to a zero value of the type.
func FirstPtr[T any](slice []T) *T {
	if len(slice) == 0 {
		var zero T
		return &zero
	}
	return &slice[0]
}

// Map runs a function over a slice and returns a new slice.
func Map[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}
