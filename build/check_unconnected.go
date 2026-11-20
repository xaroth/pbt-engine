package build

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/internal/poe1/radius"
)

// Stats of jewels that let passives be allocated without a connection to the tree.
const (
	// nearbyUnconnectedStat lets the passives in the jewel's radius be allocated so: Intuitive
	// Leap's, or with ringStat Thread of Hope's ring.
	nearbyUnconnectedStat = "local_unique_jewel_nearby_disconnected_passives_can_be_allocated"
	// ringStat names Thread of Hope's ring (ringRadius).
	ringStat = "local_jewel_variable_ring_radius_value"
	// keystoneUnconnectedStat lets the passives in the radius of the keystone whose node id it
	// holds be allocated so: Impossible Escape.
	keystoneUnconnectedStat = "local_unique_jewel_disconnected_passives_can_be_allocated_around_keystone_hash"
)

// leapRadius is the radius of Intuitive Leap, and of Impossible Escape around its keystone.
const leapRadius = "Small"

// findUnconnected finds the passives each jewel lets be allocated without a connection, by
// socket, whether the socket is allocated or not: joined counts them while a path joins it. The
// radius index is read only when such a jewel is socketed.
func (c *Checked) findUnconnected() error {
	var ix *radius.Index
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		stats := modStats(&it)
		if _, ok := stats[nearbyUnconnectedStat]; !ok {
			if _, ok := stats[keystoneUnconnectedStat]; !ok {
				continue
			}
		}
		area, ok := c.unconnectedArea(socket, stats)
		if !ok {
			continue
		}
		if ix == nil {
			var err error
			if ix, err = c.radiusIndex(); err != nil {
				return err
			}
		}
		if nodes := c.unconnectedNodes(ix, socket, area, stats); len(nodes) > 0 {
			if c.unconnected == nil {
				c.unconnected = map[uint32][]uint32{}
			}
			c.unconnected[socket] = nodes
		}
	}
	return nil
}

// unconnectedNodes returns the passives a jewel with stats in socket lets be allocated without
// a connection in area: around the socket when it has nearbyUnconnectedStat, else around the
// keystone. A socket off the tree or a keystone not on it reaches none.
func (c *Checked) unconnectedNodes(ix *radius.Index, socket uint32, area radius.Area, stats map[string]int32) []uint32 {
	var nodes []uint32
	var err error
	if _, nearby := stats[nearbyUnconnectedStat]; nearby {
		nodes, err = ix.Nodes(socket, area)
	} else {
		nodes, err = ix.NodesAround(uint32(stats[keystoneUnconnectedStat]), area) //nolint:gosec // a node id, read from its name
	}
	if err != nil {
		return nil
	}
	return nodes
}

// unconnectedArea returns the area of the jewel with stats in socket around its socket or
// keystone: Thread of Hope's ring, else leapRadius. It reports false for a ring value with no
// radius, and for a tree without the radius.
func (c *Checked) unconnectedArea(socket uint32, stats map[string]int32) (radius.Area, bool) {
	id, ring := leapRadius, false
	if v, ok := stats[ringStat]; ok {
		if id, ok = ringRadius(v); !ok {
			return radius.Area{}, false
		}
		ring = true
	}
	r, ok := c.radiusOf(socket, id)
	if !ok {
		return radius.Area{}, false
	}
	if ring {
		return radius.Ring(r), true
	}
	return radius.Disc(r), true
}

// ringRadius returns the jewel radius whose ring Thread of Hope reaches with ringStat v.
func ringRadius(v int32) (string, bool) {
	radii := [...]string{"Small", "Medium", "Large", "VeryLarge", "Massive"}
	if v < 1 || int(v) > len(radii) {
		return "", false
	}
	return radii[v-1], true
}

// unconnectedRoots returns the nodes of allocated, not yet reached, that a jewel in a reached
// socket lets be allocated without a connection, sorted.
func (c *Checked) unconnectedRoots(allocated, reached map[uint32]bool) []uint32 {
	var out []uint32
	for socket, nodes := range c.unconnected {
		if !reached[socket] {
			continue
		}
		for _, n := range nodes {
			if allocated[n] && !reached[n] {
				out = append(out, n)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
