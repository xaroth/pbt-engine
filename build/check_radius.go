package build

import (
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
)

// Stats of a jewel that change what the small passives in its radius give.
const (
	// allocatedSmallGrantNothingStat makes allocated small passives in the radius grant nothing.
	allocatedSmallGrantNothingStat = "local_jewel_allocated_non_notable_passives_in_radius_grant_nothing"
	// unallocatedSmallGrantedStat makes the jewel grant the stats of unallocated small passives
	// in the radius.
	unallocatedSmallGrantedStat = "local_jewel_copy_stats_from_unallocated_non_notable_passives_in_radius"
)

// smallRadius is the radius of Unnatural Instinct, the jewel with these stats.
const smallRadius = "Small"

// RadiusPassive is a small passive in the radius of a jewel in an allocated socket that changes
// what it gives, allocated or not.
type RadiusPassive struct {
	Node *dataset.Node
	// Socket is the jewel's socket.
	Socket uint32
	// Nothing makes the passive grant nothing while it is allocated.
	Nothing bool
	// Granted makes the jewel grant the passive's stats, as timeless jewels change them, while
	// the passive is not allocated.
	Granted bool
}

// RadiusPassives returns the small passives whose effect the build's jewels change by
// allocation, by socket then node id.
func (c *Checked) RadiusPassives() []RadiusPassive { return slices.Clone(c.radiusPassives) }

// findRadiusPassives finds the small passives in the radius of each jewel in an allocated
// socket with allocatedSmallGrantNothingStat or unallocatedSmallGrantedStat. The radius index
// is read only when such a jewel is socketed.
func (c *Checked) findRadiusPassives() error {
	var ix *radius.Index
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		nothing, granted := hasStat(&it, allocatedSmallGrantNothingStat), hasStat(&it, unallocatedSmallGrantedStat)
		if !nothing && !granted || c.allocated(socket) == nil {
			continue
		}
		r, ok := c.radiusOf(socket, smallRadius)
		if !ok {
			continue
		}
		if ix == nil {
			var err error
			if ix, err = c.radiusIndex(); err != nil {
				return err
			}
		}
		c.addRadiusPassives(ix, RadiusPassive{Socket: socket, Nothing: nothing, Granted: granted}, radius.Disc(r))
	}
	return nil
}

// addRadiusPassives adds jewel, without a node, for each small passive in area around its socket.
func (c *Checked) addRadiusPassives(ix *radius.Index, jewel RadiusPassive, area radius.Area) {
	nodes, err := ix.Nodes(jewel.Socket, area)
	if err != nil {
		// Only a socket on the tree has a radius.
		return
	}
	for _, id := range nodes {
		if n := c.node(id); n.Kind == dataset.Normal {
			jewel.Node = n
			c.radiusPassives = append(c.radiusPassives, jewel)
		}
	}
}

// radiusIndex returns the radius index of the build's tree variant.
func (c *Checked) radiusIndex() (*radius.Index, error) {
	ix, err := radius.Of(c.store, c.build.Variant)
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return ix, nil
}

// radiusOf returns the jewel radius id of the build's tree for the jewel in socket. A tree
// without it, as a dataset older than the radii, gives the jewel's radius work nothing to go on:
// radiusOf warns once for the jewel and reports false, and the work is skipped.
func (c *Checked) radiusOf(socket uint32, id string) (dataset.JewelRadius, bool) {
	if i := slices.IndexFunc(c.tree.JewelRadii, func(r dataset.JewelRadius) bool { return r.ID == id }); i >= 0 {
		return c.tree.JewelRadii[i], true
	}
	if !c.radiusWarned[socket] {
		if c.radiusWarned == nil {
			c.radiusWarned = map[uint32]bool{}
		}
		c.radiusWarned[socket] = true
		it := c.build.Jewels[socket]
		name := it.Name
		if name == "" {
			name = it.Base
		}
		c.warnAboutf(Warning{Subject: jewelSubject(socket)}, RadiusMissing,
			"%s: the %s tree has no %s jewel radius; what it does in its radius is left out", name, c.build.Variant, id)
	}
	return dataset.JewelRadius{}, false
}
