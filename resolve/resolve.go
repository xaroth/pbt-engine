// Package resolve is the first pipeline stage: it turns a checked build into stat bundles,
// game stat ids with values grouped by actor, scope and source, plus the edition constants
// the rules read. It looks data up and does arithmetic only; what a stat means is decided
// later, by semantics.
package resolve

import (
	"fmt"
	"io"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// Actor owns a bundle's stats.
type Actor uint8

// Actors.
const (
	Player Actor = iota
)

func (a Actor) String() string {
	if a == Player {
		return "player"
	}
	return "actor?"
}

// Scope says how a bundle's stats apply.
type Scope uint8

// Scopes. Buff stats are auras a node grants; they render with the passive aura scope and
// may reach allies as well as the player.
const (
	Global Scope = iota
	Buff
)

func (s Scope) String() string {
	switch s {
	case Global:
		return "global"
	case Buff:
		return "buff"
	}
	return "scope?"
}

// SourceKind is what a bundle came from.
type SourceKind uint8

// Source kinds.
const (
	FromDataset SourceKind = iota
	FromCharacter
	FromLevel
	FromNode
	FromScenario
)

// Source identifies where a bundle or constant came from.
type Source struct {
	Kind SourceKind
	// ID is the character id, the node id in decimal, or the scenario field name.
	ID string
}

func (s Source) String() string {
	switch s.Kind {
	case FromDataset:
		return "dataset"
	case FromCharacter:
		return "character " + s.ID
	case FromLevel:
		return "level"
	case FromNode:
		return "node " + s.ID
	case FromScenario:
		return "scenario " + s.ID
	}
	return "source?"
}

// StatValue is a game stat id and its value.
type StatValue struct {
	Stat  string
	Value float64
}

// Bundle is the stats one source gives one actor in one scope.
type Bundle struct {
	Actor  Actor
	Scope  Scope
	Source Source
	Stats  []StatValue
}

// Constant is an edition constant, Value per Per, with where its value came from.
type Constant struct {
	ID     string
	Value  int32
	Per    int32
	Source Source
}

// Resolved is the output of the resolve stage. Bundles are in a fixed order: character,
// level, nodes by id (each node's global bundle before its buffs).
type Resolved struct {
	Game, Version string
	Bundles       []Bundle
	Constants     []Constant
	// Notes lists build fields that were not resolved, with the reason.
	Notes []string
}

// ResistancePenalty is the constant the scenario's resistance penalty replaces.
const ResistancePenalty = "resistance_penalty_%"

// characterColumns maps Characters columns to the stat ids that carry them as bundle stats.
var characterColumns = []struct {
	stat string
	get  func(*dataset.Character) int32
}{
	{"base_strength", func(c *dataset.Character) int32 { return c.BaseStrength }},
	{"base_dexterity", func(c *dataset.Character) int32 { return c.BaseDexterity }},
	{"base_intelligence", func(c *dataset.Character) int32 { return c.BaseIntelligence }},
	{"base_maximum_life", func(c *dataset.Character) int32 { return c.BaseMaxLife }},
	{"base_maximum_mana", func(c *dataset.Character) int32 { return c.BaseMaxMana }},
}

// Resolve turns a checked build into bundles and constants.
func Resolve(c *build.Checked) (*Resolved, error) {
	v := c.Version()
	stats, err := v.Stats()
	if err != nil {
		return nil, err
	}
	consts, err := v.Constants()
	if err != nil {
		return nil, err
	}
	values := func(svs []dataset.StatValue, skip string) []StatValue {
		out := make([]StatValue, 0, len(svs))
		for _, sv := range svs {
			if id := stats[sv.Stat].ID; id != skip {
				out = append(out, StatValue{Stat: id, Value: float64(sv.Value)})
			}
		}
		return out
	}
	r := &Resolved{Game: v.Game(), Version: v.Name()}

	ch := c.Character()
	base := make([]StatValue, 0, len(characterColumns)+len(ch.Stats))
	for _, col := range characterColumns {
		base = append(base, StatValue{Stat: col.stat, Value: float64(col.get(ch))})
	}
	// The character's level stat is 1; the build's level replaces it.
	base = append(base, values(ch.Stats, "level")...)
	r.Bundles = append(r.Bundles,
		Bundle{Actor: Player, Scope: Global, Source: Source{Kind: FromCharacter, ID: ch.ID}, Stats: base},
		Bundle{Actor: Player, Scope: Global, Source: Source{Kind: FromLevel}, Stats: []StatValue{{Stat: "level", Value: float64(c.Level())}}},
	)

	for _, n := range c.Nodes() {
		src := Source{Kind: FromNode, ID: fmt.Sprint(n.ID)}
		if len(n.Stats) > 0 {
			r.Bundles = append(r.Bundles, Bundle{Actor: Player, Scope: Global, Source: src, Stats: values(n.Stats, "")})
		}
		for _, b := range n.Buffs {
			r.Bundles = append(r.Bundles, Bundle{Actor: Player, Scope: Buff, Source: src, Stats: values(b, "")})
		}
		if len(n.MasteryEffects) > 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("node %d: mastery effect not chosen; builds have no mastery choices yet", n.ID))
		}
	}

	sc := c.Scenario()
	for _, k := range consts {
		cst := Constant{ID: k.ID, Value: k.Value, Per: k.Per, Source: Source{Kind: FromDataset}}
		if k.ID == ResistancePenalty && sc.Penalty != build.AfterAct10 {
			cst.Value, cst.Per = penaltyValue(sc.Penalty), 1
			cst.Source = Source{Kind: FromScenario, ID: "resistancePenalty"}
		}
		r.Constants = append(r.Constants, cst)
	}
	if sc.Bandit != build.KillAll {
		r.Notes = append(r.Notes, fmt.Sprintf("bandit %s: reward stats are not in the dataset yet", sc.Bandit))
	}
	return r, nil
}

func penaltyValue(p build.Penalty) int32 {
	if p == build.AfterAct5 {
		return -30
	}
	return 0
}

// Dump writes r as text, one line per constant, bundle header and stat.
func (r *Resolved) Dump(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "dataset %s %s\n", r.Game, r.Version)
	for _, c := range r.Constants {
		fmt.Fprintf(&b, "constant %s = %d per %d (%s)\n", c.ID, c.Value, c.Per, c.Source)
	}
	for _, bd := range r.Bundles {
		fmt.Fprintf(&b, "bundle %s %s %s\n", bd.Actor, bd.Scope, bd.Source)
		for _, sv := range bd.Stats {
			fmt.Fprintf(&b, "  %s = %v\n", sv.Stat, sv.Value)
		}
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "note %s\n", n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
