package build

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of Check for the notables jewels allocate together.
const (
	// JewelNotableUnmatched is a jewel line that allocates a notable with a matching line on
	// another jewel, when no jewel in an allocated socket has it.
	JewelNotableUnmatched Code = "jewel-notable-unmatched"
	// JewelNotableUnknown is such a line naming a node that is no ascendancy passive of the
	// build's class.
	JewelNotableUnknown Code = "jewel-notable-unknown"
)

// The stats of Forbidden Flame's and Forbidden Flesh's lines "Allocates <notable> if you have
// the matching modifier on Forbidden Flesh" (or Flame), holding the notable's node id.
const (
	flameNotableStat = "unique_jewel_grants_notable_hash_part_1"
	fleshNotableStat = "unique_jewel_grants_notable_hash_part_2"
)

// AscendancyPassiveStats returns the stats of item lines that name an ascendancy passive by its
// node id.
func AscendancyPassiveStats() []string { return []string{flameNotableStat, fleshNotableStat} }

// JewelPassive is an ascendancy passive of the build's class, a notable or, of the Ascendant,
// a passive named after an ascendancy, that a Forbidden Flame and a Forbidden Flesh allocate
// together, both in allocated sockets and naming the same passive. It gives its stats while
// the tree does not allocate it, without a point or a path, even when it is of another
// ascendancy, and is not part of the build's nodes.
type JewelPassive struct {
	// Socket is the Forbidden Flame's socket, With the Forbidden Flesh's.
	Socket, With uint32
	// Name is the Forbidden Flame's name.
	Name string
	Node *dataset.Node
}

// JewelPassives returns the notables jewels allocate together, by the Forbidden Flame's socket.
func (c *Checked) JewelPassives() []JewelPassive { return slices.Clone(c.jewelPassives) }

// jewelNotable is a jewel line that allocates a notable with a matching line on another jewel.
type jewelNotable struct {
	socket uint32
	stat   string
	text   string
	node   uint32
}

// findJewelPassives pairs the lines of Forbidden Flame and Forbidden Flesh in allocated sockets
// that name the same notable, and warns about each line left without a match.
func (c *Checked) findJewelPassives() {
	flames, fleshes := c.jewelNotables()
	matched := func(l jewelNotable, others []jewelNotable) int {
		return slices.IndexFunc(others, func(o jewelNotable) bool { return o.node == l.node })
	}
	for _, f := range flames {
		if i := matched(f, fleshes); i >= 0 {
			c.jewelPassives = append(c.jewelPassives, JewelPassive{
				Socket: f.socket, With: fleshes[i].socket, Name: c.build.Jewels[f.socket].Name, Node: c.node(f.node),
			})
		}
	}
	for _, ls := range [][2][]jewelNotable{{flames, fleshes}, {fleshes, flames}} {
		for _, l := range ls[0] {
			if matched(l, ls[1]) < 0 {
				c.warnAboutf(Warning{Subject: jewelSubject(l.socket), Stat: l.stat}, JewelNotableUnmatched,
					"line %q has no matching line on a jewel in an allocated socket; nothing allocated", l.text)
			}
		}
	}
}

// jewelNotables returns the Forbidden Flame and Forbidden Flesh lines of the jewels in
// allocated sockets, by socket, and warns about those naming no ascendancy passive of the
// class. A jewel in a socket not allocated already has its own warning.
func (c *Checked) jewelNotables() (flames, fleshes []jewelNotable) {
	for _, id := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		if c.allocated(id) == nil {
			continue
		}
		for _, m := range c.build.Jewels[id].Mods {
			for _, sv := range m.Stats {
				l := jewelNotable{socket: id, stat: sv.Stat, text: m.Text, node: uint32(sv.Value)} //nolint:gosec // a node id, read from its name
				switch {
				case sv.Stat != flameNotableStat && sv.Stat != fleshNotableStat:
				case !c.classAscendancyPassive(l.node):
					c.warnAboutf(Warning{Subject: jewelSubject(id), Stat: sv.Stat}, JewelNotableUnknown,
						"line %q allocates node %d, no ascendancy passive of %s; left out", m.Text, l.node, c.build.Class)
				case sv.Stat == flameNotableStat:
					flames = append(flames, l)
				default:
					fleshes = append(fleshes, l)
				}
			}
		}
	}
	return flames, fleshes
}

// classAscendancyPassive reports whether node id is a passive of one of the class's
// ascendancies other than its start.
func (c *Checked) classAscendancyPassive(id uint32) bool {
	n := c.node(id)
	return n != nil && n.Kind != dataset.AscendancyStart &&
		slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == n.Ascendancy })
}
