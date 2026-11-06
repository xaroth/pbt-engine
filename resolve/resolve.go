// Package resolve is the first pipeline stage: it turns a checked build into stat bundles,
// game stat ids with values grouped by actor, scope and source, plus the edition constants
// the rules read. It looks data up and does arithmetic only; what a stat means is decided
// later, by semantics.
package resolve

import (
	"cmp"
	"fmt"
	"io"
	"slices"
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

// Source is where a bundle came from: a CharacterSource, LevelSource, NodeSource or
// QuestSource.
type Source interface {
	String() string
	source()
}

// CharacterSource is the Characters row and object template of a class.
type CharacterSource struct {
	// ID is the Character.ID.
	ID string
}

// LevelSource is the build's character level.
type LevelSource struct{}

// NodeSource is an allocated passive node.
type NodeSource struct {
	ID uint32
}

// QuestSource is a quest reward the build's campaign stage implies.
type QuestSource struct {
	// ID is the game's Quest.Id, such as "a5q6".
	ID string
}

func (s CharacterSource) String() string { return "character " + s.ID }
func (LevelSource) String() string       { return "level" }
func (s NodeSource) String() string      { return fmt.Sprintf("node %d", s.ID) }
func (s QuestSource) String() string     { return "quest " + s.ID }

func (CharacterSource) source() {}
func (LevelSource) source()     {}
func (NodeSource) source()      {}
func (QuestSource) source()     {}

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

// Constant is an edition constant from the dataset, Value per Per.
type Constant struct {
	ID         string
	Value, Per int32
	// Validated says the constant has a game source. Datasets record no source for
	// constants, so it is false for every constant.
	Validated bool
}

// Resolved is the output of the resolve stage. Bundles are in a fixed order: character,
// level, quest rewards in campaign order, nodes by id (each node's global bundle before its
// buffs).
type Resolved struct {
	Game, Version string
	Bundles       []Bundle
	Constants     []Constant
	// Notes lists build fields that were not resolved, with the reason.
	Notes []string
}

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

// levelStat is the stat that carries the character level; the build's level replaces the
// character's own (1).
const levelStat = "level"

// kitavaGame is the game whose campaign kitavaQuests and kitavaAffliction describe.
const kitavaGame = "poe1"

// kitavaQuests are the quests that end with defeating Kitava, in campaign order: The
// Ravenous God (act 5) and An End to Hunger (act 10).
var kitavaQuests = [...]string{"a5q6", "a10q3"}

// kitavaAffliction is the reward of each quest in kitavaQuests. Datasets have no quest
// reward table, so it is held here. Source: the QuestStaticRewards rows of a5q6 and a10q3,
// the same in 3.28.0.16 and 3.29.3.3, whose messages (ClientStrings KitavaWeaken1 and
// KitavaWeaken2) read "-30% to all Resistances" and "a total of -60%".
func kitavaAffliction() []StatValue {
	return []StatValue{{"base_resist_all_elements_%", -30}, {"base_chaos_damage_resistance_%", -30}}
}

func kitavaDefeats(c build.Campaign) int {
	switch c {
	case build.CampaignActs6To10:
		return 1
	case build.CampaignComplete:
		return 2
	}
	return 0
}

// Resolve turns a checked build into bundles and constants. It fails when a dataset group
// fails to load, a stat id resolve uses is not in the dataset's stats, or the build's game has
// no known campaign quest rewards (only poe1 has).
func Resolve(c *build.Checked) (*Resolved, error) {
	s := c.Store()
	m := s.Manifest()
	if m.Game != kitavaGame {
		return nil, fmt.Errorf("resolve: campaign quest rewards of %s are not known", m.Game)
	}
	stats, err := s.Stats()
	if err != nil {
		return nil, err
	}
	consts, err := s.Constants()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(stats))
	for _, st := range stats {
		known[st.ID] = true
	}
	var missing []string
	need := func(id string) string {
		if !known[id] && !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
		return id
	}
	b := c.Build()
	r := &Resolved{Game: m.Game, Version: m.Version}

	ch := c.Character()
	base := make([]StatValue, 0, len(characterColumns)+len(ch.Stats))
	for _, col := range characterColumns {
		base = append(base, StatValue{Stat: need(col.stat), Value: float64(col.get(ch))})
	}
	base = append(base, slices.DeleteFunc(statValues(stats, ch.Stats), func(sv StatValue) bool { return sv.Stat == levelStat })...)
	r.Bundles = append(r.Bundles,
		Bundle{Actor: Player, Scope: Global, Source: CharacterSource{ID: ch.ID}, Stats: base},
		Bundle{Actor: Player, Scope: Global, Source: LevelSource{}, Stats: []StatValue{{Stat: need(levelStat), Value: float64(b.Level)}}},
	)

	for _, q := range kitavaQuests[:kitavaDefeats(b.Scenario.Campaign)] {
		svs := kitavaAffliction()
		for _, sv := range svs {
			need(sv.Stat)
		}
		r.Bundles = append(r.Bundles, Bundle{Actor: Player, Scope: Global, Source: QuestSource{ID: q}, Stats: svs})
	}

	for _, n := range c.Nodes() {
		r.Bundles = append(r.Bundles, nodeBundles(stats, n)...)
		if len(n.MasteryEffects) > 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("node %d: mastery effect not chosen; builds have no mastery choices yet", n.ID))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("resolve: dataset %s has no stat %s", m.Version, strings.Join(missing, ", "))
	}

	for _, k := range consts {
		r.Constants = append(r.Constants, Constant{ID: k.ID, Value: k.Value, Per: k.Per})
	}
	if b.Scenario.Bandits != build.BanditsNotCompleted {
		r.Notes = append(r.Notes, fmt.Sprintf("bandits %s: quest rewards are not in the dataset yet", b.Scenario.Bandits))
	}
	return r, nil
}

// NodeBundles returns the bundles of the passive node with id in the variant tree of s: its
// stats in global scope, if it has any, then each buff in buff scope. The node need not be
// part of a build, so what-if questions can resolve nodes a build does not allocate. It fails
// when the tree or stats fail to load or the tree has no such node.
func NodeBundles(s *dataset.Store, variant string, id uint32) ([]Bundle, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, err
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, err
	}
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil, fmt.Errorf("resolve: dataset %s has no node %d in the %s tree", s.Manifest().Version, id, variant)
	}
	return nodeBundles(stats, &tree.Nodes[i]), nil
}

// nodeBundles is NodeBundles of n, whose stat handles index stats.
func nodeBundles(stats []dataset.Stat, n *dataset.Node) []Bundle {
	src := NodeSource{ID: n.ID}
	var out []Bundle
	if len(n.Stats) > 0 {
		out = append(out, Bundle{Actor: Player, Scope: Global, Source: src, Stats: statValues(stats, n.Stats)})
	}
	for _, bf := range n.Buffs {
		out = append(out, Bundle{Actor: Player, Scope: Buff, Source: src, Stats: statValues(stats, bf)})
	}
	return out
}

func statValues(stats []dataset.Stat, svs []dataset.StatValue) []StatValue {
	out := make([]StatValue, 0, len(svs))
	for _, sv := range svs {
		out = append(out, StatValue{Stat: stats[sv.Stat].ID, Value: float64(sv.Value)})
	}
	return out
}

// Dump writes r as text, one line per constant, bundle header and stat.
func (r *Resolved) Dump(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "dataset %s %s\n", r.Game, r.Version)
	for _, c := range r.Constants {
		status := "unvalidated"
		if c.Validated {
			status = "validated"
		}
		fmt.Fprintf(&b, "constant %s = %d per %d (%s)\n", c.ID, c.Value, c.Per, status)
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
