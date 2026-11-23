package simulator

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// Stats is the result of one simulation: output values, warnings and the config each key
// resolved to. It keeps a copy of the build, so later edits to the build do not change it.
type Stats struct {
	values   map[output.Key]float64
	keys     []output.Key
	warnings []Warning
	cfg      config.Resolved
	budget   Budget
	build    *build.Build
	checked  *build.Checked
	items    []poe1.Item
	hands    poe1.Hands
	skill    *resolve.Skill
	// sheetSkills are the sheet's skills, the main skill first.
	sheetSkills []*resolve.Skill
	// perSkill are the outputs of each skill asked for, as Skills returns them.
	perSkill []SkillStats
	// enemy is what the main skill's hits are measured against.
	enemy poe1.EnemyStats
	// totems gives the base life of the skills' totems.
	totems poe1.TotemLives
	// minions are the minions of the skills whose outputs are read, by slot.
	minions []*poe1.Minion
	// animatedWeapons is the number of Animated Weapons the build's skills allow.
	animatedWeapons int
	// fullDPS are the skills counted in full DPS, by slot.
	fullDPS []poe1.FullDPSSkill
	sim     *Sim

	// explainMu guards the explanation sheet: its session is not safe for concurrent use.
	explainMu    sync.Mutex
	explainSheet *poe1.Sheet
	explainErr   error
}

// Explain explains output k in view v. The first call opens a sheet from the build and config
// the simulation used (about a millisecond); later calls reuse it. Safe for concurrent use;
// explanations of one Stats run one at a time.
func (st *Stats) Explain(k output.Key, v View) (*Explanation, error) {
	st.explainMu.Lock()
	defer st.explainMu.Unlock()
	if st.explainSheet == nil && st.explainErr == nil {
		st.explainSheet, st.explainErr = st.openSheet()
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

// MainSkill is the main skill a simulation read its skill outputs in: its gem, granted effect,
// level and quality with what items and supports add, and its ActiveSkillType ids with those
// its supports add.
type MainSkill struct {
	// Group and Gem index the gem in build.Skills, from 0; both are -1, and ID empty, for the
	// default attack, which a build without a main skill of its own uses.
	Group, Gem     int
	ID             string
	GrantedEffect  string
	Level, Quality int
	Types          []string
	// Supports are those applied to it, from its group, the item it is socketed in and linked
	// slots.
	Supports []MainSupport
}

// MainSupport is a support applied to the main skill, at its level and quality with what items
// add to a gem.
type MainSupport struct {
	GrantedEffect  string
	Level, Quality int
}

// MainSkill returns the main skill, and false when the build has none; then the skill outputs
// are not computed.
func (st *Stats) MainSkill() (MainSkill, bool) {
	if st.skill == nil {
		return MainSkill{}, false
	}
	sk := st.skill
	supports := make([]MainSupport, 0, len(sk.Supports))
	for _, sp := range sk.Supports {
		supports = append(supports, MainSupport{GrantedEffect: sp.GrantedEffect, Level: sp.Level, Quality: sp.Quality})
	}
	return MainSkill{
		Group: sk.Source.Group, Gem: sk.Source.Gem, ID: sk.Source.ID, GrantedEffect: sk.GrantedEffect,
		Level: sk.Level, Quality: sk.Quality, Types: slices.Clone(sk.Types), Supports: supports,
	}, true
}

// Budget returns the points the build can spend under the config the simulation resolved.
// The points it uses are build.Checked.Points and, per id, build.Checked.AscendancyPoints.
func (st *Stats) Budget() Budget {
	b := st.budget
	b.Ascendancies = maps.Clone(b.Ascendancies)
	return b
}

// Warnings returns what the simulation went on without and what it did about each.
func (st *Stats) Warnings() []Warning { return slices.Clone(st.warnings) }

// Named is any config key.
type Named interface{ Name() string }

// Config returns the value the key resolved to and its source, and false for a key the game
// does not have. A condition the build can cause itself (EnemyShocked, EnemyChilled,
// EnemyIgnited, EnemyFireExposure, EnemyColdExposure, EnemyLightningExposure) that no layer sets
// is on with source config.Derived when a skill of the build inflicts it, else off with source
// config.Default. Exposure a hit inflicts only by chance is no such source.
func (st *Stats) Config(k Named) (config.Entry, bool) {
	e, ok := st.cfg[k.Name()]
	return e, ok
}

// openSheet opens a sheet of the simulated build with its items, and the conditions it causes
// derived again: the config alone does not give the Exposure the build's hits inflict.
func (st *Stats) openSheet() (*poe1.Sheet, error) {
	r, err := resolve.Resolve(st.checked)
	if err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	sh, err := poe1.Open(r, st.sim.table, st.cfg)
	if err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	if err := sh.SetItems(st.items); err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	if err := sh.SetHands(st.hands); err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	sh.SetTotemLives(st.totems)
	sh.SetAnimatedWeaponsAllowed(st.animatedWeapons)
	if len(st.sheetSkills) > 0 {
		if err := sh.SetSkills(st.sheetSkills); err != nil {
			return nil, fmt.Errorf("simulator: explain: %w", err)
		}
	}
	if err := sh.SetMinions(st.minions); err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	sh.SetFullDPS(st.fullDPS)
	sh.SetEnemy(st.enemy)
	if _, _, err := sh.DeriveConditions(st.cfg); err != nil {
		return nil, fmt.Errorf("simulator: explain: %w", err)
	}
	return sh, nil
}
