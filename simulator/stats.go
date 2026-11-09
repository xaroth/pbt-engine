package simulator

import (
	"fmt"
	"slices"
	"sync"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/rules/poe1"
)

// Stats is the result of one simulation: output values, warnings and the config each key
// resolved to. It keeps a copy of the build, so later edits to the build do not change it.
type Stats struct {
	values   map[output.Key]float64
	keys     []output.Key
	warnings []Warning
	cfg      config.Resolved
	build    *build.Build
	checked  *build.Checked
	sim      *Sim

	// explainMu guards the explanation sheet: its session is not safe for concurrent use.
	explainMu    sync.Mutex
	explainSheet *poe1.Sheet
	explainErr   error
}

// Explain explains output k in view v. The first call opens a sheet from the build and config
// the simulation used (about a millisecond); later calls reuse it. Safe for concurrent use;
// explanations of one Stats run one at a time.
func (st *Stats) Explain(k output.Key, v poe1.View) (*poe1.Explanation, error) {
	st.explainMu.Lock()
	defer st.explainMu.Unlock()
	if st.explainSheet == nil && st.explainErr == nil {
		r, err := resolve.Resolve(st.checked)
		if err != nil {
			st.explainErr = fmt.Errorf("simulator: explain: %w", err)
		} else {
			st.explainSheet, st.explainErr = poe1.Open(r, st.sim.table, st.cfg)
		}
	}
	if st.explainErr != nil {
		return nil, st.explainErr
	}
	ex, err := st.explainSheet.Explain(k, v)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return ex, nil
}

// Build returns a copy of the build as simulated, before the check's fixes.
func (st *Stats) Build() *build.Build { return st.build.Clone() }

// Text returns the dataset's display text in language, for naming sources in explanations.
func (st *Stats) Text(language string) (*dataset.Text, error) {
	t, err := st.sim.store.Text(language)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return t, nil
}

// Version returns the version of the dataset the build was simulated on.
func (st *Stats) Version() string { return st.sim.store.Manifest().Version }

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
