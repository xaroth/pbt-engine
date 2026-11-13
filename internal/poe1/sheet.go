package poe1

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Game is the edition these rules are for.
const Game = "poe1"

// Errors returned by Open and Sheet; match them with errors.Is.
var (
	ErrOtherGame   = errors.New("build is for another game")
	ErrUnknownStat = errors.New("unknown engine stat")
	// ErrConstant marks a constant the rules need that the dataset lacks or holds malformed.
	ErrConstant = errors.New("missing or malformed constant")
	// ErrActor marks a bundle for an actor the sheet does not model yet.
	ErrActor = errors.New("actor not supported")
	// ErrCheckpointOpen marks a change that adds to the sheet, such as a level not seen yet,
	// while a checkpoint is open: Revert could not take the addition back.
	ErrCheckpointOpen = errors.New("sheet: cannot add while a checkpoint is open")
)

// NoteKind says why a bundle stat is not, or not fully, in the sheet, or is in it unchecked.
type NoteKind uint8

// Note kinds.
const (
	// NoEntry: the semantics table has no entry for the stat, scope and game version.
	NoEntry NoteKind = iota + 1
	// NotApplied: the entry is unknown; Note.Text is its reason.
	NotApplied
	// Partial: the entry's modifiers apply but leave effects out; Note.Text lists them.
	Partial
	// RuleMissing: the entry needs a rule these rules do not implement; Note.Text names it.
	RuleMissing
	// Unchecked: the entry's modifiers apply without checking a condition of the stat's text;
	// Note.Text names it.
	Unchecked
)

func (k NoteKind) String() string {
	switch k {
	case NoEntry:
		return "no entry"
	case NotApplied:
		return "not applied"
	case Partial:
		return "partial"
	case RuleMissing:
		return "rule missing"
	case Unchecked:
		return "unchecked"
	}
	return "note?"
}

// Note records a bundle stat the sheet leaves out in whole or in part.
type Note struct {
	Kind   NoteKind
	Source resolve.Source
	Scope  resolve.Scope
	Stat   string
	Text   string
}

func (n Note) String() string {
	s := fmt.Sprintf("%s: %s %s from %s", n.Kind, n.Stat, n.Scope, n.Source)
	if n.Text != "" {
		s += ": " + n.Text
	}
	return s
}

// capabilityVersions maps each mechanic with more than one rule version to the highest version
// these rules implement, for dataset.Capabilities.Check.
func capabilityVersions() map[string]int { return map[string]int{} }

// Sheet is the character sheet of one resolved build. It keeps the evaluation session, so
// values are computed on first read.
type Sheet struct {
	session *core.Session
	stats   map[string]core.StatID
	ctx     contexts
	// counters are the engine stats of the tree's counters (resolve.Counters): a bundle stat
	// that is one adds its value to it and to number_of_allocated_masteries, without a table
	// entry.
	counters map[string]bool
	// sources describes each core source, by core.SourceID.
	sources []Source
	version string
	notes   []sourcedNote
	// table and gameVersion turn bundles added after Open, such as other levels, into modifiers.
	table       *semantics.Table
	gameVersion dataset.GameVersion
	// nodes maps a node, or a mastery node and effect, to its sources (global and buff bundles).
	nodes map[resolve.NodeSource][]core.SourceID
	// items maps an item, by slot and content, to its source.
	items map[itemKey]core.SourceID
	// skills holds the skills of SetSkills, by slot.
	skills []skillSlot
	// skillSources maps a skill, by slot and content, to the sources of its own stats.
	skillSources map[skillKey][]core.SourceID
	// marked is true from Mark to Commit: switches are journalled, additions refused.
	marked bool
}

// sourcedNote is a note and the core source of its bundle; always marks notes that hold
// whether the source is on or not.
type sourcedNote struct {
	note   Note
	src    core.SourceID
	always bool
}

// Notes lists the bundle stats left out of the sheet, applied in part, or applied without
// checking a condition, in bundle order, for the bundles that are on: a sheet over the whole
// tree notes only the allocated nodes.
func (sh *Sheet) Notes() []Note {
	var out []Note
	for _, n := range sh.notes {
		if n.always || sh.session.SourceEnabled(n.src) {
			out = append(out, n.note)
		}
	}
	return out
}

// Open builds the sheet of r under config cfg (resolved over ConfigSpecs; keys it lacks take
// their defaults): each bundle stat goes through t to engine modifiers, one source per bundle,
// then the rules add theirs, such as the attribute bonuses from r's constants. Quest rewards
// are on only when cfg's campaign stage or bandit choice grants them.
// Stats t cannot apply become notes, not errors. It fails when r is for another game, its
// version does not parse, its dataset needs a rule version these rules do not implement, a
// bundle is for an actor other than the player, or a constant is missing.
func Open(r *resolve.Resolved, t *semantics.Table, cfg config.Resolved) (*Sheet, error) {
	if r.Game != Game || t.Game != Game {
		return nil, fmt.Errorf("%w: build %s, table %s, rules %s", ErrOtherGame, r.Game, t.Game, Game)
	}
	v, err := dataset.ParseGameVersion(r.Version)
	if err != nil {
		return nil, fmt.Errorf("sheet: %w", err)
	}
	if _, err := r.Capabilities.Check(capabilityVersions()); err != nil {
		return nil, fmt.Errorf("sheet: dataset %s: %w", r.Version, err)
	}
	md := modelFor(r.Counters)
	s, err := core.NewSession(md.model, 1)
	if err != nil {
		return nil, fmt.Errorf("sheet: %w", err)
	}
	sh := &Sheet{session: s, stats: md.stats, ctx: md.ctx, counters: md.counters, version: r.Version, table: t, gameVersion: v}
	for i := range r.Bundles {
		if _, err := sh.add(&r.Bundles[i], t, v, -1); err != nil {
			return nil, err
		}
	}
	if err := sh.addAttributeBonuses(r.Constants); err != nil {
		return nil, err
	}
	if err := sh.addBaseValues(); err != nil {
		return nil, err
	}
	sh.SetConfig(cfg)
	s.Commit()
	return sh, nil
}

// Value returns the value of engine stat name.
func (sh *Sheet) Value(name string) (float64, error) {
	id, ok := sh.stats[name]
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownStat, name)
	}
	val, err := sh.session.Get(core.Node{Stat: id, Ctx: core.GlobalCtx})
	if err != nil {
		return 0, fmt.Errorf("sheet: %s: %w", name, err)
	}
	return val, nil
}

// newSource adds a core source described by d.
func (sh *Sheet) newSource(d Source) core.SourceID {
	sh.sources = append(sh.sources, d)
	id := sh.session.NewSource()
	if d.Bundle != nil {
		if n, ok := d.Bundle.Source.(resolve.NodeSource); ok {
			if sh.nodes == nil {
				sh.nodes = map[resolve.NodeSource][]core.SourceID{}
			}
			sh.nodes[n] = append(sh.nodes[n], id)
		}
	}
	return id
}

// proportion returns the proportional view of engine stat name.
func (sh *Sheet) proportion(name string) (core.Proportion, error) {
	id, ok := sh.stats[name]
	if !ok {
		return core.Proportion{}, fmt.Errorf("%w %q", ErrUnknownStat, name)
	}
	p, err := sh.session.Proportional(core.Node{Stat: id})
	if err != nil {
		return core.Proportion{}, fmt.Errorf("sheet: %s: %w", name, err)
	}
	return p, nil
}

// sheetModel is the core model of a sheet over a tree with some counters, shared by every
// sheet over such a tree; sheets only read it.
type sheetModel struct {
	model    *core.Model
	stats    map[string]core.StatID
	ctx      contexts
	counters map[string]bool
}

//nolint:gochecknoglobals // a cache of immutable models, one per set of counters
var models struct {
	sync.Mutex

	byCounters map[string]*sheetModel
}

// modelFor returns the model of the engine stats and counters; counters are sorted.
func modelFor(counters []string) *sheetModel {
	key := strings.Join(counters, ",")
	models.Lock()
	defer models.Unlock()
	if md, ok := models.byCounters[key]; ok {
		return md
	}
	var b core.ModelBuilder
	md := &sheetModel{stats: map[string]core.StatID{}, counters: make(map[string]bool, len(counters))}
	for _, name := range engineStats() {
		md.stats[name] = b.MustAddStat(statDef(name, md.stats))
	}
	addTransferStats(&b, md.stats)
	md.ctx = addContexts(&b)
	for _, name := range counters {
		if _, ok := md.stats[name]; !ok {
			md.stats[name] = b.MustAddStat(core.StatDef{Name: name})
			md.counters[name] = true
		}
	}
	md.model = b.Build()
	if models.byCounters == nil {
		models.byCounters = map[string]*sheetModel{}
	}
	models.byCounters[key] = md
	return md
}

// add adds bd as a new source and returns it. Its modifiers apply in the context of the skill
// in slot only, or everywhere when slot is negative.
func (sh *Sheet) add(bd *resolve.Bundle, t *semantics.Table, v dataset.GameVersion, slot int) (core.SourceID, error) {
	// Every modifier lands on core actor 0, so another actor's stats would stack onto the player.
	if bd.Actor != resolve.Player {
		return 0, fmt.Errorf("sheet: %s bundle %s: %w", bd.Actor, bd.Source, ErrActor)
	}
	b := *bd
	src := sh.newSource(Source{Bundle: &b})
	for _, sv := range bd.Stats {
		if sh.counters[sv.Stat] {
			if err := sh.addCount(sv, src); err != nil {
				return 0, err
			}
			continue
		}
		e, ok := t.Lookup(sv.Stat, bd.Scope, v)
		ruled := false
		if ok && e.Status == semantics.Authored {
			var err error
			if ruled, err = sh.addEntry(e, sv.Value, src, slot); err != nil {
				return 0, err
			}
		}
		for _, n := range notesFor(e, ok, ruled) {
			n.Source, n.Scope, n.Stat = bd.Source, bd.Scope, sv.Stat
			sh.notes = append(sh.notes, sourcedNote{note: n, src: src})
		}
	}
	return src, nil
}

// addEntry adds the modifiers and rule of authored entry e for a stat of value v, and reports
// whether its rule was applied. Rules act on the sheet's values, which a skill's own stats (slot
// not negative) do not reach, so they are not applied to them.
func (sh *Sheet) addEntry(e *semantics.Entry, v float64, src core.SourceID, slot int) (bool, error) {
	if err := sh.addMods(e, v, src, slot); err != nil {
		return false, err
	}
	if slot >= 0 {
		return false, nil
	}
	return sh.applyRule(e, v, src)
}

// addBaseValues adds the rules' base values of movement speed, damage and efficiencies.
func (sh *Sheet) addBaseValues() error {
	if err := sh.addBaseMovementSpeed(); err != nil {
		return err
	}
	if err := sh.addBaseDamage(); err != nil {
		return err
	}
	return sh.addBaseEfficiencies()
}

// addCount adds counter sv to its own stat and to number_of_allocated_masteries.
func (sh *Sheet) addCount(sv resolve.StatValue, src core.SourceID) error {
	for _, stat := range []string{sv.Stat, statNumberOfAllocatedMasteries} {
		if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[stat], Op: core.OpBase, Value: sv.Value, Source: src}); err != nil {
			return fmt.Errorf("sheet: counter %s: %w", sv.Stat, err)
		}
	}
	return nil
}

// notesFor returns the notes, kind and text only, for a stat whose lookup gave e and ok, none
// when the entry applies in full; ruled says these rules applied the entry's rule. Display-only
// entries need no note: other stats carry their effect. An unchecked condition is noted beside
// any other note, since it can over-apply where the others under-apply.
func notesFor(e *semantics.Entry, ok, ruled bool) []Note {
	switch {
	case !ok:
		return []Note{{Kind: NoEntry}}
	case e.Status == semantics.Unknown:
		return []Note{{Kind: NotApplied, Text: e.Reason.String()}}
	case e.Status != semantics.Authored:
		return nil
	}
	var out []Note
	switch {
	case e.Rule != "" && !ruled:
		out = append(out, Note{Kind: RuleMissing, Text: e.Rule})
	case e.Omitted != "":
		out = append(out, Note{Kind: Partial, Text: e.Omitted})
	}
	if e.Unchecked != "" {
		out = append(out, Note{Kind: Unchecked, Text: e.Unchecked})
	}
	return out
}

// addMods adds e's modifiers for a stat of value from src; a modifier of the skill in slot
// requires the slot's context, and slot is negative outside a skill.
func (sh *Sheet) addMods(e *semantics.Entry, value float64, src core.SourceID, slot int) error {
	for _, m := range e.Mods {
		mod := core.Modifier{Stat: sh.stats[m.Stat], Op: m.Op, Value: m.Factor.Apply(value), Source: src}
		if !m.Context.IsZero() || slot >= 0 {
			p, err := predicate(sh.session.Model(), m.Context, sh.ctx, slot)
			if err != nil {
				return fmt.Errorf("sheet: %s: %w", e.Stat, err)
			}
			mod.Pred = p
		}
		if m.Per != nil {
			mod.Scales = []core.Scale{{Ref: core.SelfRef(sh.stats[m.Per.Stat]), Step: float64(m.Per.Step)}}
		}
		if m.If != nil {
			if !sh.counters[m.If.Stat] {
				// A counter the tree lacks stays 0, below any AtLeast.
				continue
			}
			mod.Conds = []core.Cond{{Ref: core.SelfRef(sh.stats[m.If.Stat]), Cmp: core.CmpGE, Threshold: float64(m.If.AtLeast)}}
		}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: %s: %w", e.Stat, err)
		}
	}
	return nil
}
