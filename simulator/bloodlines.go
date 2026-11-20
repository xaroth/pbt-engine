package simulator

import (
	"fmt"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// Bloodline is an alternate ascendancy of a tree with the classes that may take it, and whether
// the game still grants it.
type Bloodline = poe1.Bloodline

// Bloodlines returns the bloodlines of the tree variant in store, in the tree's order, without a
// Sim: it reads only that variant's tree file and decodes none of its nodes, so a host lists
// them without loading the stats or text groups.
func Bloodlines(store *dataset.Store, variant string) ([]Bloodline, error) {
	b, err := poe1.Bloodlines(store, variant)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return b, nil
}
