package build

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/cluster"
)

// Warning codes of Check for cluster jewels.
const (
	ClusterJewelUnread Code = "cluster-jewel-unread"
	NodeNoClusterJewel Code = "node-no-cluster-jewel"
)

// generateClusters adds the passives the build's cluster jewels add to the nodes Check knows,
// numbered as Path of Building numbers them, so they can be allocated, paths run through them
// and their stats count. It adds a jewel in one of the tree's own cluster sockets, then a jewel
// in a socket an added jewel adds, and so on. The socket a jewel is in links to the first
// passive it adds. A jewel that does not fit its socket adds nothing, as checkJewels leaves it
// out; one whose lines do not say what it adds warns. The generator is read only when a
// cluster jewel is in a cluster socket.
func (c *Checked) generateClusters() error {
	var g *cluster.Generator
	done := map[uint32]bool{}
	for {
		id, ok := c.nextClusterSocket(done)
		if !ok {
			return nil
		}
		done[id] = true
		it := c.build.Jewels[id]
		if c.clusterJewel(it.Base) == nil {
			continue
		}
		if g == nil {
			var err error
			g, err = clusterGenerator(c.store, c.build.Variant)
			if errors.Is(err, cluster.ErrNoClusterData) {
				return nil
			} else if err != nil {
				return fmt.Errorf("build: %w", err)
			}
		}
		c.addSubgraph(g, id, &it)
	}
}

// generatorKey keys the cluster generator of a tree variant among a store's derived values.
type generatorKey struct{ variant string }

// clusterGenerator returns the cluster generator of the tree variant of s, made once per store:
// making one indexes every stat description, which costs far more than a Check.
func clusterGenerator(s *dataset.Store, variant string) (*cluster.Generator, error) {
	//nolint:wrapcheck // NewGenerator wraps its errors
	return dataset.Derived(s, generatorKey{variant}, func(s *dataset.Store) (*cluster.Generator, error) {
		return cluster.NewGenerator(s, variant)
	})
}

// nextClusterSocket returns the lowest socket not done that holds a jewel and is a cluster
// socket on the tree: one of the tree's own, or one a jewel added.
func (c *Checked) nextClusterSocket(done map[uint32]bool) (uint32, bool) {
	for _, id := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		if sock, ok := c.tree.ClusterSocket(id); ok && !done[id] && (sock.Parent == 0 || c.generated[id] != nil) {
			return id, true
		}
	}
	return 0, false
}

// addSubgraph adds what jewel it adds in socket, or warns when its lines do not say.
func (c *Checked) addSubgraph(g *cluster.Generator, socket uint32, it *Item) {
	j := cluster.Jewel{Base: it.Base}
	for _, m := range it.Mods {
		j.Lines = append(j.Lines, m.Text)
	}
	sg, err := g.Generate(socket, j)
	switch {
	case errors.Is(err, cluster.ErrUnread):
		c.warnAboutf(jewelAbout(socket), ClusterJewelUnread, "its lines do not say which passives it adds; it adds none")
		return
	case err != nil:
		return
	}
	if c.generated == nil {
		c.generated, c.clusterRead = map[uint32]*dataset.Node{}, map[uint32][]string{}
		c.clusterPlaced = map[uint32]ClusterGroup{}
	}
	grp := ClusterGroup{Socket: socket, X: sg.CentreX, Y: sg.CentreY, Radius: sg.Radius}
	c.clusterRead[socket] = slices.DeleteFunc(j.Lines, func(l string) bool { return slices.Contains(sg.Unread, l) })
	for i := range sg.Nodes {
		n := sg.Nodes[i].Node
		if t := c.node(n.ID); n.Kind == dataset.JewelSocket && t != nil {
			added := *t
			added.Neighbours = n.Neighbours
			n = added
		}
		c.generated[n.ID] = &n
		if cluster.IsGenerated(n.ID) {
			grp.Nodes = append(grp.Nodes, ClusterPlace{ID: n.ID, X: sg.Nodes[i].X, Y: sg.Nodes[i].Y, Passive: sg.Nodes[i].Passive})
		}
	}
	c.clusterPlaced[socket] = grp
	in := *c.node(socket)
	in.Neighbours = append(slices.Clone(in.Neighbours), sg.Entrance)
	slices.Sort(in.Neighbours)
	c.generated[socket] = &in
}

// ClusterNodes returns the passives the build's cluster jewels add, sorted by id. Tree does not
// have them; they do not depend on which nodes are allocated.
func (c *Checked) ClusterNodes() []*dataset.Node {
	var out []*dataset.Node
	for _, id := range slices.Sorted(maps.Keys(c.generated)) {
		if cluster.IsGenerated(id) {
			out = append(out, c.generated[id])
		}
	}
	return out
}

// ClusterGroup is where the passives one cluster jewel adds sit: around a centre, on an orbit of
// the given radius, in layout coordinates, so a link between neighbours can be drawn as an arc
// of that orbit.
type ClusterGroup struct {
	// Socket is the socket the jewel is in.
	Socket uint32 `json:"socket"`
	// X and Y are the centre of the orbit, Radius its radius.
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
	// Nodes are the passives the jewel adds, sorted by id. The sockets it adds are the tree's
	// own and not among them.
	Nodes []ClusterPlace `json:"nodes"`
}

// ClusterPlace is where a passive a cluster jewel adds sits, in layout coordinates.
type ClusterPlace struct {
	ID uint32  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	// Passive is the tree node whose art the passive borrows; 0 for a small passive that grants
	// nothing, which has none.
	Passive uint32 `json:"passive"`
}

// ClusterPlaces returns where the passives of each cluster jewel in the build sit, by socket.
// ClusterNodes has the passives themselves.
func (c *Checked) ClusterPlaces() []ClusterGroup {
	out := make([]ClusterGroup, 0, len(c.clusterPlaced))
	for _, id := range slices.Sorted(maps.Keys(c.clusterPlaced)) {
		out = append(out, c.clusterPlaced[id])
	}
	return out
}

// clusterJewel returns the cluster jewel layout of base, or nil when base is no cluster jewel.
func (c *Checked) clusterJewel(base string) *dataset.ClusterJewel {
	for i := range c.tree.Clusters.Jewels {
		if c.tree.Clusters.Jewels[i].Base == base {
			return &c.tree.Clusters.Jewels[i]
		}
	}
	return nil
}

// clusterNodeProblem returns the warning for allocating node id when it is a passive that only
// a cluster jewel adds, a generated passive or a socket on a cluster jewel, and no jewel in
// the build adds it.
func (c *Checked) clusterNodeProblem(id uint32) (Warning, bool) {
	if c.generated[id] != nil {
		return Warning{}, false
	}
	sock, nested := c.tree.ClusterSocket(id)
	if !cluster.IsGenerated(id) && (!nested || sock.Parent == 0) {
		return Warning{}, false
	}
	return nodeAbout(id).saying(NodeNoClusterJewel, "a passive only a cluster jewel adds, and no jewel in the build adds it; dropped"), true
}
