// Package set provides a generic Set type backed by a map.
package set

// Set is a generic unordered collection of unique values.
type Set[T comparable] map[T]struct{}

// New creates a Set pre-populated with the given items.
func New[T comparable](items ...T) Set[T] {
	s := make(Set[T], len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

// Add inserts a value into the set.
func (s Set[T]) Add(v T) {
	s[v] = struct{}{}
}

// Contains reports whether the set contains the value.
func (s Set[T]) Contains(v T) bool {
	_, ok := s[v]
	return ok
}

// Remove deletes a value from the set. No-op if absent.
func (s Set[T]) Remove(v T) {
	delete(s, v)
}

// Len returns the number of elements in the set.
func (s Set[T]) Len() int {
	return len(s)
}

// Items returns all elements as a slice (unordered).
func (s Set[T]) Items() []T {
	items := make([]T, 0, len(s))
	for v := range s {
		items = append(items, v)
	}
	return items
}
