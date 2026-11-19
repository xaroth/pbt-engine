package passives_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/poe1/passives"
)

func TestStartDistances(t *testing.T) {
	t.Parallel()
	const start, socket = 1, 100
	node := func(id uint32, kind dataset.NodeKind, links ...uint32) *dataset.Node {
		return &dataset.Node{ID: id, Kind: kind, Neighbours: links}
	}
	cases := []struct {
		name      string
		allocated []*dataset.Node
		want      int
	}{
		{"next to the start", []*dataset.Node{node(socket, dataset.JewelSocket, start)}, 0},
		{
			"shortest of two routes",
			[]*dataset.Node{
				node(socket, dataset.JewelSocket, 10, 20),
				node(10, dataset.Normal, socket, start),
				node(20, dataset.Normal, socket, 21), node(21, dataset.Normal, 20, start),
			},
			1,
		},
		{
			"unallocated passives are not passed",
			[]*dataset.Node{node(socket, dataset.JewelSocket, 10, 20), node(20, dataset.Normal, socket, 21), node(21, dataset.Normal, 20, start)},
			2,
		},
		{
			"a mastery is not passed",
			[]*dataset.Node{
				node(socket, dataset.JewelSocket, 20, 30), node(30, dataset.Mastery, socket, start),
				node(20, dataset.Normal, socket, 21), node(21, dataset.Normal, 20, start),
			},
			2,
		},
		{
			// A cluster jewel's socket links to its first passive, which does not link back.
			"from a cluster's socket through its passives",
			[]*dataset.Node{
				node(socket, dataset.JewelSocket, 40), node(40, dataset.Normal, socket, 41),
				node(41, dataset.Notable), node(42, dataset.JewelSocket, start, 41),
			},
			3,
		},
		{
			"the start among the allocated, linking one way",
			[]*dataset.Node{node(start, dataset.ClassStart, 10), node(10, dataset.Normal, socket), node(socket, dataset.JewelSocket, 10)},
			1,
		},
		{"socket not allocated", []*dataset.Node{node(10, dataset.Normal, start)}, 0},
	}
	for _, c := range cases {
		if got := passives.StartDistances(c.allocated, start, socket); got[0] != c.want {
			t.Errorf("%s: %d, want %d", c.name, got[0], c.want)
		}
	}
}
