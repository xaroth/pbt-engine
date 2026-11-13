// Package radius finds the passives that lie in a jewel's radius around its socket, from the
// tree layout's node positions.
//
// A passive is in the radius when the distance from the socket's position to its own is
// within the area, both bounds included. Only passives a jewel can act on count: the socket
// itself, masteries, class and ascendancy starts, ascendancy passives, cluster jewel proxies and
// the sockets in their groups, and nodes the layout does not draw are never in a radius. The
// passives a cluster jewel adds have no position in the layout and are not covered.
package radius

import (
	"errors"
	"fmt"

	"github.com/xaroth/pbt-engine/dataset"
)

// ErrNotSocket is a node that is not a jewel socket drawn on the tree.
var ErrNotSocket = errors.New("radius: not a jewel socket on the tree")

// Area is the part of the tree around a socket that a jewel reaches: the passives at a
// distance from Inner to Outer, in the layout's units.
type Area struct {
	Inner, Outer float64
}

// Disc is the area of a jewel of radius r.
func Disc(r dataset.JewelRadius) Area { return Area{Outer: float64(r.Radius)} }

// Ring is the ring of radius r that Thread of Hope reaches, which leaves out the disc inside
// it.
func Ring(r dataset.JewelRadius) Area {
	return Area{Inner: float64(r.RingInner), Outer: float64(r.RingOuter)}
}

// Index finds the passives in an area around the sockets of one tree variant.
type Index struct {
	// passives can be in a radius, sorted by id.
	passives []point
	sockets  map[uint32]point
}

type point struct {
	id   uint32
	x, y float64
}

// New indexes tree as layout, the layout of the same variant, draws it.
func New(tree *dataset.Tree, layout *dataset.Layout) *Index {
	proxyGroup := map[uint32]bool{}
	for _, g := range layout.Groups {
		if g.Proxy {
			proxyGroup[g.ID] = true
		}
	}
	group := make(map[uint32]uint32, len(layout.Nodes))
	for _, n := range layout.Nodes {
		group[n.ID] = n.Group
	}
	ix := &Index{sockets: map[uint32]point{}}
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		x, y, ok := layout.Position(n.ID)
		if !ok {
			continue
		}
		p := point{id: n.ID, x: x, y: y}
		// A socket a cluster jewel adds is placed in its proxy's group: a jewel in it has a
		// radius, but it is not a passive of the tree's own.
		inCluster := proxyGroup[group[n.ID]]
		if n.Kind == dataset.JewelSocket {
			ix.sockets[n.ID] = p
		}
		switch {
		case inCluster, n.Ascendancy != "":
		case n.Kind == dataset.Normal, n.Kind == dataset.Notable, n.Kind == dataset.Keystone, n.Kind == dataset.JewelSocket:
			ix.passives = append(ix.passives, p)
		}
	}
	return ix
}

// Nodes returns the passives in area a around socket, sorted by id. A node that is not a jewel
// socket on the tree returns ErrNotSocket.
func (ix *Index) Nodes(socket uint32, a Area) ([]uint32, error) {
	s, ok := ix.sockets[socket]
	if !ok {
		return nil, fmt.Errorf("%w: node %d", ErrNotSocket, socket)
	}
	inner, outer := a.Inner*a.Inner, a.Outer*a.Outer
	var out []uint32
	for _, p := range ix.passives {
		dx, dy := p.x-s.x, p.y-s.y
		if d := dx*dx + dy*dy; p.id != socket && inner <= d && d <= outer {
			out = append(out, p.id)
		}
	}
	return out, nil
}
