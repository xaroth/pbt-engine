package dataset

import (
	"errors"
	"slices"
)

// Features is what a dataset holds of the content that differs between leagues, read from the
// content itself. An empty field means the dataset has none of it.
type Features struct {
	// Bloodlines are the IDs of the tree's alternate ascendancies, in tree order.
	Bloodlines []string
	// OverrideKinds and OverrideTargets are the kinds of the tree's overrides and the nodes
	// they replace, sorted.
	OverrideKinds   []OverrideKind
	OverrideTargets []OverrideTarget
	// ItemClasses are the ItemClass.IDs that have at least one base, sorted.
	ItemClasses []string
	// Slots are the equipment slots, in game order; a stat-enabled slot is listed only when a
	// node of the tree grants its stat. None without an items group.
	Slots []Slot
	// ProxyKinds are the kinds of the tree's proxy nodes, sorted.
	ProxyKinds []ProxyKind
	Jewels     JewelFeatures
}

// Slot is an equipment slot.
type Slot struct {
	// ID is the game's Inventories.Id, such as "Ring1".
	ID string
	// Stat is the stat that opens the slot, such as "enable_ring_slot_3", or empty for a slot
	// every character has.
	Stat string
}

// equipmentSlots are the game's equipment inventories.
func equipmentSlots() []Slot {
	return []Slot{
		{ID: "Weapon1"}, {ID: "Offhand1"}, {ID: "Weapon2"}, {ID: "Offhand2"}, {ID: "Helm1"},
		{ID: "BodyArmour1"}, {ID: "Gloves1"}, {ID: "Boots1"}, {ID: "Amulet1"}, {ID: "Ring1"},
		{ID: "Ring2"}, {ID: "Ring3", Stat: "enable_ring_slot_3"}, {ID: "Belt1"}, {ID: "Flask1"},
	}
}

// ProxyKind classifies proxy nodes.
type ProxyKind uint8

// Proxy kinds.
const (
	// ClusterProxy places the nodes a cluster jewel adds; it sits in a proxy group of the
	// layout, or a cluster socket names it.
	ClusterProxy ProxyKind = iota
	// TreeProxy is any other proxy node, drawn in a group of the tree itself, such as 3.30's
	// Blightcore nodes.
	TreeProxy
)

func (k ProxyKind) String() string {
	switch k {
	case ClusterProxy:
		return "cluster"
	case TreeProxy:
		return "tree"
	}
	return unknown
}

// JewelFeatures is what a tree holds for jewels.
type JewelFeatures struct {
	// Sockets is the number of jewel sockets, ClusterSockets of those that take a cluster
	// jewel.
	Sockets, ClusterSockets int
	// Radii are the JewelRadius.IDs, in game table order.
	Radii []string
	// TimelessVersions is the number of alternate tree versions timeless jewels use.
	TimelessVersions int
}

// Features returns the features of the dataset with the tree of one variant, such as
// "standard". It reads the tree, its layout and the items group; a missing layout or items
// group leaves out what only it gives.
func (s *Store) Features(variant string) (Features, error) {
	t, err := s.Tree(variant)
	if err != nil {
		return Features{}, err
	}
	l, err := s.Layout(variant)
	if err != nil && !errors.Is(err, ErrNotInDataset) {
		return Features{}, err
	}
	items, err := s.Items()
	if err != nil && !errors.Is(err, ErrNotInDataset) {
		return Features{}, err
	}
	st, err := s.statsGroup()
	if err != nil {
		return Features{}, err
	}
	f := Features{Jewels: jewelFeatures(t), ProxyKinds: proxyKinds(t, l)}
	for _, a := range t.AlternateAscendancies {
		f.Bloodlines = append(f.Bloodlines, a.ID)
	}
	f.OverrideKinds, f.OverrideTargets = overrideFeatures(t)
	if items != nil {
		for _, b := range items.Bases {
			f.ItemClasses = append(f.ItemClasses, b.Class)
		}
		f.ItemClasses = sortedSet(f.ItemClasses)
		f.Slots = slots(t, st.index)
	}
	return f, nil
}

func overrideFeatures(t *Tree) ([]OverrideKind, []OverrideTarget) {
	kinds := make([]OverrideKind, 0, len(t.Overrides))
	targets := make([]OverrideTarget, 0, len(t.Overrides))
	for _, o := range t.Overrides {
		kinds = append(kinds, o.Kind)
		targets = append(targets, o.Target)
	}
	return sortedSet(kinds), sortedSet(targets)
}

func jewelFeatures(t *Tree) JewelFeatures {
	j := JewelFeatures{
		Sockets: len(t.JewelSlots), ClusterSockets: len(t.Clusters.Sockets), TimelessVersions: len(t.TimelessVersions),
	}
	for _, r := range t.JewelRadii {
		j.Radii = append(j.Radii, r.ID)
	}
	return j
}

// slots returns the equipment slots, leaving out a stat-enabled one whose stat no node of t
// grants.
func slots(t *Tree, index map[string]uint32) []Slot {
	var out []Slot
	for _, sl := range equipmentSlots() {
		if sl.Stat == "" {
			out = append(out, sl)
			continue
		}
		h, ok := index[sl.Stat]
		if ok && slices.ContainsFunc(t.Nodes, func(n Node) bool { return grants(n, h) }) {
			out = append(out, sl)
		}
	}
	return out
}

// grants reports whether node n or one of its mastery effects grants stat handle h.
func grants(n Node, h uint32) bool {
	has := func(sv StatValue) bool { return sv.Stat == h }
	if slices.ContainsFunc(n.Stats, has) {
		return true
	}
	return slices.ContainsFunc(n.MasteryEffects, func(e MasteryEffect) bool { return slices.ContainsFunc(e.Stats, has) })
}

// proxyKinds returns the kinds of t's proxy nodes; l, which may be nil, gives the proxy groups.
func proxyKinds(t *Tree, l *Layout) []ProxyKind {
	cluster := map[uint32]bool{}
	for _, s := range t.Clusters.Sockets {
		cluster[s.Proxy] = true
	}
	if l != nil {
		groups := map[uint32]bool{}
		for _, g := range l.Groups {
			groups[g.ID] = g.Proxy
		}
		for _, n := range l.Nodes {
			if groups[n.Group] {
				cluster[n.ID] = true
			}
		}
	}
	var kinds []ProxyKind
	for _, n := range t.Nodes {
		if n.Kind != Proxy {
			continue
		}
		if cluster[n.ID] {
			kinds = append(kinds, ClusterProxy)
		} else {
			kinds = append(kinds, TreeProxy)
		}
	}
	return sortedSet(kinds)
}

func sortedSet[T ~uint8 | ~string](s []T) []T {
	slices.Sort(s)
	return slices.Compact(s)
}
