package set

// Set is a set of comparable keys.
type Set[T comparable] map[T]struct{}

func New[T comparable](opts ...optFunc) Set[T] {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.capacity > 0 {
		return make(Set[T], o.capacity)
	}
	return make(Set[T])
}

// Add adds a key to the set.
// If the key is already in the set, it is a no-op.
func (s Set[T]) Add(key T) {
	s[key] = struct{}{}
}

// Remove removes a key from the set.
// If the key is not in the set, it is a no-op.
func (s Set[T]) Remove(key T) {
	delete(s, key)
}

// Clear clears the set.
func (s Set[T]) Clear() {
	clear(s)
}

// Contains checks if a key is in the set.
func (s Set[T]) Contains(key T) bool {
	_, ok := s[key]
	return ok
}

// Missing checks if a key is not in the set.
func (s Set[T]) Missing(key T) bool {
	return !s.Contains(key)
}
