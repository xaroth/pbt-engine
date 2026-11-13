package dataset

import (
	"cmp"
	"fmt"
	"slices"
)

// validateJewels checks the jewel data of t against a stats group of n stats and the tree's
// node ids.
func validateJewels(n int, ids map[uint32]bool, t Tree) error {
	if err := validateJewelRadii(t); err != nil {
		return err
	}
	return validateClusters(n, ids, t)
}

func validateJewelRadii(t Tree) error {
	for _, r := range t.JewelRadii {
		if r.ID == "" || r.Radius <= 0 || r.RingInner < 0 || r.RingInner > r.RingOuter {
			return fmt.Errorf("tree %s jewel radius %q: empty id, radius not positive or ring reversed: %w", t.Variant, r.ID, ErrInvalid)
		}
	}
	return nil
}

// validateClusters checks the cluster data of t against a stats group of n stats and the tree's
// node ids. validateClusterBases checks the jewel bases against the items group.
func validateClusters(n int, ids map[uint32]bool, t Tree) error {
	if err := validateClusterSockets(ids, t); err != nil {
		return err
	}
	if err := validateClusterJewels(t); err != nil {
		return err
	}
	return validateClusterRefs(n, ids, t)
}

// validateClusterRefs checks the cluster skills and notables of t against the tree's node ids
// and a stats group of n stats.
func validateClusterRefs(n int, ids map[uint32]bool, t Tree) error {
	what := "tree " + t.Variant
	for i, s := range t.Clusters.Skills {
		sw := fmt.Sprintf("%s cluster skill %d", what, i)
		switch {
		case s.Size > LargeCluster:
			return fmt.Errorf("%s: unknown size %d: %w", sw, s.Size, ErrInvalid)
		case s.Tag == "":
			return fmt.Errorf("%s: no tag: %w", sw, ErrInvalid)
		case !ids[s.Passive] || s.Mastery != 0 && !ids[s.Mastery]:
			return fmt.Errorf("%s: passive %d or mastery %d not in tree: %w", sw, s.Passive, s.Mastery, ErrInvalid)
		}
	}
	for i, nt := range t.Clusters.Notables {
		if !ids[nt.Passive] {
			return fmt.Errorf("%s cluster notable %d: passive %d not in tree: %w", what, i, nt.Passive, ErrInvalid)
		}
		if int(nt.Stat) >= n {
			return fmt.Errorf("%s cluster notable %d: stat %d out of range (%d stats): %w", what, i, nt.Stat, n, ErrInvalid)
		}
	}
	return nil
}

func validateClusterJewels(t Tree) error {
	sizes := map[ClusterSize]bool{}
	for _, j := range t.Clusters.Jewels {
		what := fmt.Sprintf("tree %s cluster jewel %s", t.Variant, j.Base)
		if j.Size > LargeCluster || sizes[j.Size] {
			return fmt.Errorf("%s: size %s unknown or repeated: %w", what, j.Size, ErrInvalid)
		}
		sizes[j.Size] = true
		if j.Base == "" || j.MinNodes > j.MaxNodes {
			return fmt.Errorf("%s: no base or node counts reversed: %w", what, ErrInvalid)
		}
		for _, x := range slices.Concat(j.SmallIndices, j.NotableIndices, j.SocketIndices) {
			if x < 0 || x >= j.TotalIndices {
				return fmt.Errorf("%s: index %d outside %d indices: %w", what, x, j.TotalIndices, ErrInvalid)
			}
		}
	}
	return nil
}

func validateClusterSockets(ids map[uint32]bool, t Tree) error {
	socks := t.Clusters.Sockets
	for i := 1; i < len(socks); i++ {
		if socks[i-1].Node >= socks[i].Node {
			return fmt.Errorf("tree %s: cluster sockets not sorted or repeated at %d: %w", t.Variant, socks[i].Node, ErrInvalid)
		}
	}
	for _, s := range socks {
		sw := fmt.Sprintf("tree %s cluster socket %d", t.Variant, s.Node)
		switch {
		case !slices.Contains(t.JewelSlots, s.Node):
			return fmt.Errorf("%s: not a jewel slot: %w", sw, ErrInvalid)
		case s.Size > LargeCluster:
			return fmt.Errorf("%s: unknown size %d: %w", sw, s.Size, ErrInvalid)
		case !isProxy(ids, t, s.Proxy):
			return fmt.Errorf("%s: proxy %d not a proxy node of the tree: %w", sw, s.Proxy, ErrInvalid)
		}
		if _, ok := t.ClusterSocket(s.Parent); s.Parent != 0 && (!ok || s.Parent == s.Node) {
			return fmt.Errorf("%s: parent %d not another cluster socket: %w", sw, s.Parent, ErrInvalid)
		}
	}
	return nil
}

// isProxy reports whether id is a proxy node of t, whose node ids are ids.
func isProxy(ids map[uint32]bool, t Tree, id uint32) bool {
	if !ids[id] {
		return false
	}
	i, _ := slices.BinarySearchFunc(t.Nodes, id, func(n Node, id uint32) int { return cmp.Compare(n.ID, id) })
	return t.Nodes[i].Kind == Proxy
}

// validateClusterBases checks that every cluster jewel base is a base of the items group. The
// Store does not repeat it, so reading a tree does not read the items group.
func (d *Dataset) validateClusterBases() error {
	var bases map[string]bool
	for _, t := range d.Trees {
		for _, j := range t.Clusters.Jewels {
			if d.Items == nil {
				return fmt.Errorf("tree %s cluster jewel %s: cluster jewel without items: %w", t.Variant, j.Base, ErrInvalid)
			}
			if bases == nil {
				bases = make(map[string]bool, len(d.Items.Bases))
				for _, b := range d.Items.Bases {
					bases[b.ID] = true
				}
			}
			if !bases[j.Base] {
				return fmt.Errorf("tree %s cluster jewel %s: base not in items: %w", t.Variant, j.Base, ErrInvalid)
			}
		}
	}
	return nil
}
