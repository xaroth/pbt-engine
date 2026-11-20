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
// transform. stats are the dataset's stats.
func nodeChanges(stats []dataset.Stat, c *build.Checked) map[uint32][]NodeChange {
	out := map[uint32][]NodeChange{}
	for _, o := range c.Overrides() {
		out[o.Node.ID] = append(out[o.Node.ID], NodeChange{Replace: true, Stats: o.Override.Stats})
	}
	for _, q := range c.Conquests() {
		out[q.Node.ID] = append(out[q.Node.ID], NodeChange{Replace: q.Replace, Stats: q.Stats})
	}
	for _, t := range c.AttributeTransforms() {
		if ch := transformChange(stats, t); len(ch.Stats) > 0 {
			out[t.Node.ID] = append(out[t.Node.ID], ch)
		}
	}
	return out
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
	// changes are what timeless jewels make of the node.
	changes []NodeChange
}

func (g grantedPassive) bundles(stats []dataset.Stat) []Bundle {
	return sourcedBundles(stats, ChangeNode(g.node, g.changes...), g.source)
}

// grantedPassives returns the passives c's jewels make grant nothing while allocated, by node
// id, and those jewels, items, then jewel pairs grant while not allocated: by socket then node
// id, then by slot, then by socket.
func grantedPassives(c *build.Checked) (map[uint32]bool, []grantedPassive) {
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
			grant(RadiusSource{Socket: p.Socket, Node: p.Node.ID}, p.Node)
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
