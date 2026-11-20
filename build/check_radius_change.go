package build

import (
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// Stats of a jewel that change what the passives in its radius give.
const (
	// radiusGrantStatPrefix starts the stats of The Light of Meaning, which make the passives in
	// its radius also give the stat its id ends with, as
	// local_unique_jewel_passives_in_radius_grant_evasion_rating_+%.
	radiusGrantStatPrefix = "local_unique_jewel_passives_in_radius_grant_"
	// LifeToManaDoubledStat is Healthy Mind's: increases and reductions to maximum life on the
	// passives in its radius apply to maximum mana at twice their value instead.
	LifeToManaDoubledStat = "local_unique_jewel_life_increases_applies_to_mana_doubled"
	// perAttributeStatPrefix starts the stats of Transcendent Flesh, Mind and Spirit that give
	// the jewel's value of an attribute per 1 of it on each allocated passive in its radius, as
	// local_unique_jewel_X_strength_per_1_strength_allocated_in_radius.
	perAttributeStatPrefix = "local_unique_jewel_X_"
	perAttributeStatSuffix = "_allocated_in_radius"
)

// The radii of the jewels with these stats.
const (
	largeRadius  = "Large"
	mediumRadius = "Medium"
)

// RadiusChange is a passive in the radius of a jewel in an allocated socket whose stats a stat
// of the jewel changes, allocated or not. The Light of Meaning and Healthy Mind do not change a
// passive a timeless jewel conquers, since conquered passives cannot be modified by other radius
// jewels; Transcendent jewels only count what passives give, conquered or not. A passive a tattoo
// or runegraft replaces is changed as it is replaced.
type RadiusChange struct {
	Node *dataset.Node
	// Socket is the jewel's socket.
	Socket uint32
	// Stat is the jewel's stat and Value its value on the jewel.
	Stat  string
	Value int32
}

// RadiusChanges returns the passives the build's jewels change by a stat that says so, by
// socket, then the jewel's stat, then node id.
func (c *Checked) RadiusChanges() []RadiusChange { return slices.Clone(c.radiusChanges) }

// findRadiusChanges finds the passives in the radius of each jewel in an allocated socket with
// a stat that changes them. A radius's index is read only when such a jewel is socketed.
func (c *Checked) findRadiusChanges() error {
	conquered := map[uint32]bool{}
	for _, q := range c.conquests {
		conquered[q.Node.ID] = true
	}
	for _, socket := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[socket]
		if c.allocated(socket) == nil {
			continue
		}
		stats := modStats(&it)
		for _, stat := range slices.Sorted(maps.Keys(stats)) {
			if r, ok := radiusChangeOf(stat); ok {
				if err := c.addRadiusChanges(RadiusChange{Socket: socket, Stat: stat, Value: stats[stat]}, r, conquered); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// addRadiusChanges adds jewel, without a node, for each passive r changes in its radius, less
// the conquered ones r does not reach.
func (c *Checked) addRadiusChanges(jewel RadiusChange, r radiusChangeStat, conquered map[uint32]bool) error {
	ix, area, err := c.jewelRadius(r.radius)
	if err != nil {
		return err
	}
	nodes, err := ix.Nodes(jewel.Socket, area)
	if err != nil {
		return nil //nolint:nilerr // only a socket on the tree has a radius
	}
	for _, id := range nodes {
		if n := c.node(id); r.changes(n.Kind) && (r.conquered || !conquered[id]) {
			jewel.Node = n
			c.radiusChanges = append(c.radiusChanges, jewel)
		}
	}
	return nil
}

// conqueredOrOverridden returns the ids of the passives a timeless jewel conquers or a tattoo or
// runegraft replaces; radius jewels leave them as they are.
func (c *Checked) conqueredOrOverridden() map[uint32]bool {
	out := map[uint32]bool{}
	for _, o := range c.overrides {
		out[o.Node.ID] = true
	}
	for _, q := range c.conquests {
		out[q.Node.ID] = true
	}
	return out
}

// radiusChangeStat is the radius of a stat that changes the passives in it, and the passives it
// changes.
type radiusChangeStat struct {
	radius  string
	changes func(dataset.NodeKind) bool
	// conquered is set when the stat reaches passives a timeless jewel conquers.
	conquered bool
}

// radiusChangeOf returns what stat changes, and reports whether it changes passives in a radius.
func radiusChangeOf(stat string) (radiusChangeStat, bool) {
	switch {
	case strings.HasPrefix(stat, radiusGrantStatPrefix):
		return radiusChangeStat{radius: largeRadius, changes: func(k dataset.NodeKind) bool { return k == dataset.Normal || k == dataset.Notable }}, true
	case stat == LifeToManaDoubledStat:
		return radiusChangeStat{radius: largeRadius, changes: notMastery}, true
	case PerAttributeOf(stat) != 0:
		return radiusChangeStat{radius: mediumRadius, changes: notMastery, conquered: true}, true
	}
	return radiusChangeStat{}, false
}

// notMastery reports whether k is not a mastery, whose effects radius jewels do not change.
func notMastery(k dataset.NodeKind) bool { return k != dataset.Mastery }

// RadiusGrantOf returns the stat a The Light of Meaning stat makes the passives in its radius
// also give, and reports whether stat is one.
func RadiusGrantOf(stat string) (string, bool) {
	granted, ok := strings.CutPrefix(stat, radiusGrantStatPrefix)
	return granted, ok && granted != ""
}

// PerAttributeOf returns the attribute a Transcendent jewel stat counts per 1 of on each
// allocated passive in its radius, or 0 when stat is no such stat.
func PerAttributeOf(stat string) Attribute {
	rest, ok := strings.CutPrefix(stat, perAttributeStatPrefix)
	if !ok {
		return 0
	}
	rest, ok = strings.CutSuffix(rest, perAttributeStatSuffix)
	if !ok {
		return 0
	}
	a, per, ok := strings.Cut(rest, "_per_1_")
	if !ok || a != per {
		return 0
	}
	switch a {
	case "strength":
		return Strength
	case "dexterity":
		return Dexterity
	case "intelligence":
		return Intelligence
	}
	return 0
}

// ChangesRadiusPassives reports whether jewel it has a stat that changes the passives in its
// radius (RadiusChanges).
func ChangesRadiusPassives(it *Item) bool {
	return slices.ContainsFunc(it.Mods, func(m ModLine) bool {
		return slices.ContainsFunc(m.Stats, func(sv StatValue) bool {
			_, ok := radiusChangeOf(sv.Stat)
			return ok
		})
	})
}
