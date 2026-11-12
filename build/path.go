package build

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// AllocatePath returns the edit that allocates node with the shortest path of unallocated nodes
// joining it to the allocated nodes or the class or ascendancy start, node last. A path takes
// no class start, no node of another ascendancy (their starts included) and no option of a
// choice node that already takes one, and ends at a mastery: a mastery's edges say which
// notables unlock it. It reports false when node is allocated or a start, or no path reaches it.
func (c *Checked) AllocatePath(node uint32) (Edit, bool) {
	seeds := c.starts()
	maps.Copy(seeds, c.allocatedSet())
	if seeds[node] {
		return Edit{}, false
	}
	taken := c.takenChoices()
	queue := slices.Sorted(maps.Keys(seeds))
	parent := make(map[uint32]uint32, len(queue))
	for _, id := range queue {
		parent[id] = id
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == node {
			return Allocate(pathTo(id, parent)...), true
		}
		n := c.node(id)
		if n == nil || n.Kind == dataset.Mastery {
			continue
		}
		for _, nb := range n.Neighbours {
			if _, seen := parent[nb]; seen || !c.takeable(c.node(nb), taken) {
				continue
			}
			parent[nb] = id
			queue = append(queue, nb)
		}
	}
	return Edit{}, false
}

// pathTo follows parent from id to a seed, its own parent, and returns the nodes on the way,
// id last.
func pathTo(id uint32, parent map[uint32]uint32) []uint32 {
	var path []uint32
	for n := id; parent[n] != n; n = parent[n] {
		path = append(path, n)
	}
	slices.Reverse(path)
	return path
}

// takenChoices returns the choice nodes with an allocated option.
func (c *Checked) takenChoices() map[uint32]bool {
	taken := map[uint32]bool{}
	for _, n := range c.nodes {
		if p := c.choiceOf(n); p != nil {
			taken[p.ID] = true
		}
	}
	return taken
}

// takeable reports whether a path may enter n; taken holds the choice nodes with an option.
func (c *Checked) takeable(n *dataset.Node, taken map[uint32]bool) bool {
	if n == nil || n.Kind == dataset.ClassStart || (n.Ascendancy != "" && n.Ascendancy != c.build.Ascendancy) {
		return false
	}
	p := c.choiceOf(n)
	return p == nil || !taken[p.ID]
}

// DeallocateBranch returns the edits that deallocate node and the allocated nodes joined to the
// class or ascendancy start only through it, and clear the mastery choices of those. Nodes not
// joined before stay, as jewels can make them legal. It reports false when node is not
// allocated.
func (c *Checked) DeallocateBranch(node uint32) ([]Edit, bool) {
	allocated := c.allocatedSet()
	if !allocated[node] {
		return nil, false
	}
	before := c.joined(allocated)
	delete(allocated, node)
	after := c.joined(allocated)
	gone := []uint32{node}
	for id := range allocated {
		if before[id] && !after[id] {
			gone = append(gone, id)
		}
	}
	return c.deallocate(gone), true
}

// deallocate returns the edits that deallocate ids, if any, and clear their mastery choices.
func (c *Checked) deallocate(ids []uint32) []Edit {
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	edits := []Edit{Deallocate(ids...)}
	for _, id := range ids {
		if _, ok := c.build.Masteries[id]; ok {
			edits = append(edits, ClearMastery(id))
		}
	}
	return edits
}

func (c *Checked) allocatedSet() map[uint32]bool {
	out := make(map[uint32]bool, len(c.nodes))
	for _, n := range c.nodes {
		out[n.ID] = true
	}
	return out
}

// starts returns the class start and, with an ascendancy, its start.
func (c *Checked) starts() map[uint32]bool {
	out := map[uint32]bool{c.class.StartNode: true}
	if c.ascStart != nil {
		out[c.ascStart.ID] = true
	}
	return out
}

// joined returns the nodes a path of allocated nodes joins to the class or ascendancy start,
// the starts included.
func (c *Checked) joined(allocated map[uint32]bool) map[uint32]bool {
	starts := c.starts()
	allowed := maps.Clone(allocated)
	maps.Copy(allowed, starts)
	return c.reach(slices.Sorted(maps.Keys(starts)), allowed)
}
