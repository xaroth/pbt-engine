package cluster

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Bits of a generated node id.
const (
	idFlag      = 1 << 16
	sizeShift   = 4
	largeShift  = 6
	mediumShift = 9
)

// Orbit indices with a rule of their own.
const (
	// largeOneSocket is where a large jewel with one socket puts it, the middle of its sockets.
	largeOneSocket = 6
	mediumFew      = 4 // passives of a medium jewel whose indices move
	mediumFive     = 5
)

// placed is a node at an orbit index before it is given an id.
type placed struct {
	index   int32
	kind    dataset.NodeKind
	passive uint32
}

// build fills sg with the nodes jewel layout cj adds in sock as r reads them.
func (g *Generator) build(sg *Subgraph, sock *dataset.ClusterSocket, cj *dataset.ClusterJewel, r *reading) error {
	ring, err := g.place(sock, cj, r)
	if err != nil {
		return err
	}
	base := g.idBase(sock, cj.Size)
	small := g.smallStats(r.skill, r)
	ids := make([]uint32, len(ring))
	orbit := g.orbitRadius(sock, cj.Size)
	for i, p := range ring {
		n := Node{ID: base + uint32(p.index), Kind: p.kind, Passive: p.passive, OrbitIndex: p.index} //nolint:gosec // orbit indices are below 16
		switch p.kind {
		case dataset.JewelSocket:
			n.ID = p.passive
		case dataset.Normal:
			n.Stats = slices.Clone(small)
		default:
			n.Stats = slices.Clone(g.node(p.passive).Stats)
		}
		n.X, n.Y = g.position(sock, cj, orbit, p.index)
		if p.kind == dataset.JewelSocket {
			n.X, n.Y, _ = g.layout.Position(n.ID)
		}
		ids[i] = n.ID
		sg.Nodes = append(sg.Nodes, n)
	}
	sg.Entrance = ids[0]
	for i := range sg.Nodes {
		sg.Nodes[i].Neighbours = ringNeighbours(ids, i, cj.Size != dataset.SmallCluster)
		if i == 0 {
			sg.Nodes[i].Neighbours = append(sg.Nodes[i].Neighbours, sock.Node)
		}
		slices.Sort(sg.Nodes[i].Neighbours)
		sg.Nodes[i].Neighbours = slices.Compact(sg.Nodes[i].Neighbours)
	}
	slices.SortFunc(sg.Nodes, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) })
	sg.Unread = r.unread
	return nil
}

// ringNeighbours returns the neighbours of ids[i] on a chain of ids in orbit order: the ones
// before and after it, the last linking back to the first when closed. Small jewels leave the
// chain open.
func ringNeighbours(ids []uint32, i int, closed bool) []uint32 {
	var out []uint32
	if i > 0 {
		out = append(out, ids[i-1])
	} else if closed && len(ids) > 1 {
		out = append(out, ids[len(ids)-1])
	}
	if i+1 < len(ids) {
		out = append(out, ids[i+1])
	} else if closed && len(ids) > 1 {
		out = append(out, ids[0])
	}
	return out
}

// idBase returns the id of orbit index 0 of a jewel of size in sock: the flag, the size and the
// indices of the large and medium sockets on the way from the tree to sock.
func (g *Generator) idBase(sock *dataset.ClusterSocket, size dataset.ClusterSize) uint32 {
	id := uint32(idFlag) | uint32(size)<<sizeShift
	for s, ok := sock, true; ok; s, ok = g.tree.ClusterSocket(s.Parent) {
		switch s.Size {
		case dataset.LargeCluster:
			id |= uint32(s.Index) << largeShift //nolint:gosec // socket indices are small
		case dataset.MediumCluster:
			id |= uint32(s.Index) << mediumShift //nolint:gosec // socket indices are small
		case dataset.SmallCluster:
		}
	}
	return id
}

// place returns the nodes of a jewel of layout cj in sock as r reads it, in orbit order: sockets
// first, then notables, then small passives each take the first free indices of their list,
// with the moves Path of Building makes for medium jewels. Notables go in stat order on their
// indices in ascending order.
func (g *Generator) place(sock *dataset.ClusterSocket, cj *dataset.ClusterJewel, r *reading) ([]placed, error) {
	taken := map[int32]bool{}
	var ring []placed
	sockets := cj.SocketIndices[:min(max(int(r.sockets), 0), len(cj.SocketIndices))]
	if cj.Size == dataset.LargeCluster && r.sockets == 1 {
		sockets = []int32{largeOneSocket}
	}
	for k, i := range sockets {
		id, err := g.socketAt(sock, cj, k, len(sockets))
		if err != nil {
			return nil, err
		}
		taken[i] = true
		ring = append(ring, placed{index: i, kind: dataset.JewelSocket, passive: id})
	}
	notables := pick(cj.NotableIndices, len(r.notables), taken, func(i int32) int32 { return moveNotable(cj, r, i) })
	slices.Sort(notables)
	for k, i := range notables {
		taken[i] = true
		ring = append(ring, placed{index: i, kind: g.node(r.notables[k].Passive).Kind, passive: r.notables[k].Passive})
	}
	smalls := int(r.nodes) - len(sockets) - len(notables)
	var passive uint32
	if r.skill != nil {
		passive = r.skill.Passive
	}
	for _, i := range pick(cj.SmallIndices, smalls, taken, func(i int32) int32 { return moveSmall(cj, r, i) }) {
		taken[i] = true
		ring = append(ring, placed{index: i, kind: dataset.Normal, passive: passive})
	}
	if !taken[0] {
		return nil, fmt.Errorf("%s with %d passives: no entrance: %w", cj.Base, r.nodes, ErrUnread)
	}
	slices.SortFunc(ring, func(a, b placed) int { return int(a.index - b.index) })
	return ring, nil
}

// pick returns up to n of indices, each moved, that are not taken, in list order.
func pick(indices []int32, n int, taken map[int32]bool, move func(int32) int32) []int32 {
	var out []int32
	for _, i := range indices {
		if len(out) >= n {
			break
		}
		if i = move(i); !taken[i] && !slices.Contains(out, i) {
			out = append(out, i)
		}
	}
	return out
}

// moveNotable moves a medium jewel's notable index: with two notables and no socket, 6 and 10
// to 4 and 8; with four passives, 10 and 2 to 9 and 3.
func moveNotable(cj *dataset.ClusterJewel, r *reading, i int32) int32 {
	if cj.Size != dataset.MediumCluster {
		return i
	}
	switch {
	case r.sockets == 0 && len(r.notables) == 2:
		return moved(i, [][2]int32{{6, 4}, {10, 8}})
	case r.nodes == mediumFew:
		return moved(i, [][2]int32{{10, 9}, {2, 3}})
	}
	return i
}

// moveSmall moves a medium jewel's small passive index: with five passives, 4 to 3; with four,
// 8 and 4 to 9 and 3.
func moveSmall(cj *dataset.ClusterJewel, r *reading, i int32) int32 {
	if cj.Size != dataset.MediumCluster {
		return i
	}
	switch r.nodes {
	case mediumFive:
		return moved(i, [][2]int32{{4, 3}})
	case mediumFew:
		return moved(i, [][2]int32{{8, 9}, {4, 3}})
	}
	return i
}

// moved returns where moves, pairs of from and to, move i.
func moved(i int32, moves [][2]int32) int32 {
	for _, m := range moves {
		if m[0] == i {
			return m[1]
		}
	}
	return i
}

// position returns where orbit index i of a jewel of layout cj in sock is drawn: on the orbit of
// the given radius around the socket's proxy's group, turned by the socket's start index for
// the jewel's size.
func (g *Generator) position(sock *dataset.ClusterSocket, cj *dataset.ClusterJewel, radius float64, i int32) (x, y float64) {
	p := g.layoutNode(sock.Proxy)
	gi, _ := slices.BinarySearchFunc(g.layout.Groups, p.Group, func(gr dataset.LayoutGroup, id uint32) int { return cmp.Compare(gr.ID, id) })
	grp := g.layout.Groups[gi]
	var start int32
	if int(cj.Size) < len(sock.StartIndices) {
		start = sock.StartIndices[cj.Size]
	}
	angle := 2 * math.Pi * float64((i+start)%cj.TotalIndices) / float64(cj.TotalIndices)
	sin, cos := math.Sincos(angle)
	return grp.X + radius*sin, grp.Y - radius*cos
}

// orbitRadius returns the radius a jewel of size draws on in sock: the orbit of the proxies of
// sockets of that size, or of sock's proxy when the tree has no such socket.
func (g *Generator) orbitRadius(sock *dataset.ClusterSocket, size dataset.ClusterSize) float64 {
	proxy := sock.Proxy
	for _, s := range g.tree.Clusters.Sockets {
		if s.Size == size {
			proxy = s.Proxy
			break
		}
	}
	return g.layout.Orbits[g.layoutNode(proxy).Orbit].Radius
}

// socketOrder is the cluster index of the k-th socket a jewel adds.
func socketOrder() []int32 { return []int32{0, 2, 1} }

// socketAt returns the tree socket on sock that is the k-th of n sockets a jewel of layout cj
// adds: by cluster index 0, 2, 1 in turn, or 1 for a large jewel's only socket.
func (g *Generator) socketAt(sock *dataset.ClusterSocket, cj *dataset.ClusterJewel, k, n int) (uint32, error) {
	index := socketOrder()[k]
	if cj.Size == dataset.LargeCluster && n == 1 {
		index = 1
	}
	for _, s := range g.tree.Clusters.Sockets {
		if s.Parent == sock.Node && s.Index == index {
			return s.Node, nil
		}
	}
	return 0, fmt.Errorf("%s in socket %d: no socket of cluster index %d: %w", cj.Base, sock.Node, index, ErrNoClusterData)
}
