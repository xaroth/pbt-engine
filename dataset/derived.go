package dataset

import (
	"fmt"
	"sync"
)

// Derived returns what build makes from s for key, built on the first call for that key and
// shared after, for the life of s, like a group. It is for values that are costly to make from
// the groups and wanted on many calls. Like a group, the first result is kept, error included,
// and the value is read-only.
//
// key must be comparable and of a type the calling package defines, as with context values,
// so that two packages never share a key; every call with one key must give the same T.
func Derived[T any](s *Store, key any, build func(*Store) (T, error)) (T, error) {
	v, _ := s.derived.LoadOrStore(key, &derivedValue{})
	d, _ := v.(*derivedValue)
	d.once.Do(func() { d.v, d.err = build(s) })
	if d.err != nil {
		var zero T
		return zero, d.err
	}
	t, ok := d.v.(T)
	if !ok {
		panic(fmt.Sprintf("dataset: Derived key %v holds a %T, not a %T", key, d.v, t))
	}
	return t, nil
}

type derivedValue struct {
	once sync.Once
	v    any
	err  error
}
