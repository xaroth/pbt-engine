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

// nodeChanges returns the changes c's build makes to nodes, by node id.
func nodeChanges(c *build.Checked) map[uint32][]NodeChange {
	out := map[uint32][]NodeChange{}
	for _, o := range c.Overrides() {
		out[o.Node.ID] = append(out[o.Node.ID], NodeChange{Replace: true, Stats: o.Override.Stats})
	}
	return out
}
