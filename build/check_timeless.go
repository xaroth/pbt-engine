package build

import (
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
	"github.com/xaroth/pbt-engine/internal/poe1/timeless"
)

// TimelessPartial warns about a timeless jewel whose changes to passives other than keystones
// are not modelled.
const TimelessPartial Code = "timeless-jewel-partial"

// timelessRadius is the radius of every timeless jewel that conquers passives around it.
const timelessRadius = "Large"

// Conquest is what a timeless jewel in an allocated socket makes of a passive in its radius,
// allocated or not.
type Conquest struct {
	Node *dataset.Node
	// Socket is the jewel's socket.
	Socket uint32
	// Replace makes Stats take the place of the node's stats; otherwise they add to them.
	Replace bool
	Stats   []dataset.StatValue
}

// Conquests returns the passives the build's timeless jewels change, by socket then node id.
func (c *Checked) Conquests() []Conquest { return slices.Clone(c.conquests) }

// conquer finds what the timeless jewels in allocated sockets make of the passives in their
// radius: Elegant Hubris replaces or adds to each, every other jewel only replaces keystones,
// with a warning. A jewel whose version the tree does not have, as an abyss timeless jewel,
// changes nothing. The radius and versions are read only when such a jewel is socketed.
func (c *Checked) conquer() error {
	var data *timelessData
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		if !hasStat(&it, timeless.VersionStat) || c.allocated(socket) == nil {
			continue
		}
		j, ok := timeless.JewelOf(modStats(&it))
		if !ok {
			continue
		}
		if data == nil {
			var err error
			if data, err = c.timelessData(); err != nil {
				return err
			}
		}
		c.conquerFrom(socket, j, data)
	}
	return nil
}

// timelessData is what conquer reads from the dataset.
type timelessData struct {
	versions *timeless.Versions
	radius   *radius.Index
	area     radius.Area
}

func (c *Checked) timelessData() (*timelessData, error) {
	variant := c.build.Variant
	versions, err := timeless.VersionsOf(c.store, variant)
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	ix, err := radius.Of(c.store, variant)
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	i := slices.IndexFunc(c.tree.JewelRadii, func(r dataset.JewelRadius) bool { return r.ID == timelessRadius })
	if i < 0 {
		return nil, fmt.Errorf("build: jewel radius %s in the %s tree: %w", timelessRadius, variant, dataset.ErrNotInDataset)
	}
	return &timelessData{versions: versions, radius: ix, area: radius.Disc(c.tree.JewelRadii[i])}, nil
}

// conquerFrom adds what jewel j in socket makes of the passives in its radius.
func (c *Checked) conquerFrom(socket uint32, j timeless.ModJewel, data *timelessData) {
	name := data.versions.Name(j.Version)
	if name == "" {
		return
	}
	nodes, err := data.radius.Nodes(socket, data.area)
	if err != nil {
		// Only a socket on the tree has a radius; a timeless jewel fits no other.
		return
	}
	if !data.versions.ChangesPassives(j.Version) {
		c.warnf(TimelessPartial, jewelSubject(socket), "%s conquest of passives other than keystones not modelled; left out", name)
	}
	for _, id := range nodes {
		n := c.node(id)
		if ch, ok := data.versions.Conquer(j, timeless.Node{ID: id, Kind: c.timelessKind(n)}); ok {
			c.conquests = append(c.conquests, Conquest{Node: n, Socket: socket, Replace: ch.Replace, Stats: ch.Stats})
		}
	}
}

// timelessKind is n's kind as the alternate tree tables number it, from its own stats: a
// tattoo on it does not change the kind the jewel sees.
func (c *Checked) timelessKind(n *dataset.Node) timeless.Kind {
	switch n.Kind {
	case dataset.Normal:
		ids := make([]string, 0, len(n.Stats))
		for _, sv := range n.Stats {
			ids = append(ids, c.stats[sv.Stat].ID)
		}
		return timeless.SmallKind(ids)
	case dataset.Notable:
		return timeless.Notable
	case dataset.Keystone:
		return timeless.Keystone
	default:
		return timeless.JewelSocket
	}
}

func hasStat(it *Item, id string) bool {
	return slices.ContainsFunc(it.Mods, func(m ModLine) bool {
		return slices.ContainsFunc(m.Stats, func(sv StatValue) bool { return sv.Stat == id })
	})
}

// modStats returns the stats of it's mod lines by id.
func modStats(it *Item) map[string]int32 {
	out := map[string]int32{}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			out[sv.Stat] += sv.Value
		}
	}
	return out
}
