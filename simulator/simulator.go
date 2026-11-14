// Package simulator is the public surface of the engine: a Sim holds one dataset and its
// config, and simulates builds against it. It reads no files; the host opens the dataset.
//
// Stats, Diff, sweep Result and Explanation encode to JSON for hosts that send results on,
// such as a browser worker to its UI; wire.schema.json (JSON Schema 2020-12) describes them,
// for generating types in other languages. Every field is always present except the optional
// fields of an edit, lists keep the engine's order, and a percentage change from 0 is null, so
// equal results give equal bytes. Sim.Outputs encodes to the catalogue entries alone.
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
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/output"
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
	// UncheckedCondition is a stat applied without checking a condition of its text: unlike a
	// stat left out, it can over-apply.
	UncheckedCondition build.Code = "unchecked-condition"
	// SkillsNotSimulated is a build with skill groups beyond a main spell hit's: outputs leave
	// them out until those skills are simulated.
	SkillsNotSimulated build.Code = "skills-not-simulated"
	// ItemsNotSimulated is a build with items on a dataset without the items group: outputs
	// leave them out.
	ItemsNotSimulated build.Code = "items-not-simulated"
	// ItemDefenceAssumed is an item defence its text does not give, on a base whose roll of it
	// varies: the highest roll is taken.
	ItemDefenceAssumed build.Code = "item-defence-assumed"
)

// Warning is something a simulation went on without; its message says what was done.
type Warning = build.Warning

// Budget is the passive and ascendancy points a build can spend under its config. A build
// over it keeps its nodes and has a warning.
type Budget = poe1.Budget

// maxIdle is how many idle sheets a sim keeps over all build shapes; the least recently used
// goes first. A whole-tree sheet is about a megabyte.
const maxIdle = 16

// maxCompiled is how many compiled items a sim keeps; past it the cache starts over.
const maxCompiled = 1024

// Sim simulates builds against one dataset. Its config can change with Configure; each call
// takes a snapshot when it starts. A Sim is safe for concurrent use.
type Sim struct {
	store *dataset.Store
	table *semantics.Table
	// items reads item text; made on first use, since most uses of a sim read none.
	items func() (*items.Reader, error)
	// bases indexes the dataset's item bases by id, on first use.
	bases func() (map[string]*dataset.ItemBase, error)
	// nodeTexts renders passive nodes, per language on first use.
	nodeTexts nodeTexts
	// radii find the nodes around jewel sockets, per tree variant on first use.
	radii radii

	mu  sync.Mutex
	cfg config.Values
	// idle sheets, least recently returned first.
	idle []idleSheet
	// compiled items by content.
	compiled map[string]compiledItem
}

type compiledItem struct {
	stats   []resolve.StatValue
	assumed []string
}

// shape is what a sheet over the whole tree is fixed to; nodes, masteries, level and config
// are switched.
type shape struct {
	variant, class, ascendancy, bloodline string
	// overrides lists node:override pairs by node, since they change the nodes' bundles.
	overrides string
}

// shapeOf returns the shape of the sheet c's build needs: the tree nodes it can allocate
// and what they give depend on these.
func shapeOf(c *build.Checked) shape {
	cb := c.Build()
	var overrides strings.Builder
	for _, o := range c.Overrides() {
		fmt.Fprintf(&overrides, "%d:%s,", o.Node.ID, o.Override.ID)
	}
	return shape{variant: cb.Variant, class: cb.Class, ascendancy: cb.Ascendancy, bloodline: cb.Bloodline, overrides: overrides.String()}
}

type idleSheet struct {
	shape shape
	sheet *poe1.Sheet
}

// New returns a sim over the dataset in store, configured by opts (Set and Clear). It fails
// for a game without rules (only poe1 has them) or a bad option.
func New(store *dataset.Store, opts ...Option) (*Sim, error) {
	if g := store.Manifest().Game; g != poe1.Game {
		return nil, fmt.Errorf("%w: %s", ErrGame, g)
	}
	t, err := poe1.Load()
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	s := &Sim{store: store, table: t, cfg: config.Values{}}
	s.items = sync.OnceValues(func() (*items.Reader, error) { return items.NewReader(store) })
	s.bases = sync.OnceValues(func() (map[string]*dataset.ItemBase, error) { return indexBases(store) })
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
	run, err := s.prepare(b, co, s.Config())
	if err != nil {
		return nil, err
	}
	sh, err := s.checkout(run.checked)
	if err != nil {
		return nil, err
	}
	defer s.checkin(run.shape, sh)
	return s.statsOf(run, sh, keys)
}

// run is one build prepared for a sheet: checked, its config resolved, its warnings so far.
type run struct {
	build   *build.Build
	checked *build.Checked
	shape   shape
	cfg     config.Resolved
	budget  Budget
	items   []poe1.Item
	// skill is the main skill, nil when the build has none; reserving are the other skills
	// that reserve life or mana.
	skill     *resolve.Skill
	reserving []*resolve.Skill
	// enemy is what the main skill's hits are measured against.
	enemy    poe1.EnemyStats
	warnings []Warning
}

// prepare checks b and resolves its config against the sim config snapshot sim.
func (s *Sim) prepare(b *build.Build, co callOptions, sim config.Values) (*run, error) {
	c, warnings, err := build.Check(b, s.store)
	if err != nil {
		if _, ok := errors.AsType[*build.CheckError](err); ok {
			return nil, fmt.Errorf("%w: %w", ErrUnreconcilable, err)
		}
		return nil, fmt.Errorf("simulator: %w", err)
	}
	r := &run{build: b.Clone(), checked: c, warnings: warnings}
	cb := c.Build()
	r.shape = shapeOf(c)
	if w, ok := versionWarning(b.Version, s.store.Manifest().Version); ok {
		r.warnings = append(r.warnings, w)
	}
	if err := s.compileItems(r, cb.Items); err != nil {
		return nil, err
	}
	if err := mainSkill(r); err != nil {
		return nil, err
	}
	sim = sim.Clone()
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
	var over []Warning
	r.budget, over, err = poe1.PointBudget(c, r.cfg)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	r.warnings = append(r.warnings, over...)
	if r.enemy, err = enemyOf(s.store, r); err != nil {
		return nil, err
	}
	return r, nil
}

// enemyOf resolves the enemy of r's config when the sheet gives the main skill's hit, the
// default enemy otherwise. An enemy kind with a normal monster's armour and evasion reads them
// from the dataset's monsters group, and fails without it.
func enemyOf(store *dataset.Store, r *run) (poe1.EnemyStats, error) {
	if r.skill == nil || !poe1.SpellHit(r.skill) {
		return poe1.DefaultEnemy(), nil
	}
	var levels []dataset.MonsterLevel
	m, err := store.Monsters()
	switch {
	case err == nil:
		levels = m.Levels
	case !errors.Is(err, dataset.ErrNotInDataset):
		return poe1.EnemyStats{}, fmt.Errorf("simulator: %w", err)
	}
	e, err := poe1.EnemyOf(r.cfg, r.checked.Build().Level, levels)
	if err != nil {
		return poe1.EnemyStats{}, fmt.Errorf("simulator: %w", err)
	}
	return e, nil
}

// mainSkill resolves the run's main skill, none when the build has no group with an active
// gem to resolve or the dataset has no skills group (the build check warns about that), warns
// about the skills it does not simulate, and resolves the other skills that reserve life or
// mana.
func mainSkill(r *run) error {
	if len(r.checked.Build().Skills.Groups) == 0 {
		return nil
	}
	sk, warnings, err := resolve.MainSkill(r.checked, poe1.GemBonuses())
	switch {
	case errors.Is(err, resolve.ErrNoActiveSkill), errors.Is(err, dataset.ErrNotInDataset):
	case err != nil:
		return fmt.Errorf("simulator: %w", err)
	default:
		r.skill = sk
		r.warnings = append(r.warnings, warnings...)
	}
	if w, ok := skillsWarning(r.checked.Build().Skills, r.skill); ok {
		r.warnings = append(r.warnings, w)
	}
	return reservingSkills(r)
}

// reservingSkills resolves the run's skills other than its main skill that reserve life or
// mana.
func reservingSkills(r *run) error {
	all, err := resolve.Skills(r.checked, poe1.GemBonuses())
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		return nil
	case err != nil:
		return fmt.Errorf("simulator: %w", err)
	}
	main := r.skill
	for _, s := range all {
		if !poe1.Reserves(s.Skill) || main != nil && s.Skill.Source == main.Source && s.Skill.GrantedEffect == main.GrantedEffect {
			continue
		}
		r.reserving = append(r.reserving, s.Skill)
		r.warnings = append(r.warnings, s.Warnings...)
	}
	return nil
}

// skills returns the run's skills for the sheet: the main skill first, if any, then the other
// reserving skills.
func (r *run) skills() []*resolve.Skill {
	if r.skill == nil {
		return r.reserving
	}
	return append([]*resolve.Skill{r.skill}, r.reserving...)
}

// switchTo switches sh to the run's level, nodes, masteries, items, skills and config.
func switchTo(r *run, sh *poe1.Sheet) error {
	cb := r.checked.Build()
	if err := sh.SetLevel(cb.Level); err != nil {
		return fmt.Errorf("simulator: %w", err)
	}
	sh.SetNodes(cb.Nodes, cb.Masteries)
	if err := sh.SetItems(r.items); err != nil {
		return fmt.Errorf("simulator: %w", err)
	}
	if err := sh.SetSkills(r.skills()); err != nil {
		return fmt.Errorf("simulator: %w", err)
	}
	sh.SetConfig(r.cfg)
	sh.SetEnemy(r.enemy)
	return nil
}

// statsOf reads keys from sh after switching it to the run's nodes and config. Skill outputs
// the run's main skill has none of (poe1.Gives) are left out.
func (s *Sim) statsOf(r *run, sh *poe1.Sheet, keys []output.Key) (*Stats, error) {
	if err := switchTo(r, sh); err != nil {
		return nil, err
	}
	cat := catalogue()
	keys = slices.DeleteFunc(slices.Clone(keys), func(k output.Key) bool { return !poe1.Gives(cat[k], r.skill) })
	st := &Stats{
		values: make(map[output.Key]float64, len(keys)), keys: keys, cfg: r.cfg, budget: r.budget,
		build: r.build, checked: r.checked, items: r.items, skill: r.skill, skills: r.skills(), enemy: r.enemy, sim: s,
	}
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

// checkout takes the most recently returned idle sheet of c's shape or opens one over the
// whole tree.
func (s *Sim) checkout(c *build.Checked) (*poe1.Sheet, error) {
	k := shapeOf(c)
	s.mu.Lock()
	for i, e := range slices.Backward(s.idle) {
		if e.shape == k {
			s.idle = slices.Delete(s.idle, i, i+1)
			s.mu.Unlock()
			return e.sheet, nil
		}
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

// checkin returns a sheet to the pool, dropping the least recently used beyond maxIdle.
func (s *Sim) checkin(k shape, sh *poe1.Sheet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idle = append(s.idle, idleSheet{shape: k, sheet: sh})
	if over := len(s.idle) - maxIdle; over > 0 {
		s.idle = slices.Delete(s.idle, 0, over)
	}
}

// idleSheets returns how many idle sheets the pool holds, for tests.
func (s *Sim) idleSheets() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.idle)
}

// versionWarning warns when the build was made for a version the dataset is not a patch of.
func versionWarning(want, have string) (Warning, bool) {
	if want == "" || want == have || strings.HasPrefix(have, want+".") {
		return Warning{}, false
	}
	return Warning{Code: VersionMismatch, Subject: "version", Message: fmt.Sprintf("build for %s, simulated on dataset %s", want, have)}, true
}

// skillsWarning warns about the gems of s outside the main skill's group whose hit the sheet
// gives (main, nil for none).
func skillsWarning(s build.Skills, main *resolve.Skill) (Warning, bool) {
	gems, groups := 0, 0
	for i, g := range s.Groups {
		if len(g.Gems) == 0 || main != nil && i == main.Source.Group && poe1.SpellHit(main) {
			continue
		}
		gems += len(g.Gems)
		groups++
	}
	if gems == 0 {
		return Warning{}, false
	}
	msg := fmt.Sprintf("%d gems in %d groups not simulated yet; outputs leave them out", gems, groups)
	return Warning{Code: SkillsNotSimulated, Subject: "skills", Message: msg}, true
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
		w := Warning{Subject: n.Stat + " from " + n.Source.String(), Stat: n.Stat}
		if item, ok := n.Source.(resolve.ItemSource); ok {
			w.Slot = item.Slot
		}
		if n.Kind == poe1.Unchecked {
			w.Code, w.Message = UncheckedCondition, "applied without checking: "+n.Text
			out = append(out, w)
			continue
		}
		w.Code, w.Message = UnmodelledStat, n.Kind.String()+", left out"
		if n.Text != "" {
			w.Message = n.Kind.String() + ": " + n.Text + ", left out"
		}
		out = append(out, w)
	}
	return out
}
