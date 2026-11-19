package passives

import "github.com/xaroth/pbt-engine/dataset"

// StartDistances returns, for each of sockets, how many passives lie between it and the class
// start node start on the shortest route over allocated passives, the two ends not counted; 0
// when no such route joins them or the socket is not allocated. allocated are the allocated
// nodes, the passives cluster jewels add among them, with the links of each; start may be among
// them. A route steps only
// on allocated nodes, never leaves a mastery and passes no class or ascendancy start. This is how
// Path of Building counts the passives for Split Personality (PassiveSpec.lua,
// SetNodeDistanceToClassStart).
func StartDistances(allocated []*dataset.Node, start uint32, sockets ...uint32) []int {
	g := linksOf(allocated, start)
	out := make([]int, len(sockets))
	dist := make([]int, len(allocated))
	queue := make([]int, 0, len(allocated))
	for i, socket := range sockets {
		if from, ok := g.index[socket]; ok {
			out[i] = g.distance(from, dist, queue)
		}
	}
	return out
}

// links are allocated nodes by index, with the links among them and to the start, both ways:
// a cluster jewel's socket links to the first passive the jewel adds, not that passive to it.
type links struct {
	index    map[uint32]int
	next     [][]int
	toStart  []bool
	passable []bool
}

func linksOf(allocated []*dataset.Node, start uint32) *links {
	g := &links{
		index: make(map[uint32]int, len(allocated)), next: make([][]int, len(allocated)),
		toStart: make([]bool, len(allocated)), passable: make([]bool, len(allocated)),
	}
	for i, n := range allocated {
		g.index[n.ID] = i
		g.passable[i] = n.Kind != dataset.Mastery && n.Kind != dataset.ClassStart && n.Kind != dataset.AscendancyStart
	}
	for i, n := range allocated {
		for _, nb := range n.Neighbours {
			j, ok := g.index[nb]
			switch {
			case nb == start:
				g.toStart[i] = true
			case n.ID == start:
				if ok {
					g.toStart[j] = true
				}
			case ok:
				g.next[i] = append(g.next[i], j)
				g.next[j] = append(g.next[j], i)
			}
		}
	}
	return g
}

// distance runs a breadth-first search from node from to the start, with dist and queue as
// scratch space.
func (g *links) distance(from int, dist, queue []int) int {
	for i := range dist {
		dist[i] = -1
	}
	dist[from] = 0
	queue = append(queue[:0], from)
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		if i != from && !g.passable[i] {
			continue
		}
		if g.toStart[i] {
			return dist[i]
		}
		for _, j := range g.next[i] {
			if dist[j] < 0 {
				dist[j] = dist[i] + 1
				queue = append(queue, j)
			}
		}
	}
	return 0
}
