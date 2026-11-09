package poe1

import (
	"errors"
	"fmt"

	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/semantics"
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
)

// NoteKind says why a bundle stat is not, or not fully, in the sheet.
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
	// sources describes each core source, by core.SourceID.
	sources []Source
	version string
	// Notes lists the bundle stats left out of the sheet, in bundle order.
	Notes []Note
}

// Open builds the sheet of r: each bundle stat goes through t to engine modifiers, one source
// per bundle, then the rules add theirs, such as the attribute bonuses from r's constants.
// Stats t cannot apply become notes, not errors. It fails when r is for another game, its
// version does not parse, its dataset needs a rule version these rules do not implement, a
// bundle is for an actor other than the player, or a constant is missing.
func Open(r *resolve.Resolved, t *semantics.Table) (*Sheet, error) {
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
	m, stats := model()
	s, err := core.NewSession(m, 1)
	if err != nil {
		return nil, fmt.Errorf("sheet: %w", err)
	}
	sh := &Sheet{session: s, stats: stats, version: r.Version}
	for i := range r.Bundles {
		if err := sh.add(&r.Bundles[i], t, v); err != nil {
			return nil, err
		}
	}
	if err := sh.addAttributeBonuses(r.Constants); err != nil {
		return nil, err
	}
	s.Commit()
	return sh, nil
}

// Value returns the value of engine stat name.
func (sh *Sheet) Value(name string) (float64, error) {
	id, ok := sh.stats[name]
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownStat, name)
	}
	val, err := sh.session.Get(0, id, core.GlobalCtx)
	if err != nil {
		return 0, fmt.Errorf("sheet: %s: %w", name, err)
	}
	return val, nil
}

// newSource adds a core source described by d.
func (sh *Sheet) newSource(d Source) core.SourceID {
	sh.sources = append(sh.sources, d)
	return sh.session.NewSource()
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

func model() (*core.Model, map[string]core.StatID) {
	var b core.ModelBuilder
	stats := map[string]core.StatID{}
	for _, name := range engineStats() {
		stats[name] = b.MustAddStat(statDef(name, stats))
	}
	addTransferStats(&b, stats)
	return b.Build(), stats
}

func (sh *Sheet) add(bd *resolve.Bundle, t *semantics.Table, v dataset.GameVersion) error {
	// Every modifier lands on core actor 0, so another actor's stats would stack onto the player.
	if bd.Actor != resolve.Player {
		return fmt.Errorf("sheet: %s bundle %s: %w", bd.Actor, bd.Source, ErrActor)
	}
	b := *bd
	src := sh.newSource(Source{Bundle: &b})
	for _, sv := range bd.Stats {
		e, ok := t.Lookup(sv.Stat, bd.Scope, v)
		ruled := false
		if ok && e.Status == semantics.Authored {
			if err := sh.addMods(e, sv.Value, src); err != nil {
				return err
			}
			var err error
			if ruled, err = sh.applyRule(e, sv.Value, src); err != nil {
				return err
			}
		}
		if kind, text := noteFor(e, ok, ruled); kind != 0 {
			sh.Notes = append(sh.Notes, Note{Kind: kind, Source: bd.Source, Scope: bd.Scope, Stat: sv.Stat, Text: text})
		}
	}
	return nil
}

// noteFor returns the note for a stat whose lookup gave e and ok, or kind 0 when the entry
// applies in full; ruled says these rules applied the entry's rule. Display-only entries need
// no note: other stats carry their effect.
func noteFor(e *semantics.Entry, ok, ruled bool) (NoteKind, string) {
	switch {
	case !ok:
		return NoEntry, ""
	case e.Status == semantics.Unknown:
		return NotApplied, e.Reason.String()
	case e.Status != semantics.Authored:
		return 0, ""
	case e.Rule != "" && !ruled:
		return RuleMissing, e.Rule
	case e.Omitted != "":
		return Partial, e.Omitted
	}
	return 0, ""
}

func (sh *Sheet) addMods(e *semantics.Entry, value float64, src core.SourceID) error {
	for _, m := range e.Mods {
		mod := core.Modifier{Stat: sh.stats[m.Stat], Op: m.Op, Value: m.Factor.Apply(value), Source: src}
		if m.Per != nil {
			mod.Scales = []core.Scale{{Ref: core.SelfRef(sh.stats[m.Per.Stat]), Step: float64(m.Per.Step)}}
		}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: %s: %w", e.Stat, err)
		}
	}
	return nil
}
