// Package simulator is the public surface of the engine: a Sim holds one dataset and its
// config, and simulates builds against it. It reads no files; the host opens the dataset.
package simulator

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/semantics"
	sempoe1 "github.com/xaroth/pbt-engine/semantics/poe1"
)

// Errors of the simulator; match them with errors.Is.
var (
	// ErrGame is a dataset of a game the simulator has no rules for.
	ErrGame = errors.New("simulator: no rules for the game")
	// ErrUnreconcilable is a build the dataset cannot take at all: another game, a tree
	// variant or class the dataset lacks.
	ErrUnreconcilable = errors.New("simulator: build cannot be reconciled with the dataset")
	// ErrUnknownOutput is an output the game's rules do not have.
	ErrUnknownOutput = errors.New("simulator: unknown output")
)

// Warning codes the simulator adds to the build check's.
const (
	VersionMismatch  build.Code = "version-mismatch"
	ConfigUnknownKey build.Code = "config-unknown-key"
	ConfigBadValue   build.Code = "config-bad-value"
	UnmodelledStat   build.Code = "unmodelled-stat"
)

// Warning is something a simulation went on without; its message says what was done.
type Warning = build.Warning

// maxIdle is how many idle sheets a sim keeps per build shape.
const maxIdle = 8

// Sim simulates builds against one dataset. Its config can change with Configure; each call
// takes a snapshot when it starts. A Sim is safe for concurrent use.
type Sim struct {
	store *dataset.Store
	table *semantics.Table

	mu   sync.Mutex
	cfg  config.Values
	idle map[shape][]*poe1.Sheet
}

// shape is what a sheet over the whole tree is fixed to; everything else is switched.
type shape struct {
	variant, class, ascendancy string
	level                      int
}

// New returns a sim over the dataset in store, configured by opts (Set and Clear). It fails
// for a game without rules (only poe1 has them) or a bad option.
func New(store *dataset.Store, opts ...Option) (*Sim, error) {
	if g := store.Manifest().Game; g != poe1.Game {
		return nil, fmt.Errorf("%w: %s", ErrGame, g)
	}
	t, err := sempoe1.Load()
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	s := &Sim{store: store, table: t, cfg: config.Values{}, idle: map[shape][]*poe1.Sheet{}}
	if err := s.Configure(opts...); err != nil {
		return nil, err
	}
	return s, nil
}

// Store returns the sim's dataset.
func (s *Sim) Store() *dataset.Store { return s.store }

// Configure sets and clears sim config values. Calls already running keep their snapshot.
func (s *Sim) Configure(opts ...Option) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg.Clone()
	for _, o := range opts {
		switch {
		case o.err != nil:
			return fmt.Errorf("%w: %w", ErrOption, o.err)
		case o.kind == optSet:
			next[o.key] = o.values[o.key]
		case o.kind == optClear:
			next.Clear(o.key)
		default:
			return fmt.Errorf("%w: only Set and Clear configure a sim", ErrOption)
		}
	}
	s.cfg = next
	return nil
}

// Config returns a copy of the sim's config values.
func (s *Sim) Config() config.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Clone()
}

// Simulate computes the outputs of b. Config resolves per key from the call's options, the
// sim, the build, then the default. What the dataset cannot take in b is fixed and warned
// about (see build.Check); it fails only with ErrUnreconcilable, or for an unknown output or
// a fault of the dataset or engine.
func (s *Sim) Simulate(b *build.Build, opts ...Option) (*Stats, error) {
	co, err := readCallOptions(opts)
	if err != nil {
		return nil, err
	}
	keys, err := outputKeys(co.outputs)
	if err != nil {
		return nil, err
	}
	run, err := s.prepare(b, co)
	if err != nil {
		return nil, err
	}
	sh, err := s.checkout(run.checked)
	if err != nil {
		return nil, err
	}
	defer s.checkin(run.shape, sh)
	return run.stats(sh, keys)
}

// run is one build prepared for a sheet: checked, its config resolved, its warnings so far.
type run struct {
	build    *build.Build
	checked  *build.Checked
	shape    shape
	cfg      config.Resolved
	warnings []Warning
}

func (s *Sim) prepare(b *build.Build, co callOptions) (*run, error) {
	c, warnings, err := build.Check(b, s.store)
	if err != nil {
		if _, ok := errors.AsType[*build.CheckError](err); ok {
			return nil, fmt.Errorf("%w: %w", ErrUnreconcilable, err)
		}
		return nil, fmt.Errorf("simulator: %w", err)
	}
	r := &run{build: b.Clone(), checked: c, warnings: warnings}
	cb := c.Build()
	r.shape = shape{variant: cb.Variant, class: cb.Class, ascendancy: cb.Ascendancy, level: cb.Level}
	if w, ok := versionWarning(b.Version, s.store.Manifest().Version); ok {
		r.warnings = append(r.warnings, w)
	}
	sim := s.Config()
	for k := range co.cleared {
		sim.Clear(k)
	}
	var problems []config.Problem
	r.cfg, problems = config.Resolve(poe1.ConfigSpecs(),
		config.Layer{Source: config.FromCall, Values: co.set},
		config.Layer{Source: config.FromSim, Values: sim},
		config.Layer{Source: config.FromBuild, Values: cb.Config})
	for _, p := range problems {
		r.warnings = append(r.warnings, configWarning(p))
	}
	budget, err := poe1.PointBudget(c, r.cfg)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	r.warnings = append(r.warnings, budget...)
	return r, nil
}

// stats reads keys from sh after switching it to the run's nodes and config.
func (r *run) stats(sh *poe1.Sheet, keys []output.Key) (*Stats, error) {
	cb := r.checked.Build()
	sh.SetNodes(cb.Nodes, cb.Masteries)
	sh.SetConfig(r.cfg)
	st := &Stats{values: make(map[output.Key]float64, len(keys)), keys: keys, cfg: r.cfg, build: r.build}
	for _, k := range keys {
		v, err := sh.Output(k)
		if err != nil {
			return nil, fmt.Errorf("simulator: %w", err)
		}
		st.values[k] = v
	}
	st.warnings = slices.Concat(r.warnings, noteWarnings(sh.Notes()))
	return st, nil
}

// outputKeys returns keys, or every output when none are given; an unknown key fails.
func outputKeys(keys []output.Key) ([]output.Key, error) {
	outputs := poe1.Outputs()
	all := make([]output.Key, 0, len(outputs))
	for _, o := range outputs {
		all = append(all, o.Key)
	}
	if len(keys) == 0 {
		return all, nil
	}
	for _, k := range keys {
		if !slices.Contains(all, k) {
			return nil, fmt.Errorf("%w %q", ErrUnknownOutput, k)
		}
	}
	return keys, nil
}

// checkout takes an idle sheet of c's shape or opens one over the whole tree.
func (s *Sim) checkout(c *build.Checked) (*poe1.Sheet, error) {
	cb := c.Build()
	k := shape{variant: cb.Variant, class: cb.Class, ascendancy: cb.Ascendancy, level: cb.Level}
	s.mu.Lock()
	if idle := s.idle[k]; len(idle) > 0 {
		sh := idle[len(idle)-1]
		s.idle[k] = idle[:len(idle)-1]
		s.mu.Unlock()
		return sh, nil
	}
	s.mu.Unlock()
	r, err := resolve.Tree(c)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	sh, err := poe1.Open(r, s.table, poe1.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return sh, nil
}

func (s *Sim) checkin(k shape, sh *poe1.Sheet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.idle[k]) < maxIdle {
		s.idle[k] = append(s.idle[k], sh)
	}
}

// versionWarning warns when the build was made for a version the dataset is not a patch of.
func versionWarning(want, have string) (Warning, bool) {
	if want == "" || want == have || strings.HasPrefix(have, want+".") {
		return Warning{}, false
	}
	return Warning{Code: VersionMismatch, Subject: "version", Message: fmt.Sprintf("build for %s, simulated on dataset %s", want, have)}, true
}

func configWarning(p config.Problem) Warning {
	code, what := ConfigBadValue, "ignored"
	if errors.Is(p.Err, config.ErrUnknownKey) {
		code = ConfigUnknownKey
	}
	return Warning{Code: code, Subject: "config " + p.Key, Message: fmt.Sprintf("%s value: %v; %s", p.Source, p.Err, what)}
}

func noteWarnings(notes []poe1.Note) []Warning {
	out := make([]Warning, 0, len(notes))
	for _, n := range notes {
		msg := n.Kind.String() + ", left out"
		if n.Text != "" {
			msg = n.Kind.String() + ": " + n.Text + ", left out"
		}
		out = append(out, Warning{Code: UnmodelledStat, Subject: n.Stat + " from " + n.Source.String(), Message: msg})
	}
	return out
}
