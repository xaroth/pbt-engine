// Package cluster generates the passives a cluster jewel adds to the passive tree: from a
// socket and the jewel in it, the nodes with their ids, kinds, stats, links and positions.
//
// The game data gives each jewel size its orbit indices and each socket its proxy and start
// indices. The node ids, the moves of a medium jewel's indices, which sockets a jewel adds and
// the order of notables (by the row of their stat) follow Path of Building, whose ids every
// imported build uses. Jewels that add a keystone are not covered.
package cluster

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Errors of Generate; match them with errors.Is.
var (
	// ErrNotSocket is a node that is not a socket taking a cluster jewel.
	ErrNotSocket = errors.New("cluster: not a cluster jewel socket")
	// ErrNotCluster is a jewel whose base is not a cluster jewel.
	ErrNotCluster = errors.New("cluster: not a cluster jewel")
	// ErrTooLarge is a jewel larger than its socket.
	ErrTooLarge = errors.New("cluster: jewel larger than its socket")
	// ErrUnread is a jewel whose lines do not say which passives it adds: no node count, or a
	// small passive kind or notable the tree does not have.
	ErrUnread = errors.New("cluster: jewel lines not read")
	// ErrNoClusterData is a tree without cluster jewel data, as in datasets written before it.
	ErrNoClusterData = errors.New("cluster: tree has no cluster jewel data")
)

// Jewel is a cluster jewel as an item holds it: its base and its mod lines' English text, as
// the game or Path of Building writes them.
type Jewel struct {
	// Base is the dataset ItemBase.ID.
	Base  string
	Lines []string
}

// Subgraph is what a cluster jewel adds in one socket.
type Subgraph struct {
	// Socket is the socket node the jewel is in.
	Socket uint32
	// Size is the jewel's size, at most the socket's.
	Size dataset.ClusterSize
	// Skill is the kind of small passive the jewel adds, nil when its small passives grant
	// nothing.
	Skill *dataset.ClusterSkill
	// Nodes are sorted by ID. The first in orbit order, Entrance, links to Socket.
	Nodes    []Node
	Entrance uint32
	// Unread are the jewel's lines that no description reads, and lines adding a stat to small
	// passives that the stats group lacks; they are left out.
	Unread []string
}

// Node is a node a cluster jewel adds. Its ID is Path of Building's: bit 16 set, the orbit
// index in bits 0-3, the jewel size in bits 4-5, the index of the large socket in the chain in
// bits 6-8 and of the medium socket in bits 9-10. A jewel socket the jewel adds is the tree's
// own socket node and keeps its id.
type Node struct {
	// Node holds the ID, the kind (Normal, Notable, Keystone or JewelSocket), the stats sorted
	// by handle and the neighbours, sorted. The entrance's neighbours include the socket the
	// jewel is in.
	dataset.Node

	// Passive is the tree node the node is a copy of, which names its art: the small passive
	// kind's node, the notable's or keystone's node, or the socket itself. It is 0 for a small
	// passive that grants nothing.
	Passive uint32
	// OrbitIndex is the node's place on the jewel's ring, before the ring is turned to the
	// socket.
	OrbitIndex int32
	// X and Y are the node's position in layout coordinates; a socket's is the tree's.
	X, Y float64
}

// Generator generates subgraphs on one tree variant. It is safe for concurrent use.
type Generator struct {
	tree   *dataset.Tree
	layout *dataset.Layout
	set    *statdesc.Set
	// handles are the stat handles by stat id.
	handles map[string]uint32
	stats   []dataset.Stat
	// notables are the cluster notables by the stat of the jewel line that adds them.
	notables map[uint32]dataset.ClusterNotable
	// also are the handles of the stats small passives also grant, by the rest of the jewel
	// stat's id after alsoPrefix.
	also map[string]uint32
	// skillTexts are the described stats of each small passive kind's node, by kind.
	skillTexts map[*dataset.ClusterSkill][]string
}

// NewGenerator reads the tree, layout, stats and English stat descriptions of variant in s. A
// tree without cluster jewel data fails with ErrNoClusterData.
func NewGenerator(s *dataset.Store, variant string) (*Generator, error) {
	t, err := s.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("cluster: %w", err)
	}
	if len(t.Clusters.Sockets) == 0 || len(t.Clusters.Jewels) == 0 {
		return nil, fmt.Errorf("%w: %s %s", ErrNoClusterData, s.Manifest().Version, variant)
	}
	l, err := s.Layout(variant)
	if err != nil {
		return nil, fmt.Errorf("cluster: %w", err)
	}
	stats, err := s.Stats()
	if err != nil {
		return nil, fmt.Errorf("cluster: %w", err)
	}
	text, err := s.Text("en")
	if err != nil {
		return nil, fmt.Errorf("cluster: %w", err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return nil, fmt.Errorf("cluster: %w", err)
	}
	g := &Generator{tree: t, layout: l, set: set, stats: stats, handles: make(map[string]uint32, len(stats))}
	for i, st := range stats {
		g.handles[st.ID] = uint32(i)
	}
	g.indexAlso()
	g.notables = notablesByStat(t.Clusters.Notables)
	if err := g.indexSkills(); err != nil {
		return nil, err
	}
	return g, nil
}

// Generate returns what jewel j adds in socket. A socket that takes no cluster jewel fails
// with ErrNotSocket, a jewel that is not a cluster jewel with ErrNotCluster, one larger than
// the socket with ErrTooLarge, and one whose lines do not say what it adds with ErrUnread.
// Whether the socket itself is on the tree, which for a socket a jewel adds depends on the
// jewel holding it, is not checked.
func (g *Generator) Generate(socket uint32, j Jewel) (*Subgraph, error) {
	sock, ok := g.tree.ClusterSocket(socket)
	if !ok {
		return nil, fmt.Errorf("node %d: %w", socket, ErrNotSocket)
	}
	cj := g.jewel(j.Base)
	if cj == nil {
		return nil, fmt.Errorf("%s: %w", j.Base, ErrNotCluster)
	}
	if cj.Size > sock.Size {
		return nil, fmt.Errorf("%s jewel in %s socket %d: %w", cj.Size, sock.Size, socket, ErrTooLarge)
	}
	r, err := g.read(cj, j.Lines)
	if err != nil {
		return nil, err
	}
	sg := &Subgraph{Socket: socket, Size: cj.Size, Skill: r.skill}
	if err := g.build(sg, sock, cj, &r); err != nil {
		return nil, err
	}
	return sg, nil
}

// indexAlso finds the stats small passives also grant for each jewel stat that adds one.
func (g *Generator) indexAlso() {
	renamed := alsoRenamed()
	g.also = map[string]uint32{}
	for _, st := range g.stats {
		rest, ok := strings.CutPrefix(st.ID, alsoPrefix)
		if !ok {
			continue
		}
		to := rest
		if r, ok := renamed[rest]; ok {
			to = r
		}
		if h, ok := g.handles[to]; ok {
			g.also[rest] = h
		}
	}
}

// notablesByStat keys notables by the stat of the jewel line that adds them.
func notablesByStat(ns []dataset.ClusterNotable) map[uint32]dataset.ClusterNotable {
	m := make(map[uint32]dataset.ClusterNotable, len(ns))
	for _, n := range ns {
		m[n.Stat] = n
	}
	return m
}

// indexSkills describes each small passive kind's node, which the enchant names it by.
func (g *Generator) indexSkills() error {
	g.skillTexts = make(map[*dataset.ClusterSkill][]string, len(g.tree.Clusters.Skills))
	for i := range g.tree.Clusters.Skills {
		sk := &g.tree.Clusters.Skills[i]
		n := g.node(sk.Passive)
		if n == nil {
			return fmt.Errorf("cluster: small passive %d not in tree: %w", sk.Passive, ErrNoClusterData)
		}
		lines, err := g.set.Describe(treeScope, g.statMap(n.Stats))
		if err != nil {
			return fmt.Errorf("cluster: small passive %d: %w", sk.Passive, err)
		}
		texts := make([]string, len(lines))
		for j, l := range lines {
			texts[j] = l.Text
		}
		slices.Sort(texts)
		g.skillTexts[sk] = texts
	}
	return nil
}

// statMap returns stats by id.
func (g *Generator) statMap(stats []dataset.StatValue) map[string]int32 {
	m := make(map[string]int32, len(stats))
	for _, sv := range stats {
		m[g.stats[sv.Stat].ID] += sv.Value
	}
	return m
}

// node returns the tree node id, or nil.
func (g *Generator) node(id uint32) *dataset.Node {
	i, ok := slices.BinarySearchFunc(g.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &g.tree.Nodes[i]
}

// layoutNode returns the layout of node id, or nil.
func (g *Generator) layoutNode(id uint32) *dataset.LayoutNode {
	i, ok := slices.BinarySearchFunc(g.layout.Nodes, id, func(n dataset.LayoutNode, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &g.layout.Nodes[i]
}

// jewel returns the layout of cluster jewels with base, or nil.
func (g *Generator) jewel(base string) *dataset.ClusterJewel {
	for i := range g.tree.Clusters.Jewels {
		if g.tree.Clusters.Jewels[i].Base == base {
			return &g.tree.Clusters.Jewels[i]
		}
	}
	return nil
}
