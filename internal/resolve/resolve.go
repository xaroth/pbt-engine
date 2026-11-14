// Package resolve is the first pipeline stage: it turns a checked build into stat bundles,
// game stat ids with values grouped by actor, scope and source, plus the edition constants
// the rules read. It looks data up and does arithmetic only; what a stat means is decided
// later, by semantics.
package resolve

import (
	"cmp"
	"fmt"
	"maps"
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

// Source is where a bundle came from: a CharacterSource, LevelSource, NodeSource,
// QuestSource, ItemSource or JewelSource.
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

// NodeSource is a passive node, or with Effect one of a mastery node's effects.
type NodeSource struct {
	ID uint32
	// Effect is the mastery effect id; 0 for the node's own stats.
	Effect uint32
}

// QuestSource is a quest reward from the dataset. Resolve returns every reward with stats; the
// rules decide from the build's config which ones the character has.
type QuestSource struct {
	// ID is the game's Quest.Id, such as "a5q6"; empty for a reward no quest owns.
	ID string
	// Flag is the QuestFlags.Id that grants the reward, such as "a5q6-CompletedBoss".
	Flag string
}

func (s CharacterSource) String() string { return "character " + s.ID }
func (LevelSource) String() string       { return "level" }
func (s NodeSource) String() string {
	if s.Effect != 0 {
		return fmt.Sprintf("node %d effect %d", s.ID, s.Effect)
	}
	return fmt.Sprintf("node %d", s.ID)
}
func (s QuestSource) String() string { return "quest " + s.ID }

// ItemSource is an equipped item. Resolve returns none; the rules compile items themselves. A
// skill's support an item grants has it.
type ItemSource struct {
	Slot build.Slot
	// Name is a rare or unique item's name, or a magic item's full name; empty for others.
	Name string
}

func (s ItemSource) String() string {
	if s.Name != "" {
		return "item " + string(s.Slot) + ": " + s.Name
	}
	return "item " + string(s.Slot)
}

// JewelSource is a jewel in a tree socket. Resolve returns none; the rules compile jewels as
// items.
type JewelSource struct {
	// Node is the jewel socket node's id.
	Node uint32
	// Name is as ItemSource's.
	Name string
}

func (s JewelSource) String() string {
	if s.Name != "" {
		return fmt.Sprintf("jewel %d: %s", s.Node, s.Name)
	}
	return fmt.Sprintf("jewel %d", s.Node)
}

func (CharacterSource) source() {}
func (ItemSource) source()      {}
func (JewelSource) source()     {}
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
}

// Resolved is the output of the resolve stage. Bundles are in a fixed order: character,
// level, quest rewards in campaign order, nodes by id (each node's global bundle before its
// buffs).
type Resolved struct {
	Game, Version string
	// Capabilities are the dataset's rule versions; rules pick formulas by them.
	Capabilities dataset.Capabilities
	Bundles      []Bundle
	Constants    []Constant
	// Counters are the stats the tree counts rather than gives: each mastery group's count
	// stat, which every mastery of the group carries at 1. Sorted.
	Counters []string
}

// CharacterColumn is a Characters column and the stat id that carries it in a character's
// bundle.
type CharacterColumn struct {
	Column, Stat string
	get          func(*dataset.Character) int32
}

// CharacterColumns returns the Characters columns resolve carries as stats, in bundle order.
func CharacterColumns() []CharacterColumn {
	return []CharacterColumn{
		{"BaseStrength", "base_strength", func(c *dataset.Character) int32 { return c.BaseStrength }},
		{"BaseDexterity", "base_dexterity", func(c *dataset.Character) int32 { return c.BaseDexterity }},
		{"BaseIntelligence", "base_intelligence", func(c *dataset.Character) int32 { return c.BaseIntelligence }},
		{"BaseMaxLife", "base_maximum_life", func(c *dataset.Character) int32 { return c.BaseMaxLife }},
		{"BaseMaxMana", "base_maximum_mana", func(c *dataset.Character) int32 { return c.BaseMaxMana }},
	}
}

// LevelStat is the stat that carries the character level. The level bundle carries it with
// the build's level; the character's own (1) is dropped.
const LevelStat = "level"

// CharacterStats returns the stats of the bundle of the character with id in s: the
// CharacterColumns, then its object template stats without LevelStat. It fails when a group
// fails to load or s has no such character.
func CharacterStats(s *dataset.Store, id string) ([]StatValue, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	chars, err := s.Characters()
	if err != nil {
		return nil, fmt.Errorf("resolve: load characters: %w", err)
	}
	i := slices.IndexFunc(chars, func(c dataset.Character) bool { return c.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("resolve: dataset %s has no character %s: %w", s.Manifest().Version, id, dataset.ErrNotInDataset)
	}
	return characterStats(stats, &chars[i]), nil
}

// characterStats is CharacterStats of ch, whose stat handles index stats.
func characterStats(stats []dataset.Stat, ch *dataset.Character) []StatValue {
	cols := CharacterColumns()
	out := make([]StatValue, 0, len(cols)+len(ch.Stats))
	for _, col := range cols {
		out = append(out, StatValue{Stat: col.Stat, Value: float64(col.get(ch))})
	}
	return append(out, slices.DeleteFunc(statValues(stats, ch.Stats), func(sv StatValue) bool { return sv.Stat == LevelStat })...)
}

// QuestBundles returns a bundle for each quest reward of s with stats, in dataset order.
func QuestBundles(s *dataset.Store) ([]Bundle, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	qs, err := s.Quests()
	if err != nil {
		return nil, fmt.Errorf("resolve: load quests: %w", err)
	}
	var out []Bundle
	for _, q := range qs {
		if len(q.Stats) > 0 {
			out = append(out, Bundle{Actor: Player, Scope: Global, Source: QuestSource{ID: q.Quest, Flag: q.Flag}, Stats: statValues(stats, q.Stats)})
		}
	}
	return out, nil
}

// Resolve turns a checked build into bundles and constants: character, level, every quest
// reward with stats (the rules pick the ones the build's config grants), then the allocated
// nodes as the build's overrides change them (ChangeNode), each followed by its chosen mastery
// effect. It fails when a dataset group fails to load or a stat id resolve uses is not in the
// dataset's stats.
func Resolve(c *build.Checked) (*Resolved, error) {
	return resolve(c, false)
}

// Tree is Resolve with the bundles of every node the build's class and ascendancy can
// allocate, and of every mastery effect, allocated or not, in tree order. A sheet over it
// turns sources on and off to match an allocation, so edits and what-ifs need no new bundles.
func Tree(c *build.Checked) (*Resolved, error) {
	return resolve(c, true)
}

func resolve(c *build.Checked, wholeTree bool) (*Resolved, error) {
	s := c.Store()
	m := s.Manifest()
	b := c.Build()
	quests, err := QuestBundles(s)
	if err != nil {
		return nil, err
	}
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	consts, err := s.Constants()
	if err != nil {
		return nil, fmt.Errorf("resolve: load constants: %w", err)
	}
	r := &Resolved{Game: m.Game, Version: m.Version, Capabilities: maps.Clone(m.Capabilities), Counters: counters(stats, c.Tree())}

	ch := c.Character()
	r.Bundles = append(r.Bundles,
		Bundle{Actor: Player, Scope: Global, Source: CharacterSource{ID: ch.ID}, Stats: characterStats(stats, ch)},
		Bundle{Actor: Player, Scope: Global, Source: LevelSource{}, Stats: []StatValue{{Stat: LevelStat, Value: float64(b.Level)}}},
	)
	r.Bundles = append(r.Bundles, quests...)
	missing := missingStats(r.Bundles, stats)

	if wholeTree {
		r.Bundles = append(r.Bundles, treeBundles(stats, c)...)
	} else {
		r.Bundles = append(r.Bundles, allocatedBundles(stats, c)...)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("resolve: dataset %s has no stat %s: %w", m.Version, strings.Join(missing, ", "), dataset.ErrNotInDataset)
	}

	for _, k := range consts {
		r.Constants = append(r.Constants, Constant{ID: k.ID, Value: k.Value, Per: k.Per})
	}
	return r, nil
}

func allocatedBundles(stats []dataset.Stat, c *build.Checked) []Bundle {
	choices := c.Masteries()
	changes := nodeChanges(c)
	var out []Bundle
	for _, n := range c.Nodes() {
		if ch, ok := changes[n.ID]; ok {
			n = ChangeNode(n, ch...)
		}
		out = append(out, nodeBundles(stats, n)...)
		if i := slices.IndexFunc(choices, func(m build.MasteryChoice) bool { return m.Node.ID == n.ID }); i >= 0 {
			out = append(out, masteryBundle(stats, n, choices[i].Effect))
		}
	}
	return out
}

// treeBundles returns the bundles of every node c's class, ascendancy and bloodline can
// allocate: not other ascendancies' nodes or other classes' starts.
func treeBundles(stats []dataset.Stat, c *build.Checked) []Bundle {
	changes := nodeChanges(c)
	var out []Bundle
	for i := range c.Tree().Nodes {
		n := &c.Tree().Nodes[i]
		if !c.OwnsAscendancy(n.Ascendancy) || (n.Kind == dataset.ClassStart && n.ID != c.Class().StartNode) {
			continue
		}
		if ch, ok := changes[n.ID]; ok {
			n = ChangeNode(n, ch...)
		}
		out = append(out, nodeBundles(stats, n)...)
		for _, e := range n.MasteryEffects {
			out = append(out, masteryBundle(stats, n, e))
		}
	}
	return out
}

// missingStats lists the stats of bundles absent from stats. Only stats resolve names itself
// can be missing; node and template stats come from the dataset's stats.
func missingStats(bundles []Bundle, stats []dataset.Stat) []string {
	known := make(map[string]bool, len(stats))
	for _, st := range stats {
		known[st.ID] = true
	}
	var missing []string
	for _, bd := range bundles {
		for _, sv := range bd.Stats {
			if !known[sv.Stat] && !slices.Contains(missing, sv.Stat) {
				missing = append(missing, sv.Stat)
			}
		}
	}
	return missing
}

// NodeBundles returns the bundles of the passive node with id in the variant tree of s: its
// stats in global scope, if it has any, then each buff in buff scope. The node need not be
// part of a build, so what-if questions can resolve nodes a build does not allocate. It fails
// when the tree or stats fail to load or the tree has no such node.
func NodeBundles(s *dataset.Store, variant string, id uint32) ([]Bundle, error) {
	stats, n, err := node(s, variant, id)
	if err != nil {
		return nil, err
	}
	return nodeBundles(stats, n), nil
}

// MasteryEffectBundle returns the bundle that the mastery effect numbered effect of the node with id in the
// variant tree of s gives once chosen: its stats in global scope, from the node. It fails when
// the tree or stats fail to load or the node has no such effect.
func MasteryEffectBundle(s *dataset.Store, variant string, id, effect uint32) (Bundle, error) {
	stats, n, err := node(s, variant, id)
	if err != nil {
		return Bundle{}, err
	}
	i := slices.IndexFunc(n.MasteryEffects, func(e dataset.MasteryEffect) bool { return e.ID == effect })
	if i < 0 {
		return Bundle{}, fmt.Errorf("resolve: node %d has no mastery effect %d: %w", id, effect, dataset.ErrNotInDataset)
	}
	return masteryBundle(stats, n, n.MasteryEffects[i]), nil
}

// node returns the node with id in the variant tree of s and the stats its handles index.
func node(s *dataset.Store, variant string, id uint32) ([]dataset.Stat, *dataset.Node, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve: load tree: %w", err)
	}
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil, nil, fmt.Errorf("resolve: dataset %s has no node %d in the %s tree: %w", s.Manifest().Version, id, variant, dataset.ErrNotInDataset)
	}
	return stats, &tree.Nodes[i], nil
}

// Counters returns the counters of the variant tree of s, as Resolved.Counters.
func Counters(s *dataset.Store, variant string) ([]string, error) {
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("resolve: load stats: %w", err)
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("resolve: load tree: %w", err)
	}
	return counters(stats, tree), nil
}

// counters returns the stats of tree's masteries: a mastery's only stat is its group's count.
func counters(stats []dataset.Stat, tree *dataset.Tree) []string {
	var out []string
	for i := range tree.Nodes {
		if n := &tree.Nodes[i]; n.Kind == dataset.Mastery {
			for _, sv := range n.Stats {
				out = append(out, stats[sv.Stat].ID)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
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

func masteryBundle(stats []dataset.Stat, n *dataset.Node, e dataset.MasteryEffect) Bundle {
	return Bundle{Actor: Player, Scope: Global, Source: NodeSource{ID: n.ID, Effect: e.ID}, Stats: statValues(stats, e.Stats)}
}

func statValues(stats []dataset.Stat, svs []dataset.StatValue) []StatValue {
	out := make([]StatValue, 0, len(svs))
	for _, sv := range svs {
		out = append(out, StatValue{Stat: stats[sv.Stat].ID, Value: float64(sv.Value)})
	}
	return out
}
