// Package passives reads the passive tree of a Path of Exile 1 dataset: the text of a node and
// the passives in a jewel's radius. It needs no simulator.
package passives

import "github.com/xaroth/pbt-engine/dataset"

// Reader reads the passive trees of one dataset. What it derives from the dataset, such as a
// language's descriptions or a variant's radius index, is made on first use and kept with the
// store, so every Reader of a store shares it. A Reader is safe for concurrent use.
type Reader struct {
	store *dataset.Store
}

// New returns a Reader of s.
func New(s *dataset.Store) *Reader {
	return &Reader{store: s}
}
