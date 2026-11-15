package build

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// ItemNotableUnknown is an item line that allocates a node that is no notable of the tree.
const ItemNotableUnknown Code = "item-notable-unknown"

// itemPassiveStats are the stats of an item line "Allocates <notable>", such as an anointment,
// holding the notable's node id. An item can have several.
func itemPassiveStats() []string {
	return []string{
		"mod_granted_passive_hash", "mod_granted_passive_hash_2", "mod_granted_passive_hash_3",
		"mod_granted_passive_hash_4", "mod_granted_passive_hash_5",
	}
}

// ItemPassive is a notable an equipped item allocates. It gives its stats while the tree does
// not allocate it, without a point or a path, and is not part of the build's nodes.
type ItemPassive struct {
	Slot Slot
	// Name is the item's name.
	Name string
	Node *dataset.Node
}

// ItemPassives returns the notables the build's items allocate, by slot then line, each once.
func (c *Checked) ItemPassives() []ItemPassive { return slices.Clone(c.itemPassives) }

// findItemPassives finds the notables the equipped items allocate, and warns about lines that
// allocate a node that is no notable of the tree.
func (c *Checked) findItemPassives() {
	for _, s := range slices.Sorted(maps.Keys(c.build.Items)) {
		it := c.build.Items[s]
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				if !slices.Contains(itemPassiveStats(), sv.Stat) {
					continue
				}
				n := c.node(uint32(sv.Value)) //nolint:gosec // a node id, read from its name
				if n == nil || n.Kind != dataset.Notable || n.Ascendancy != "" {
					about := itemWarning(s)
					about.Stat = sv.Stat
					c.warnAboutf(about, ItemNotableUnknown, "line %q allocates node %d, no notable of the %s tree; left out", m.Text, sv.Value, c.build.Variant)
					continue
				}
				if !slices.ContainsFunc(c.itemPassives, func(p ItemPassive) bool { return p.Node.ID == n.ID }) {
					c.itemPassives = append(c.itemPassives, ItemPassive{Slot: s, Name: it.Name, Node: n})
				}
			}
		}
	}
}
