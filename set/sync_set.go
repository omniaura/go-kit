package set

import (
	"iter"
	"sync"
)

// SyncSet is a thread-safe set of comparable keys.
type SyncSet[T comparable] struct {
	mu  sync.RWMutex
	set Set[T]
}

func NewSync[T comparable](opts ...optFunc) *SyncSet[T] {
	return &SyncSet[T]{
		set: New[T](opts...),
	}
}

// Add adds a key to the set.
func (s *SyncSet[T]) Add(key T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set.Add(key)
}

// AddAll adds multiple keys to the set.
func (s *SyncSet[T]) AddAll(keys ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set.AddAll(keys...)
}

// Remove removes a key from the set.
func (s *SyncSet[T]) Remove(key T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set.Remove(key)
}

// RemoveAll removes multiple keys from the set.
func (s *SyncSet[T]) RemoveAll(keys ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set.RemoveAll(keys...)
}

// Clear clears the set.
func (s *SyncSet[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set.Clear()
}

// Contains checks if a key is in the set.
func (s *SyncSet[T]) Contains(key T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.set.Contains(key)
}

// Missing checks if a key is not in the set.
func (s *SyncSet[T]) Missing(key T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.set.Missing(key)
}

// Slice returns the keys of the set as a slice.
func (s *SyncSet[T]) Slice() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.set.Slice()
}

// Len returns the number of keys in the set.
func (s *SyncSet[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.set)
}

// Iter returns an iterator over the keys in the set.
// The set is read-locked for the duration of iteration.
func (s *SyncSet[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for key := range s.set {
			if !yield(key) {
				return
			}
		}
	}
}
