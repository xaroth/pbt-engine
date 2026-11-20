package build

import (
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
)

// transformStatPrefix starts the stats of a jewel that transforms one attribute of the passives
// in its radius to another, as local_jewel_nearby_passives_str_to_int.
const transformStatPrefix = "local_jewel_nearby_passives_"

// transformRadius is the radius of every jewel with such a stat.
const transformRadius = "Large"

// Attribute is one of the character's three attributes.
type Attribute uint8

// The attributes.
const (
	Strength Attribute = iota + 1
	Dexterity
	Intelligence
)

// AttributeTransform is a passive in the radius of a jewel in an allocated socket whose From
// attribute the jewel makes give To instead, allocated or not. A passive a timeless jewel
// conquers or a tattoo or runegraft replaces is not transformed.
type AttributeTransform struct {
	Node *dataset.Node
	// Socket is the jewel's socket.
	Socket   uint32
	From, To Attribute
}

// AttributeTransforms returns the passives whose attributes the build's jewels transform, by
// socket, then the jewel's stat, then node id.
func (c *Checked) AttributeTransforms() []AttributeTransform { return slices.Clone(c.transforms) }

// findTransforms finds the passives in the radius of each jewel in an allocated socket with a
// transform stat, less those conquered or overridden. The radius index is read only when such a
// jewel is socketed.
func (c *Checked) findTransforms() error {
	jewels := c.transformJewels()
	if len(jewels) == 0 {
		return nil
	}
	var ix *radius.Index
	changed := c.conqueredOrOverridden()
	for _, j := range jewels {
		r, ok := c.radiusOf(j.Socket, transformRadius)
		if !ok {
			continue
		}
		if ix == nil {
			var err error
			if ix, err = c.radiusIndex(); err != nil {
				return err
			}
		}
		nodes, err := ix.Nodes(j.Socket, radius.Disc(r))
		if err != nil {
			// Only a socket on the tree has a radius.
			continue
		}
		for _, id := range nodes {
			if !changed[id] {
				j.Node = c.node(id)
				c.transforms = append(c.transforms, j)
			}
		}
	}
	return nil
}

// transformJewels returns the transforms of the jewels in allocated sockets, without a node, by
// socket then stat.
func (c *Checked) transformJewels() []AttributeTransform {
	var out []AttributeTransform
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		if c.allocated(socket) == nil {
			continue
		}
		for _, stat := range slices.Sorted(maps.Keys(modStats(&it))) {
			if from, to, ok := transformOf(stat); ok {
				out = append(out, AttributeTransform{Socket: socket, From: from, To: to})
			}
		}
	}
	return out
}

// TransformsAttributes reports whether jewel it transforms an attribute of the passives in its
// radius to another.
func TransformsAttributes(it *Item) bool {
	return slices.ContainsFunc(it.Mods, func(m ModLine) bool {
		return slices.ContainsFunc(m.Stats, func(sv StatValue) bool {
			_, _, ok := transformOf(sv.Stat)
			return ok
		})
	})
}

// transformOf returns the attributes a transform stat transforms from and to, and reports
// whether stat is one.
func transformOf(stat string) (from, to Attribute, ok bool) {
	rest, ok := strings.CutPrefix(stat, transformStatPrefix)
	if !ok {
		return 0, 0, false
	}
	f, t, ok := strings.Cut(rest, "_to_")
	if !ok {
		return 0, 0, false
	}
	from, to = abbreviated(f), abbreviated(t)
	return from, to, from != 0 && to != 0 && from != to
}

// abbreviated returns the attribute the stat ids abbreviate as s, or 0.
func abbreviated(s string) Attribute {
	switch s {
	case "str":
		return Strength
	case "dex":
		return Dexterity
	case "int":
		return Intelligence
	}
	return 0
}
