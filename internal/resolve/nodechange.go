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
// stat the node already has adds to its value.
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
				out.Stats[i].Value += sv.Value
			} else {
				out.Stats = append(out.Stats, sv)
			}
		}
	}
	return &out
}

// nodeChanges returns the changes c's build makes to nodes, by node id: its tattoo or
// runegraft first, then what its timeless jewels make of it.
func nodeChanges(c *build.Checked) map[uint32][]NodeChange {
	out := map[uint32][]NodeChange{}
	for _, o := range c.Overrides() {
		out[o.Node.ID] = append(out[o.Node.ID], NodeChange{Replace: true, Stats: o.Override.Stats})
	}
	for _, q := range c.Conquests() {
		out[q.Node.ID] = append(out[q.Node.ID], NodeChange{Replace: q.Replace, Stats: q.Stats})
	}
	return out
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
// id, and those jewels then items grant while not allocated: by socket then node id, then by
// slot.
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
	return nothing, granted
}
