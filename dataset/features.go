package dataset

import (
	"errors"
	"slices"
	"strings"
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
	// SlotStats are the IDs of the stats a node of the tree grants that open or close an
	// equipment slot, which the game names enable_..._slot... and disable_..._slot..., such as
	// "enable_ring_slot_3" and "disable_amulet_slot", sorted.
	// None without an items group.
	SlotStats []string
	// ProxyKinds are the kinds of the tree's proxy nodes, sorted.
	ProxyKinds []ProxyKind
	Jewels     JewelFeatures
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
		f.SlotStats = slotStats(t, st.stats)
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

// slotStats returns the IDs of the stats a node of t grants that open or close a slot.
func slotStats(t *Tree, stats []Stat) []string {
	var out []string
	for _, n := range t.Nodes {
		for _, sv := range n.Stats {
			out = appendSlotStat(out, stats[sv.Stat].ID)
		}
		for _, e := range n.MasteryEffects {
			for _, sv := range e.Stats {
				out = appendSlotStat(out, stats[sv.Stat].ID)
			}
		}
	}
	return sortedSet(out)
}

func appendSlotStat(out []string, id string) []string {
	if (strings.HasPrefix(id, "enable_") || strings.HasPrefix(id, "disable_")) && strings.Contains(id, "_slot") {
		return append(out, id)
	}
	return out
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
