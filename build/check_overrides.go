package build

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// OverrideChoice is an allocated node and the tattoo or runegraft put on it.
type OverrideChoice struct {
	Node     *dataset.Node
	Override *dataset.NodeOverride
}

// Overrides returns the overrides kept, by node id.
func (c *Checked) Overrides() []OverrideChoice { return slices.Clone(c.overrides) }

// checkOverrides keeps the overrides on allocated nodes they may replace.
func (c *Checked) checkOverrides() {
	for _, id := range slices.Sorted(maps.Keys(c.build.Overrides)) {
		n := c.allocated(id)
		if w, bad := c.overrideProblem(id, c.build.Overrides[id], n != nil); bad {
			c.warnings = append(c.warnings, w)
			delete(c.build.Overrides, id)
			continue
		}
		o, _ := c.tree.Override(c.build.Overrides[id])
		c.overrides = append(c.overrides, OverrideChoice{Node: n, Override: o})
	}
	if len(c.build.Overrides) == 0 {
		c.build.Overrides = nil
	}
}

// overridden reports whether an override was kept on node id.
func (c *Checked) overridden(id uint32) bool {
	_, ok := slices.BinarySearchFunc(c.overrides, id, func(o OverrideChoice, id uint32) int { return cmp.Compare(o.Node.ID, id) })
	return ok
}

// overrideProblem returns the warning for putting the override id on node when it does not fit.
func (c *Checked) overrideProblem(node uint32, id string, allocated bool) (Warning, bool) {
	subject := fmt.Sprintf("node %d", node)
	o, ok := c.tree.Override(id)
	if !ok {
		msg := fmt.Sprintf("override %q is not in the %s tree of %s; dropped", id, c.build.Variant, c.store.Manifest().Version)
		return Warning{Code: OverrideUnknown, Subject: subject, Message: msg}, true
	}
	n := c.node(node)
	if n == nil || !allocated {
		return Warning{Code: OverrideNotAllocated, Subject: subject, Message: fmt.Sprintf("override %s on a node that is not allocated; dropped", id)}, true
	}
	if why := c.cannotReplace(o, n); why != "" {
		return Warning{Code: OverrideNotPlaceable, Subject: subject, Message: fmt.Sprintf("override %s %s; dropped", id, why)}, true
	}
	return Warning{}, false
}

// cannotReplace says why o may not replace n, or returns "" when it may.
func (c *Checked) cannotReplace(o *dataset.NodeOverride, n *dataset.Node) string {
	if !fitsTarget(c.stats, o.Target, n) {
		return "replaces only a " + o.Target.String() + " node"
	}
	if _, excluded := slices.BinarySearch(o.ExcludedNodes, n.ID); excluded {
		return "may not replace this node"
	}
	links := c.links(n)
	if links < int(o.MinNeighbours) {
		return fmt.Sprintf("needs at least %d neighbours, the node has %d", o.MinNeighbours, links)
	}
	if o.MaxNeighbours > 0 && links > int(o.MaxNeighbours) {
		return fmt.Sprintf("needs at most %d neighbours, the node has %d", o.MaxNeighbours, links)
	}
	return ""
}

// links counts n's neighbours other than masteries, whose edges only say which notables unlock
// them.
func (c *Checked) links(n *dataset.Node) int {
	count := 0
	for _, id := range n.Neighbours {
		if nb := c.node(id); nb != nil && nb.Kind != dataset.Mastery {
			count++
		}
	}
	return count
}

// Attribute stats as small attribute nodes and attribute notables grant them.
const (
	strengthStat     = "base_strength"
	dexterityStat    = "base_dexterity"
	intelligenceStat = "base_intelligence"
	// notableAttribute is the attribute an attribute notable grants.
	notableAttribute = 30
)

// attributeOf returns the attribute stat of a target of one attribute, and whether it is a
// notable's; "" for other targets.
func attributeOf(t dataset.OverrideTarget) (stat string, notable bool) {
	switch t {
	case dataset.SmallStrength, dataset.StrengthNotable:
		stat = strengthStat
	case dataset.SmallDexterity, dataset.DexterityNotable:
		stat = dexterityStat
	case dataset.SmallIntelligence, dataset.IntelligenceNotable:
		stat = intelligenceStat
	case dataset.SmallAttribute, dataset.KeystoneTarget, dataset.MasteryTarget, dataset.AscendancyNotable:
	}
	notable = t == dataset.StrengthNotable || t == dataset.DexterityNotable || t == dataset.IntelligenceNotable
	return stat, notable
}

func fitsTarget(stats []dataset.Stat, t dataset.OverrideTarget, n *dataset.Node) bool {
	attr, notable := attributeOf(t)
	switch {
	case attr == "":
		return fitsKind(stats, t, n)
	case notable:
		return n.Ascendancy == "" && n.Kind == dataset.Notable && grants(stats, n, attr, notableAttribute)
	}
	return smallAttribute(stats, n) == attr
}

// fitsKind is fitsTarget of a target not of one attribute.
func fitsKind(stats []dataset.Stat, t dataset.OverrideTarget, n *dataset.Node) bool {
	main := n.Ascendancy == ""
	switch t {
	case dataset.SmallAttribute:
		return smallAttribute(stats, n) != ""
	case dataset.KeystoneTarget:
		return main && n.Kind == dataset.Keystone
	case dataset.MasteryTarget:
		return n.Kind == dataset.Mastery
	case dataset.AscendancyNotable:
		return !main && n.Kind == dataset.Notable
	case dataset.SmallStrength, dataset.SmallDexterity, dataset.SmallIntelligence,
		dataset.StrengthNotable, dataset.DexterityNotable, dataset.IntelligenceNotable:
	}
	return false
}

// smallAttribute returns the attribute stat of a small attribute node, one of the main tree
// whose only stat is an attribute, or "".
func smallAttribute(stats []dataset.Stat, n *dataset.Node) string {
	if n.Kind != dataset.Normal || n.Ascendancy != "" || n.MultipleChoiceOption || len(n.Stats) != 1 {
		return ""
	}
	switch id := stats[n.Stats[0].Stat].ID; id {
	case strengthStat, dexterityStat, intelligenceStat:
		return id
	}
	return ""
}

func grants(stats []dataset.Stat, n *dataset.Node, stat string, value int32) bool {
	return slices.ContainsFunc(n.Stats, func(sv dataset.StatValue) bool { return stats[sv.Stat].ID == stat && sv.Value == value })
}
