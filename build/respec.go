package build

import (
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// ChangeAscendancy returns the edits that change the build's ascendancy to id, empty for none:
// they set it, deallocate the old ascendancy's nodes and clear their mastery choices, and swap
// an allocated old ascendancy start for the new one. Nodes of other ascendancy kinds and of the
// main tree stay. It reports false when id is the build's ascendancy or not one of its class.
func (c *Checked) ChangeAscendancy(id string) ([]Edit, bool) {
	old := c.build.Ascendancy
	if id == old || (id != "" && !slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == id })) {
		return nil, false
	}
	if old == "" {
		return []Edit{SetAscendancy(id)}, true
	}
	return append([]Edit{SetAscendancy(id)}, c.respec(func(n *dataset.Node) bool { return n.Ascendancy == old }, c.ascendancyStart(id))...), true
}

// ChangeBloodline returns the edits that change the build's bloodline to id, empty for none:
// they set it, deallocate the old bloodline's nodes and clear their mastery choices, and swap
// an allocated old bloodline start for the new one. It reports false when id is the build's
// bloodline or not one of the tree's alternate ascendancies.
func (c *Checked) ChangeBloodline(id string) ([]Edit, bool) {
	old := c.build.Bloodline
	if id == old || (id != "" && !slices.ContainsFunc(c.tree.AlternateAscendancies, func(a dataset.Ascendancy) bool { return a.ID == id })) {
		return nil, false
	}
	if old == "" {
		return []Edit{SetBloodline(id)}, true
	}
	return append([]Edit{SetBloodline(id)}, c.respec(func(n *dataset.Node) bool { return n.Ascendancy == old }, c.ascendancyStart(id))...), true
}

// ChangeClass returns the edits that change the build's class to class, a Character.ID, as a
// respec does: they set it, clear the ascendancy, deallocate every node and clear every mastery
// choice. An allocated old class start becomes the new class's start. The bloodline stays, as
// any class may take it, but its nodes go. It reports false when
// class is the build's class or not in its tree and characters.
func (c *Checked) ChangeClass(class string) ([]Edit, bool) {
	i := slices.IndexFunc(c.tree.Classes, func(cl dataset.Class) bool { return cl.Character == class })
	if class == c.build.Class || i < 0 || !slices.ContainsFunc(c.chars, func(ch dataset.Character) bool { return ch.ID == class }) {
		return nil, false
	}
	edits := []Edit{SetClass(class)}
	if c.build.Ascendancy != "" {
		edits = append(edits, SetAscendancy(""))
	}
	return append(edits, c.respec(func(*dataset.Node) bool { return true }, c.node(c.tree.Classes[i].StartNode))...), true
}

// respec returns the edits that deallocate the allocated nodes drop selects and clear their
// mastery choices, and allocate start, if any, when drop takes an allocated start of its kind.
func (c *Checked) respec(drop func(*dataset.Node) bool, start *dataset.Node) []Edit {
	var gone []uint32
	hadStart := false
	for _, n := range c.nodes {
		if drop(n) {
			gone = append(gone, n.ID)
			hadStart = hadStart || (start != nil && n.Kind == start.Kind)
		}
	}
	var edits []Edit
	if len(gone) > 0 {
		edits = append(edits, Deallocate(gone...))
	}
	for _, id := range gone {
		if _, ok := c.build.Masteries[id]; ok {
			edits = append(edits, ClearMastery(id))
		}
	}
	if hadStart {
		edits = append(edits, Allocate(start.ID))
	}
	return edits
}

// ascendancyStart returns the start of ascendancy id, or nil.
func (c *Checked) ascendancyStart(id string) *dataset.Node {
	for i := range c.tree.Nodes {
		if n := &c.tree.Nodes[i]; n.Kind == dataset.AscendancyStart && n.Ascendancy == id {
			return n
		}
	}
	return nil
}
