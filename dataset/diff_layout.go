package dataset

import (
	"reflect"
	"slices"
)

// LayoutDiff lists the changes in one tree variant's layout present in both datasets. Nodes
// added to or removed from the tree show in its TreeDiff.
type LayoutDiff struct {
	Variant                      string
	BoundsChanged, OrbitsChanged bool
	GroupsAdded, GroupsRemoved   []uint32
	GroupsChanged                []uint32
	// NodesMoved are nodes of both layouts drawn at another position, or drawn in only one.
	NodesMoved []uint32
	// ArtChanged are nodes of both layouts whose icon or mastery art changed.
	ArtChanged      []uint32
	ClassArtChanged bool
}

func compareLayoutGroups(d *Diff, a, b *Store) error {
	x, err := layoutsOf(a)
	if err != nil {
		return err
	}
	y, err := layoutsOf(b)
	if err != nil {
		return err
	}
	ax, by := byKey(x, func(l *Layout) string { return l.Variant }), byKey(y, func(l *Layout) string { return l.Variant })
	d.LayoutVariantsAdded, d.LayoutVariantsRemoved = setDiff(keys(ax), keys(by))
	for _, v := range sortedUnion(ax, by) {
		p, inA := ax[v]
		q, inB := by[v]
		if inA && inB {
			d.Layouts = append(d.Layouts, compareLayouts(p, q))
		}
	}
	return nil
}

func layoutsOf(s *Store) ([]*Layout, error) {
	var out []*Layout
	for _, v := range s.Manifest().Layouts() {
		l, err := s.Layout(v)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func compareLayouts(p, q *Layout) LayoutDiff {
	ld := LayoutDiff{
		Variant:         p.Variant,
		BoundsChanged:   p.MinX != q.MinX || p.MinY != q.MinY || p.MaxX != q.MaxX || p.MaxY != q.MaxY,
		OrbitsChanged:   !reflect.DeepEqual(p.Orbits, q.Orbits),
		ClassArtChanged: !slices.Equal(p.ClassArt, q.ClassArt),
	}
	ld.GroupsAdded, ld.GroupsRemoved, ld.GroupsChanged = changes(p.Groups, q.Groups,
		func(g LayoutGroup) uint32 { return g.ID }, func(g, h LayoutGroup) bool { return reflect.DeepEqual(g, h) })
	ld.NodesMoved, ld.ArtChanged = compareLayoutNodes(p, q)
	return ld
}

func compareLayoutNodes(p, q *Layout) (moved, art []uint32) {
	pn, qn := byKey(p.Nodes, layoutNodeID), byKey(q.Nodes, layoutNodeID)
	for _, id := range sortedUnion(pn, qn) {
		m, inP := pn[id]
		n, inQ := qn[id]
		if !inP || !inQ {
			continue
		}
		x0, y0, drawnP := p.Position(id)
		x1, y1, drawnQ := q.Position(id)
		if drawnP != drawnQ || x0 != x1 || y0 != y1 {
			moved = append(moved, id)
		}
		m.Group, m.Orbit, m.OrbitIndex = n.Group, n.Orbit, n.OrbitIndex
		if m != n {
			art = append(art, id)
		}
	}
	return moved, art
}

func layoutNodeID(n LayoutNode) uint32 { return n.ID }
