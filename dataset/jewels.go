package dataset

import (
	"cmp"
	"slices"
)

// JewelRadius is a jewel radius as the game's PassiveJewelRadii row gives it, in the tree's
// distance units: the disc of Radius, and the ring between RingInner and RingOuter that Thread
// of Hope uses.
type JewelRadius struct {
	// ID is the row's id, such as "Large".
	ID                           string
	Radius, RingInner, RingOuter int32
}

// ClusterSize is the size of a cluster jewel and of the sockets that take one.
type ClusterSize uint8

// Cluster sizes, in the game's PassiveTreeExpansionJewelSizes order.
const (
	SmallCluster ClusterSize = iota
	MediumCluster
	LargeCluster
)

func (s ClusterSize) String() string {
	switch s {
	case SmallCluster:
		return "small"
	case MediumCluster:
		return "medium"
	case LargeCluster:
		return "large"
	}
	return unknown
}

// Clusters is what cluster jewels need of a tree. Every list is empty in trees without cluster
// jewels. The passives a cluster jewel adds are nodes of the tree, without neighbours.
type Clusters struct {
	// Sockets are the jewel sockets that take a cluster jewel, sorted by Node.
	Sockets []ClusterSocket
	// Jewels lay out the nodes of each size, in game table order.
	Jewels []ClusterJewel
	// Skills are the kinds of small passive a jewel can add, in game table order.
	Skills []ClusterSkill
	// Notables are the notables and keystones a jewel mod can add, in game table order.
	Notables []ClusterNotable
}

// ClusterSocket is a jewel socket that takes a cluster jewel, from the game's PassiveJewelSlots.
type ClusterSocket struct {
	// Node is the socket's node, one of the tree's jewel slots.
	Node uint32
	Size ClusterSize
	// Index is which of its parent's sockets it is (ClusterIndex).
	Index int32
	// Proxy is the proxy node whose group places the nodes the jewel adds.
	Proxy uint32
	// Parent is the socket of the jewel this socket sits on, or 0 for the tree's own sockets.
	Parent uint32
	// StartIndices are the game's StartIndices.
	StartIndices []int32
}

// ClusterJewel is how a cluster jewel of one size lays out the nodes it adds, from the game's
// PassiveTreeExpansionJewels. Indices are orbit indices, below TotalIndices.
type ClusterJewel struct {
	Size ClusterSize
	// Base is the ItemBase.ID of the jewel.
	Base                                        string
	MinNodes, MaxNodes                          int32
	SmallIndices, NotableIndices, SocketIndices []int32
	TotalIndices                                int32
}

// ClusterSkill is a kind of small passive a cluster jewel adds, from the game's
// PassiveTreeExpansionSkills.
type ClusterSkill struct {
	Size ClusterSize
	// Passive is the small passive's node.
	Passive uint32
	// Mastery is the mastery's node, or 0.
	Mastery uint32
	// Tag is the Tags id the jewel's enchant names the kind by.
	Tag string
}

// ClusterNotable is a notable or keystone a cluster jewel mod adds, from the game's
// PassiveTreeExpansionSpecialSkills.
type ClusterNotable struct {
	// Passive is the node of the notable or keystone.
	Passive uint32
	// Stat is the handle of the stat on the jewel's mod that adds it.
	Stat uint32
}

// empty reports whether c holds nothing.
func (c *Clusters) empty() bool {
	return len(c.Sockets) == 0 && len(c.Jewels) == 0 && len(c.Skills) == 0 && len(c.Notables) == 0
}

// ClusterSocket returns the cluster socket of t at node.
func (t *Tree) ClusterSocket(node uint32) (*ClusterSocket, bool) {
	i, ok := slices.BinarySearchFunc(t.Clusters.Sockets, node, func(s ClusterSocket, n uint32) int { return cmp.Compare(s.Node, n) })
	if !ok {
		return nil, false
	}
	return &t.Clusters.Sockets[i], true
}
