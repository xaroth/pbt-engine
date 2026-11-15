package dataset_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

type derivedKey struct{ name string }

var errBuild = errors.New("build failed")

// TestDerived: concurrent calls with one key build once and share the value, an error is kept
// like a value, and another key or another store builds its own.
func TestDerived(t *testing.T) {
	t.Parallel()
	open := func() *dataset.Store {
		s, err := dataset.Open(written(t, sample()))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		return s
	}
	s := open()
	var builds atomic.Int32
	counted := func(err error) func(*dataset.Store) (*int, error) {
		return func(*dataset.Store) (*int, error) {
			builds.Add(1)
			if err != nil {
				return nil, err
			}
			return new(int), nil
		}
	}
	build := counted(nil)

	var wg sync.WaitGroup
	got := make([]*int, 8)
	for i := range got {
		wg.Go(func() {
			v, err := dataset.Derived(s, derivedKey{"a"}, build)
			if err != nil {
				t.Errorf("Derived: %v", err)
			}
			got[i] = v
		})
	}
	wg.Wait()
	for _, v := range got[1:] {
		if v != got[0] {
			t.Fatal("Derived: different values for one key, want one shared")
		}
	}
	if n := builds.Load(); n != 1 {
		t.Errorf("built %d times for one key, want 1", n)
	}

	if v, _ := dataset.Derived(s, derivedKey{"b"}, build); v == got[0] {
		t.Error("another key shares the first key's value")
	}
	if v, _ := dataset.Derived(open(), derivedKey{"a"}, build); v == got[0] {
		t.Error("another store shares the first store's value")
	}

	failing := counted(errBuild)
	before := builds.Load()
	for range 2 {
		if _, err := dataset.Derived(s, derivedKey{"c"}, failing); !errors.Is(err, errBuild) {
			t.Errorf("Derived: error %v, want %v", err, errBuild)
		}
	}
	if n := builds.Load() - before; n != 1 {
		t.Errorf("failing build ran %d times, want 1", n)
	}
}
