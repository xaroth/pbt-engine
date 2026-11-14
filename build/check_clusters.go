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

// generateClusters adds the passives the build's cluster jewels add to the nodes Check knows:
// a jewel in one of the tree's own cluster sockets, then a jewel in a socket an added jewel
// adds, and so on. The socket a jewel is in links to the first passive it adds. A jewel that
// does not fit its socket adds nothing, as checkJewels leaves it out; one whose lines do not
// say what it adds warns. The generator is made only when a cluster jewel is in a cluster
// socket.
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
			if g, err = cluster.NewGenerator(c.store, c.build.Variant); errors.Is(err, cluster.ErrNoClusterData) {
				return nil
			} else if err != nil {
				return fmt.Errorf("build: %w", err)
			}
		}
		c.addSubgraph(g, id, &it)
	}
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
		c.warnf(ClusterJewelUnread, jewelSubject(socket), "its lines do not say which passives it adds; it adds none")
		return
	case err != nil:
		return
	}
	if c.generated == nil {
		c.generated, c.clusterRead = map[uint32]*dataset.Node{}, map[uint32][]string{}
	}
	c.clusterRead[socket] = slices.DeleteFunc(j.Lines, func(l string) bool { return slices.Contains(sg.Unread, l) })
	for i := range sg.Nodes {
		n := sg.Nodes[i].Node
		if t := c.node(n.ID); n.Kind == dataset.JewelSocket && t != nil {
			added := *t
			added.Neighbours = n.Neighbours
			n = added
		}
		c.generated[n.ID] = &n
	}
	in := *c.node(socket)
	in.Neighbours = append(slices.Clone(in.Neighbours), sg.Entrance)
	slices.Sort(in.Neighbours)
	c.generated[socket] = &in
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
	return Warning{
		Code: NodeNoClusterJewel, Subject: fmt.Sprintf("node %d", id),
		Message: "a passive only a cluster jewel adds, and no jewel in the build adds it; dropped",
	}, true
}
