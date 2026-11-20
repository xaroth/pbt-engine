package resolve

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// NodeChange changes what a node gives before simulation: a tattoo or runegraft replaces its
// stats; a timeless jewel replaces them or adds to them.
type NodeChange struct {
	// Replace makes Stats take the place of the node's stats, buffs and mastery effects;
	// otherwise Stats are added to its stats.
	Replace bool
	Stats   []dataset.StatValue
}

// ChangeNode returns a copy of n with changes applied in order; n is not modified. An added
// stat the node already has adds to its value, and the node loses it when that comes to 0.
func ChangeNode(n *dataset.Node, changes ...NodeChange) *dataset.Node {
	out := *n
	out.Stats = slices.Clone(n.Stats)
	for _, ch := range changes {
		if ch.Replace {
			out.Stats, out.Buffs, out.MasteryEffects = slices.Clone(ch.Stats), nil, nil
			continue
		}
		for _, sv := range ch.Stats {
			if i := slices.IndexFunc(out.Stats, func(o dataset.StatValue) bool { return o.Stat == sv.Stat }); i >= 0 {
				if out.Stats[i].Value += sv.Value; out.Stats[i].Value == 0 {
					out.Stats = slices.Delete(out.Stats, i, i+1)
				}
			} else {
				out.Stats = append(out.Stats, sv)
			}
		}
	}
	return &out
}

// nodeChanges returns the changes c's build makes to nodes, by node id: its tattoo or
// runegraft first, then what its timeless jewels make of it, then the attributes its jewels
// transform and what other radius jewels make the node give, then the attributes Transcendent
// jewels count on it. It also returns the changes to what a node gives while not allocated:
// without a tattoo or runegraft, which only allocated passives keep, and without Transcendent
// counts, which only allocated passives give. stats are the dataset's stats.
func nodeChanges(stats []dataset.Stat, c *build.Checked) (all, unallocated map[uint32][]NodeChange) {
	return changesOf(stats, c, true), changesOf(stats, c, false)
}

// changesOf returns nodeChanges' changes to allocated nodes, or with allocated unset to
// unallocated ones.
func changesOf(stats []dataset.Stat, c *build.Checked, allocated bool) map[uint32][]NodeChange {
	out := map[uint32][]NodeChange{}
	if allocated {
		for _, o := range c.Overrides() {
			addChange(out, o.Node.ID, NodeChange{Replace: true, Stats: o.Override.Stats})
		}
	}
	for _, q := range c.Conquests() {
		addChange(out, q.Node.ID, NodeChange{Replace: q.Replace, Stats: q.Stats})
	}
	for _, t := range c.AttributeTransforms() {
		addChange(out, t.Node.ID, transformChange(stats, t))
	}
	var counted []build.RadiusChange
	for _, rc := range c.RadiusChanges() {
		if build.PerAttributeOf(rc.Stat) != 0 {
			counted = append(counted, rc)
			continue
		}
		addChange(out, rc.Node.ID, radiusChange(stats, rc, ChangeNode(rc.Node, out[rc.Node.ID]...)))
	}
	if allocated {
		addCounts(stats, out, counted)
	}
	return out
}

// addChange adds ch to node id's changes in out unless it changes nothing.
func addChange(out map[uint32][]NodeChange, id uint32, ch NodeChange) {
	if ch.Replace || len(ch.Stats) > 0 {
		out[id] = append(out[id], ch)
	}
}

// addCounts adds the changes of the Transcendent jewel stats counted to out. Each jewel counts
// the attribute the node gives after the changes in out, not after another's count.
func addCounts(stats []dataset.Stat, out map[uint32][]NodeChange, counted []build.RadiusChange) {
	counts := make([]NodeChange, len(counted))
	for i, rc := range counted {
		counts[i] = perAttributeChange(stats, rc, ChangeNode(rc.Node, out[rc.Node.ID]...))
	}
	for i, rc := range counted {
		addChange(out, rc.Node.ID, counts[i])
	}
}

// Healthy Mind moves increases to maximum life to maximum mana at manaPerLife times their value.
const (
	lifeIncreaseStat = "maximum_life_+%"
	manaIncreaseStat = "maximum_mana_+%"
	manaPerLife      = 2
)

// radiusChange returns the change rc's jewel stat makes to n, rc's passive as its tattoo or
// runegraft and attribute transforms leave it: The Light of Meaning adds its stat, and Healthy
// Mind moves increases to maximum life to maximum mana at twice their value.
func radiusChange(stats []dataset.Stat, rc build.RadiusChange, n *dataset.Node) NodeChange {
	if granted, ok := build.RadiusGrantOf(rc.Stat); ok {
		h, ok := statHandle(stats, granted)
		if !ok {
			return NodeChange{}
		}
		return NodeChange{Stats: []dataset.StatValue{{Stat: h, Value: rc.Value}}}
	}
	if rc.Stat != build.LifeToManaDoubledStat {
		return NodeChange{}
	}
	mana, ok := statHandle(stats, manaIncreaseStat)
	if !ok {
		return NodeChange{}
	}
	var ch NodeChange
	for _, sv := range n.Stats {
		if stats[sv.Stat].ID == lifeIncreaseStat {
			ch.Stats = append(ch.Stats, dataset.StatValue{Stat: sv.Stat, Value: -sv.Value}, dataset.StatValue{Stat: mana, Value: manaPerLife * sv.Value})
		}
	}
	return ch
}

// perAttributeChange returns the change rc's Transcendent jewel stat makes to n, rc's passive as
// changed so far: its value of the attribute per 1 of it n gives, counted where n is allocated.
func perAttributeChange(stats []dataset.Stat, rc build.RadiusChange, n *dataset.Node) NodeChange {
	a := build.PerAttributeOf(rc.Stat)
	var total int32
	for _, sv := range n.Stats {
		if slices.Contains(attributesOf(stats[sv.Stat].ID), a) {
			total += sv.Value
		}
	}
	h, ok := statHandle(stats, baseAttributeStat(a))
	if total == 0 || !ok {
		return NodeChange{}
	}
	return NodeChange{Stats: []dataset.StatValue{{Stat: h, Value: rc.Value * total}}}
}

// transformChange returns the change that makes t's passive's stats that give t.From give t.To
// instead. A stat that gives t.From among other attributes gives the others as before. It reads
// the passive as the tree has it, so jewels that transform one passive do not chain.
func transformChange(stats []dataset.Stat, t build.AttributeTransform) NodeChange {
	var ch NodeChange
	for _, sv := range t.Node.Stats {
		attrs := attributesOf(stats[sv.Stat].ID)
		if !slices.Contains(attrs, t.From) {
			continue
		}
		ch.Stats = append(ch.Stats, dataset.StatValue{Stat: sv.Stat, Value: -sv.Value})
		for _, a := range attrs {
			if a == t.From {
				a = t.To
			}
			h, ok := statHandle(stats, baseAttributeStat(a))
			if !ok {
				return NodeChange{}
			}
			ch.Stats = append(ch.Stats, dataset.StatValue{Stat: h, Value: sv.Value})
		}
	}
	return ch
}

// statHandle returns the handle of stat id among stats, and reports whether it is there.
func statHandle(stats []dataset.Stat, id string) (uint32, bool) {
	for h, st := range stats {
		if st.ID == id {
			return uint32(h), true
		}
	}
	return 0, false
}

// Stats that add to one attribute alone.
const (
	baseStrengthStat     = "base_strength"
	baseDexterityStat    = "base_dexterity"
	baseIntelligenceStat = "base_intelligence"
)

// attributesOf returns the attributes the passive stat id adds to.
func attributesOf(id string) []build.Attribute {
	switch id {
	case baseStrengthStat, "additional_strength":
		return []build.Attribute{build.Strength}
	case baseDexterityStat, "additional_dexterity":
		return []build.Attribute{build.Dexterity}
	case baseIntelligenceStat, "additional_intelligence":
		return []build.Attribute{build.Intelligence}
	case "base_strength_and_dexterity", "additional_strength_and_dexterity":
		return []build.Attribute{build.Strength, build.Dexterity}
	case "base_strength_and_intelligence", "additional_strength_and_intelligence":
		return []build.Attribute{build.Strength, build.Intelligence}
	case "base_dexterity_and_intelligence", "additional_dexterity_and_intelligence":
		return []build.Attribute{build.Dexterity, build.Intelligence}
	case "additional_all_attributes":
		return []build.Attribute{build.Strength, build.Dexterity, build.Intelligence}
	}
	return nil
}

// baseAttributeStat returns the passive stat that adds to a alone.
func baseAttributeStat(a build.Attribute) string {
	switch a {
	case build.Strength:
		return baseStrengthStat
	case build.Dexterity:
		return baseDexterityStat
	case build.Intelligence:
		return baseIntelligenceStat
	}
	return ""
}

// grantedPassive is a passive a jewel or an item grants while it is not allocated.
type grantedPassive struct {
	source Source
	node   *dataset.Node
	// changes are what jewels make of the node.
	changes []NodeChange
}

func (g grantedPassive) bundles(stats []dataset.Stat) []Bundle {
	return sourcedBundles(stats, ChangeNode(g.node, g.changes...), g.source)
}

// grantedPassives returns the passives c's jewels make grant nothing while allocated, by node
// id, and those jewels, items, then jewel pairs grant while not allocated: by socket then node
// id, then by slot, then by socket. A passive a jewel grants from its radius gives what
// unallocated, the changes to an unallocated node by id, makes of it; the others what timeless
// jewels make of them.
func grantedPassives(c *build.Checked, unallocated map[uint32][]NodeChange) (map[uint32]bool, []grantedPassive) {
	nothing := map[uint32]bool{}
	var granted []grantedPassive
	conquests := c.Conquests()
	grant := func(src Source, n *dataset.Node) {
		g := grantedPassive{source: src, node: n}
		for _, q := range conquests {
			if q.Node.ID == n.ID {
				g.changes = append(g.changes, NodeChange{Replace: q.Replace, Stats: q.Stats})
			}
		}
		granted = append(granted, g)
	}
	for _, p := range c.RadiusPassives() {
		if p.Nothing {
			nothing[p.Node.ID] = true
		}
		if p.Granted {
			granted = append(granted, grantedPassive{source: RadiusSource{Socket: p.Socket, Node: p.Node.ID}, node: p.Node, changes: unallocated[p.Node.ID]})
		}
	}
	for _, p := range c.ItemPassives() {
		grant(ItemSource{Slot: p.Slot, Name: p.Name, Passive: p.Node.ID}, p.Node)
	}
	for _, p := range c.JewelPassives() {
		grant(JewelSource{Node: p.Socket, Name: p.Name, Passive: p.Node.ID, With: p.With}, p.Node)
	}
	return nothing, granted
}
