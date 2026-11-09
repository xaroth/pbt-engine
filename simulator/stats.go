package simulator

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/output"
)

// Stats is the result of one simulation: output values, warnings and the config each key
// resolved to. It keeps a copy of the build, so later edits to the build do not change it.
type Stats struct {
	values   map[output.Key]float64
	keys     []output.Key
	warnings []Warning
	cfg      config.Resolved
	build    *build.Build
}

// Get returns the value of output k, and false when the simulation did not compute it.
func (st *Stats) Get(k output.Key) (float64, bool) {
	v, ok := st.values[k]
	return v, ok
}

// Keys returns the outputs computed, in the rules' display order or as asked for.
func (st *Stats) Keys() []output.Key { return slices.Clone(st.keys) }

// Warnings returns what the simulation went on without and what it did about each.
func (st *Stats) Warnings() []Warning { return slices.Clone(st.warnings) }

// Named is any config key.
type Named interface{ Name() string }

// Config returns the value the key resolved to and its source, and false for a key the game
// does not have.
func (st *Stats) Config(k Named) (config.Entry, bool) {
	e, ok := st.cfg[k.Name()]
	return e, ok
}
