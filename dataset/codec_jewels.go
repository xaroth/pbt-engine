package dataset

import (
	"fmt"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func int32Vector(b *flatbuffers.Builder, start startFunc, vs []int32) flatbuffers.UOffsetT {
	start(b, len(vs))
	for _, v := range slices.Backward(vs) {
		b.PrependInt32(v)
	}
	return b.EndVector(len(vs))
}

func decodeInt32s(n int, get func(int) int32) []int32 {
	if n == 0 {
		return nil
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = get(i)
	}
	return out
}

// jewelStatLists returns the stats the jewel data of t uses, for the tree's stat table.
func jewelStatLists(t Tree) [][]StatValue {
	lists := make([][]StatValue, 0, len(t.Clusters.Notables))
	for _, n := range t.Clusters.Notables {
		lists = append(lists, []StatValue{{Stat: n.Stat}})
	}
	return append(lists, timelessStatLists(t.TimelessVersions)...)
}

func encodeJewelRadii(b *flatbuffers.Builder, radii []JewelRadius) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(radii))
	for i, r := range radii {
		id := b.CreateString(r.ID)
		fb.JewelRadiusStart(b)
		fb.JewelRadiusAddId(b, id)
		fb.JewelRadiusAddRadius(b, r.Radius)
		fb.JewelRadiusAddRingInner(b, r.RingInner)
		fb.JewelRadiusAddRingOuter(b, r.RingOuter)
		offs[i] = fb.JewelRadiusEnd(b)
	}
	return tableVector(b, fb.TreeStartJewelRadiiVector, offs)
}

func decodeJewelRadii(root *fb.Tree) []JewelRadius {
	if root.JewelRadiiLength() == 0 {
		return nil
	}
	out := make([]JewelRadius, root.JewelRadiiLength())
	var r fb.JewelRadius
	for i := range out {
		root.JewelRadii(&r, i)
		out[i] = JewelRadius{ID: string(r.Id()), Radius: r.Radius(), RingInner: r.RingInner(), RingOuter: r.RingOuter()}
	}
	return out
}

// encodeJewels encodes the jewel data of t and returns what adds it to the tree being built.
// It adds nothing for a tree without jewel data, so such a tree keeps its bytes.
func encodeJewels(b *flatbuffers.Builder, st statTable, t *Tree) (add func()) {
	var radii, clusters, timeless flatbuffers.UOffsetT
	if len(t.JewelRadii) > 0 {
		radii = encodeJewelRadii(b, t.JewelRadii)
	}
	if !t.Clusters.empty() {
		clusters = encodeClusters(b, st, &t.Clusters)
	}
	if len(t.TimelessVersions) > 0 {
		timeless = encodeTimeless(b, st, t.TimelessVersions)
	}
	return func() {
		if radii != 0 {
			fb.TreeAddJewelRadii(b, radii)
		}
		if clusters != 0 {
			fb.TreeAddClusters(b, clusters)
		}
		if timeless != 0 {
			fb.TreeAddTimelessVersions(b, timeless)
		}
	}
}

func encodeClusters(b *flatbuffers.Builder, st statTable, c *Clusters) flatbuffers.UOffsetT {
	sockets := encodeClusterSockets(b, c.Sockets)
	jewels := encodeClusterJewels(b, c.Jewels)
	skillOffs := make([]flatbuffers.UOffsetT, len(c.Skills))
	for i, s := range c.Skills {
		tag := b.CreateSharedString(s.Tag)
		fb.ClusterSkillStart(b)
		fb.ClusterSkillAddSize(b, fb.ClusterSize(s.Size))
		fb.ClusterSkillAddPassive(b, s.Passive)
		fb.ClusterSkillAddMastery(b, s.Mastery)
		fb.ClusterSkillAddTag(b, tag)
		skillOffs[i] = fb.ClusterSkillEnd(b)
	}
	notableOffs := make([]flatbuffers.UOffsetT, len(c.Notables))
	for i, n := range c.Notables {
		fb.ClusterNotableStart(b)
		fb.ClusterNotableAddPassive(b, n.Passive)
		fb.ClusterNotableAddStat(b, st.local[n.Stat])
		notableOffs[i] = fb.ClusterNotableEnd(b)
	}
	skills := tableVector(b, fb.ClustersStartSkillsVector, skillOffs)
	notables := tableVector(b, fb.ClustersStartNotablesVector, notableOffs)
	fb.ClustersStart(b)
	fb.ClustersAddSockets(b, sockets)
	fb.ClustersAddJewels(b, jewels)
	fb.ClustersAddSkills(b, skills)
	fb.ClustersAddNotables(b, notables)
	return fb.ClustersEnd(b)
}

func encodeClusterSockets(b *flatbuffers.Builder, sockets []ClusterSocket) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(sockets))
	for i, s := range sockets {
		starts := int32Vector(b, fb.ClusterSocketStartStartIndicesVector, s.StartIndices)
		fb.ClusterSocketStart(b)
		fb.ClusterSocketAddNode(b, s.Node)
		fb.ClusterSocketAddSize(b, fb.ClusterSize(s.Size))
		fb.ClusterSocketAddIndex(b, s.Index)
		fb.ClusterSocketAddProxy(b, s.Proxy)
		fb.ClusterSocketAddParent(b, s.Parent)
		fb.ClusterSocketAddStartIndices(b, starts)
		offs[i] = fb.ClusterSocketEnd(b)
	}
	return tableVector(b, fb.ClustersStartSocketsVector, offs)
}

func encodeClusterJewels(b *flatbuffers.Builder, jewels []ClusterJewel) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(jewels))
	for i, j := range jewels {
		base := b.CreateString(j.Base)
		small := int32Vector(b, fb.ClusterJewelStartSmallIndicesVector, j.SmallIndices)
		notable := int32Vector(b, fb.ClusterJewelStartNotableIndicesVector, j.NotableIndices)
		socket := int32Vector(b, fb.ClusterJewelStartSocketIndicesVector, j.SocketIndices)
		fb.ClusterJewelStart(b)
		fb.ClusterJewelAddSize(b, fb.ClusterSize(j.Size))
		fb.ClusterJewelAddBase(b, base)
		fb.ClusterJewelAddMinNodes(b, j.MinNodes)
		fb.ClusterJewelAddMaxNodes(b, j.MaxNodes)
		fb.ClusterJewelAddSmallIndices(b, small)
		fb.ClusterJewelAddNotableIndices(b, notable)
		fb.ClusterJewelAddSocketIndices(b, socket)
		fb.ClusterJewelAddTotalIndices(b, j.TotalIndices)
		offs[i] = fb.ClusterJewelEnd(b)
	}
	return tableVector(b, fb.ClustersStartJewelsVector, offs)
}

func decodeClusters(root *fb.Tree, h []uint32) (Clusters, error) {
	c := root.Clusters(nil)
	if c == nil {
		return Clusters{}, nil
	}
	var out Clusters
	var s fb.ClusterSocket
	for i := range c.SocketsLength() {
		c.Sockets(&s, i)
		out.Sockets = append(out.Sockets, ClusterSocket{
			Node: s.Node(), Size: ClusterSize(s.Size()), Index: s.Index(), Proxy: s.Proxy(), Parent: s.Parent(),
			StartIndices: decodeInt32s(s.StartIndicesLength(), s.StartIndices),
		})
	}
	var j fb.ClusterJewel
	for i := range c.JewelsLength() {
		c.Jewels(&j, i)
		out.Jewels = append(out.Jewels, ClusterJewel{
			Size: ClusterSize(j.Size()), Base: string(j.Base()), MinNodes: j.MinNodes(), MaxNodes: j.MaxNodes(),
			SmallIndices:   decodeInt32s(j.SmallIndicesLength(), j.SmallIndices),
			NotableIndices: decodeInt32s(j.NotableIndicesLength(), j.NotableIndices),
			SocketIndices:  decodeInt32s(j.SocketIndicesLength(), j.SocketIndices),
			TotalIndices:   j.TotalIndices(),
		})
	}
	var sk fb.ClusterSkill
	for i := range c.SkillsLength() {
		c.Skills(&sk, i)
		out.Skills = append(out.Skills, ClusterSkill{Size: ClusterSize(sk.Size()), Passive: sk.Passive(), Mastery: sk.Mastery(), Tag: string(sk.Tag())})
	}
	var n fb.ClusterNotable
	for i := range c.NotablesLength() {
		c.Notables(&n, i)
		if int(n.Stat()) >= len(h) {
			return Clusters{}, fmt.Errorf("cluster notable %d: stat %d out of range (%d ids): %w", n.Passive(), n.Stat(), len(h), ErrInvalid)
		}
		out.Notables = append(out.Notables, ClusterNotable{Passive: n.Passive(), Stat: h[n.Stat()]})
	}
	return out, nil
}
