package build

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// ChangeClass returns the edits that set the class, clear the ascendancy and deallocate what no
// longer fits: the old class start, the ascendancy's nodes, and nodes no longer joined to the
// new class start, with their mastery choices. Nodes not joined before stay. It reports false
// when class is not a class of the tree.
func (c *Checked) ChangeClass(class string) ([]Edit, bool) {
	i := slices.IndexFunc(c.tree.Classes, func(cl dataset.Class) bool { return cl.Character == class })
	if i < 0 {
		return nil, false
	}
	edits := []Edit{SetClass(class)}
	if c.build.Ascendancy != "" {
		edits = append(edits, SetAscendancy(""))
	}
	gone, _ := c.regroup(c.tree.Classes[i].StartNode, "")
	return append(edits, c.deallocate(gone)...), true
}

// ChangeAscendancy returns the edits that set the ascendancy, empty to clear it, deallocate the
// nodes of the old one, with their mastery choices, and allocate the new one's start. It
// reports false when ascendancy is not one of the class's.
func (c *Checked) ChangeAscendancy(ascendancy string) ([]Edit, bool) {
	if ascendancy != "" && !slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == ascendancy }) {
		return nil, false
	}
	gone, start := c.regroup(c.class.StartNode, ascendancy)
	edits := append([]Edit{SetAscendancy(ascendancy)}, c.deallocate(gone)...)
	if start != nil && !slices.Contains(c.build.Nodes, start.ID) {
		edits = append(edits, Allocate(start.ID))
	}
	return edits, true
}

// regroup returns the allocated nodes that do not fit class start and ascendancy, or were
// joined to the old starts and are not to the new ones, and the ascendancy's start, if any.
func (c *Checked) regroup(start uint32, ascendancy string) ([]uint32, *dataset.Node) {
	starts := map[uint32]bool{start: true}
	ascStart := c.ascendancyStart(ascendancy)
	if ascStart != nil {
		starts[ascStart.ID] = true
	}
	fits := c.fitting(start, ascendancy)
	maps.Copy(fits, starts)
	before := c.joined(c.allocatedSet())
	after := c.reach(slices.Sorted(maps.Keys(starts)), fits)
	var gone []uint32
	for _, n := range c.nodes {
		if !fits[n.ID] || (before[n.ID] && !after[n.ID]) {
			gone = append(gone, n.ID)
		}
	}
	return gone, ascStart
}

// fitting returns the allocated nodes a build of class start and ascendancy can take.
func (c *Checked) fitting(start uint32, ascendancy string) map[uint32]bool {
	out := map[uint32]bool{}
	for _, n := range c.nodes {
		if (n.Kind != dataset.ClassStart || n.ID == start) && (n.Ascendancy == "" || n.Ascendancy == ascendancy) {
			out[n.ID] = true
		}
	}
	return out
}
