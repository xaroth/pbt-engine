package dataset

import (
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func encodeLayout(l Layout) []byte {
	b := flatbuffers.NewBuilder(0)
	orbits := encodeOrbits(b, l.Orbits)
	groupOffs := make([]flatbuffers.UOffsetT, len(l.Groups))
	for i, g := range l.Groups {
		groupOffs[i] = encodeLayoutGroup(b, g)
	}
	groups := tableVector(b, fb.LayoutStartGroupsVector, groupOffs)
	nodeOffs := make([]flatbuffers.UOffsetT, len(l.Nodes))
	for i, n := range l.Nodes {
		nodeOffs[i] = encodeLayoutNode(b, n)
	}
	nodes := tableVector(b, fb.LayoutStartNodesVector, nodeOffs)
	art := encodeClassArt(b, l.ClassArt)
	variant := b.CreateString(l.Variant)
	fb.LayoutStart(b)
	fb.LayoutAddVariant(b, variant)
	fb.LayoutAddMinX(b, l.MinX)
	fb.LayoutAddMinY(b, l.MinY)
	fb.LayoutAddMaxX(b, l.MaxX)
	fb.LayoutAddMaxY(b, l.MaxY)
	fb.LayoutAddOrbits(b, orbits)
	fb.LayoutAddGroups(b, groups)
	fb.LayoutAddNodes(b, nodes)
	fb.LayoutAddClassArt(b, art)
	fb.FinishLayoutBuffer(b, fb.LayoutEnd(b))
	return b.FinishedBytes()
}

func encodeOrbits(b *flatbuffers.Builder, orbits []Orbit) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(orbits))
	for i, o := range orbits {
		fb.OrbitStart(b)
		fb.OrbitAddRadius(b, o.Radius)
		fb.OrbitAddSlots(b, o.Slots)
		offs[i] = fb.OrbitEnd(b)
	}
	return tableVector(b, fb.LayoutStartOrbitsVector, offs)
}

func encodeClassArt(b *flatbuffers.Builder, art []ClassArt) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(art))
	for i, a := range art {
		image := b.CreateString(a.Image)
		fb.ClassArtStart(b)
		fb.ClassArtAddClassId(b, a.Class)
		fb.ClassArtAddImage(b, image)
		fb.ClassArtAddX(b, a.X)
		fb.ClassArtAddY(b, a.Y)
		offs[i] = fb.ClassArtEnd(b)
	}
	return tableVector(b, fb.LayoutStartClassArtVector, offs)
}

func encodeLayoutGroup(b *flatbuffers.Builder, g LayoutGroup) flatbuffers.UOffsetT {
	var bg flatbuffers.UOffsetT
	if g.Background != nil {
		image := b.CreateSharedString(g.Background.Image)
		fb.GroupBackgroundStart(b)
		fb.GroupBackgroundAddImage(b, image)
		fb.GroupBackgroundAddHalfImage(b, g.Background.HalfImage)
		bg = fb.GroupBackgroundEnd(b)
	}
	orbits := b.CreateByteVector(g.Orbits)
	fb.LayoutGroupStart(b)
	fb.LayoutGroupAddId(b, g.ID)
	fb.LayoutGroupAddX(b, g.X)
	fb.LayoutGroupAddY(b, g.Y)
	fb.LayoutGroupAddOrbits(b, orbits)
	if g.Background != nil {
		fb.LayoutGroupAddBackground(b, bg)
	}
	fb.LayoutGroupAddProxy(b, g.Proxy)
	return fb.LayoutGroupEnd(b)
}

func encodeLayoutNode(b *flatbuffers.Builder, n LayoutNode) flatbuffers.UOffsetT {
	icon := b.CreateSharedString(n.Icon)
	// Empty strings are left out, so nodes other than masteries carry no mastery art fields.
	optional := func(s string) flatbuffers.UOffsetT {
		if s == "" {
			return 0
		}
		return b.CreateSharedString(s)
	}
	inactive, active, effect := optional(n.MasteryInactiveIcon), optional(n.MasteryActiveIcon), optional(n.MasteryActiveEffect)
	fb.LayoutNodeStart(b)
	fb.LayoutNodeAddId(b, n.ID)
	fb.LayoutNodeAddGroup(b, n.Group)
	fb.LayoutNodeAddOrbit(b, n.Orbit)
	fb.LayoutNodeAddOrbitIndex(b, n.OrbitIndex)
	fb.LayoutNodeAddIcon(b, icon)
	if inactive != 0 {
		fb.LayoutNodeAddMasteryInactiveIcon(b, inactive)
	}
	if active != 0 {
		fb.LayoutNodeAddMasteryActiveIcon(b, active)
	}
	if effect != 0 {
		fb.LayoutNodeAddMasteryActiveEffect(b, effect)
	}
	return fb.LayoutNodeEnd(b)
}

func decodeLayout(buf []byte) (Layout, error) {
	if !fb.LayoutBufferHasIdentifier(buf) {
		return Layout{}, errIdentifier
	}
	root := fb.GetRootAsLayout(buf, 0)
	l := Layout{
		Variant: string(root.Variant()),
		MinX:    root.MinX(), MinY: root.MinY(), MaxX: root.MaxX(), MaxY: root.MaxY(),
	}
	var o fb.Orbit
	for i := range root.OrbitsLength() {
		root.Orbits(&o, i)
		l.Orbits = append(l.Orbits, Orbit{Radius: o.Radius(), Slots: o.Slots()})
	}
	var g fb.LayoutGroup
	var bg fb.GroupBackground
	for i := range root.GroupsLength() {
		root.Groups(&g, i)
		lg := LayoutGroup{ID: g.Id(), X: g.X(), Y: g.Y(), Orbits: slices.Clone(g.OrbitsBytes()), Proxy: g.Proxy()}
		if g.Background(&bg) != nil {
			lg.Background = &GroupBackground{Image: string(bg.Image()), HalfImage: bg.HalfImage()}
		}
		l.Groups = append(l.Groups, lg)
	}
	var n fb.LayoutNode
	for i := range root.NodesLength() {
		root.Nodes(&n, i)
		l.Nodes = append(l.Nodes, LayoutNode{
			ID: n.Id(), Group: n.Group(), Orbit: n.Orbit(), OrbitIndex: n.OrbitIndex(), Icon: string(n.Icon()),
			MasteryInactiveIcon: string(n.MasteryInactiveIcon()),
			MasteryActiveIcon:   string(n.MasteryActiveIcon()),
			MasteryActiveEffect: string(n.MasteryActiveEffect()),
		})
	}
	var a fb.ClassArt
	for i := range root.ClassArtLength() {
		root.ClassArt(&a, i)
		l.ClassArt = append(l.ClassArt, ClassArt{Class: a.ClassId(), Image: string(a.Image()), X: a.X(), Y: a.Y()})
	}
	return l, nil
}
