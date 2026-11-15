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
	var area radius.Area
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		nothing, granted := hasStat(&it, allocatedSmallGrantNothingStat), hasStat(&it, unallocatedSmallGrantedStat)
		if !nothing && !granted || c.allocated(socket) == nil {
			continue
		}
		if ix == nil {
			var err error
			if ix, area, err = c.smallRadius(); err != nil {
				return err
			}
		}
		nodes, err := ix.Nodes(socket, area)
		if err != nil {
			// Only a socket on the tree has a radius.
			continue
		}
		for _, id := range nodes {
			if n := c.node(id); n.Kind == dataset.Normal {
				c.radiusPassives = append(c.radiusPassives, RadiusPassive{Node: n, Socket: socket, Nothing: nothing, Granted: granted})
			}
		}
	}
	return nil
}

func (c *Checked) smallRadius() (*radius.Index, radius.Area, error) {
	ix, err := radius.Of(c.store, c.build.Variant)
	if err != nil {
		return nil, radius.Area{}, fmt.Errorf("build: %w", err)
	}
	i := slices.IndexFunc(c.tree.JewelRadii, func(r dataset.JewelRadius) bool { return r.ID == smallRadius })
	if i < 0 {
		return nil, radius.Area{}, fmt.Errorf("build: jewel radius %s in the %s tree: %w", smallRadius, c.build.Variant, dataset.ErrNotInDataset)
	}
	return ix, radius.Disc(c.tree.JewelRadii[i]), nil
}
