package cluster

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// Description files: items' lines, and passives' stats.
const (
	itemScope = "Metadata/StatDescriptions/stat_descriptions.txt"
	treeScope = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
)

// grantPrefix starts the enchant lines, which name the small passive kind by its node's stat
// lines.
const grantPrefix = "Added Small Passive Skills grant: "

// alsoPrefix starts the stats of jewel lines that add a stat to every small passive; the rest
// of the id is the stat added, unless alsoRenamed names it.
const alsoPrefix = "local_affliction_jewel_small_nodes_grant_"

// alsoRenamed are the stats small passives also grant, by the rest of the jewel stat's id, where
// the two differ. They are the stats the tree's passives give for the same text.
func alsoRenamed() map[string]string {
	return map[string]string{
		"str":                            "base_strength",
		"dex":                            "base_dexterity",
		"int":                            "base_intelligence",
		"all_attributes":                 "additional_all_attributes",
		"maximum_life":                   "base_maximum_life",
		"maximum_mana":                   "base_maximum_mana",
		"maximum_energy_shield":          "base_maximum_energy_shield",
		"armour":                         "base_physical_damage_reduction_rating",
		"evasion":                        "base_evasion_rating",
		"mana_regeneration_+%":           "mana_regeneration_rate_+%",
		"fire_resistance_%":              "base_fire_damage_resistance_%",
		"cold_resistance_%":              "base_cold_damage_resistance_%",
		"lightning_resistance_%":         "base_lightning_damage_resistance_%",
		"chaos_resistance_%":             "base_chaos_damage_resistance_%",
		"elemental_resistance_%":         "base_resist_all_elements_%",
		"%_life_regeneration_per_minute": "life_regeneration_rate_per_minute_%",
	}
}

// Stats of jewel lines that shape the subgraph.
const (
	statNodes = "local_jewel_expansion_passive_node_count"
	// statSockets counts sockets among the nodes; statSocketsAdded adds sockets (Voices).
	statSockets      = "local_jewel_expansion_jewels_count"
	statSocketsAdded = "local_jewel_expansion_jewels_count_override"
	// statEmpty adds small passives that grant nothing (Voices).
	statEmpty = "local_unique_jewel_grants_x_empty_passives"
	// statNothing makes the small passives grant nothing (Megalomaniac).
	statNothing = "local_affliction_jewel_display_small_nodes_grant_nothing"
	statEffect  = "local_affliction_jewel_small_nodes_have_effect_+%"
)

// reading is what a jewel's lines say it adds.
type reading struct {
	// nodes counts the passives, sockets those that are sockets.
	nodes, sockets int32
	// added sockets and empty small passives make up the passives when set (Voices); nothing is
	// non-zero when the small passives grant nothing.
	added, empty, nothing int32
	// effect is the small passives' increased effect, in percent.
	effect int32
	// grants are the enchant's stat lines, which name the small passive kind.
	grants   []string
	skill    *dataset.ClusterSkill
	notables []dataset.ClusterNotable
	// also are the stats every small passive also grants, by handle.
	also map[uint32]int32
	// unread are lines no description reads, and lines adding a stat to small passives that the
	// stats group lacks.
	unread []string
}

// count returns the field of r that stat st sets, or nil.
func (r *reading) count(st string) *int32 {
	switch st {
	case statNodes:
		return &r.nodes
	case statSockets:
		return &r.sockets
	case statSocketsAdded:
		return &r.added
	case statEmpty:
		return &r.empty
	case statNothing:
		return &r.nothing
	case statEffect:
		return &r.effect
	}
	return nil
}

// read reads the lines of a jewel of layout cj.
func (g *Generator) read(cj *dataset.ClusterJewel, lines []string) (reading, error) {
	r := reading{also: map[uint32]int32{}}
	for _, l := range lines {
		if err := g.readLine(&r, l); err != nil {
			return reading{}, err
		}
	}
	if r.added > 0 {
		r.sockets = r.added
		r.nodes = r.empty + r.added
	}
	if r.nodes <= 0 {
		return reading{}, fmt.Errorf("%s: no passive count: %w", cj.Base, ErrUnread)
	}
	if r.nothing == 0 && r.empty == 0 {
		r.skill = g.skill(cj.Size, r.grants)
		if r.skill == nil {
			return reading{}, fmt.Errorf("%s: small passives %q: %w", cj.Base, r.grants, ErrUnread)
		}
	}
	slices.SortStableFunc(r.notables, func(a, b dataset.ClusterNotable) int { return g.order[a.Stat] - g.order[b.Stat] })
	return r, nil
}

// readLine adds line l to r.
func (g *Generator) readLine(r *reading, l string) error {
	if rest, ok := strings.CutPrefix(l, grantPrefix); ok {
		r.grants = append(r.grants, rest)
		return nil
	}
	rs, err := g.set.Read(itemScope, []string{l})
	if err != nil {
		return fmt.Errorf("cluster: %w", err)
	}
	if len(rs) == 0 {
		r.unread = append(r.unread, l)
		return nil
	}
	for i, st := range rs[0].Stats {
		if p := r.count(st); p != nil {
			*p = rs[0].Values[i]
		} else {
			g.readStat(r, l, st, rs[0].Values[i])
		}
	}
	return nil
}

// readStat adds stat st of value v, read from line l, to r: a notable, or a stat every small
// passive also grants. A stat added to small passives that the stats group lacks leaves l
// unread.
func (g *Generator) readStat(r *reading, l, st string, v int32) {
	if rest, ok := strings.CutPrefix(st, alsoPrefix); ok {
		if h, ok := g.also[rest]; ok {
			r.also[h] += v
		} else {
			r.unread = append(r.unread, l)
		}
		return
	}
	if h, ok := g.handles[st]; ok {
		if n, ok := g.notables[h]; ok {
			r.notables = append(r.notables, n)
		}
	}
}

// skill returns the small passive kind of size whose node's stat lines are grants, or nil.
func (g *Generator) skill(size dataset.ClusterSize, grants []string) *dataset.ClusterSkill {
	want := slices.Sorted(slices.Values(grants))
	for i := range g.tree.Clusters.Skills {
		sk := &g.tree.Clusters.Skills[i]
		if sk.Size == size && slices.Equal(g.skillTexts[sk], want) {
			return sk
		}
	}
	return nil
}

// smallStats returns the stats of a small passive of kind sk: its node's, those r adds, with
// r's increased effect, truncated toward zero.
func (g *Generator) smallStats(sk *dataset.ClusterSkill, r *reading) []dataset.StatValue {
	m := maps.Clone(r.also)
	if sk != nil {
		for _, sv := range g.node(sk.Passive).Stats {
			m[sv.Stat] += sv.Value
		}
	}
	out := make([]dataset.StatValue, 0, len(m))
	for _, h := range slices.Sorted(maps.Keys(m)) {
		v := int64(m[h]) * int64(100+r.effect) / 100 //nolint:mnd // percent
		if v != 0 {
			out = append(out, dataset.StatValue{Stat: h, Value: int32(v)}) //nolint:gosec // an increased stat stays in range
		}
	}
	return out
}
