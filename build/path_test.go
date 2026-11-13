package build_test

import (
	"cmp"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

func checked(t *testing.T, s *dataset.Store, b *build.Build) *build.Checked {
	t.Helper()
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// As in the game's tree, ascendancy start 20 is linked to class start 1.
func TestAllocatePath(t *testing.T) {
	t.Parallel()
	d := testDataset()
	for i, n := range d.Trees[0].Nodes {
		switch n.ID {
		case 1, 20:
			d.Trees[0].Nodes[i].Neighbours = slices.Sorted(slices.Values(append(n.Neighbours, 21-n.ID)))
		}
	}
	s := openStore(t, d)
	testCases := []struct {
		name       string
		ascendancy string
		nodes      []uint32
		node       uint32
		want       []uint32
	}{
		{"from the class start", "Juggernaut", []uint32{1}, 5, []uint32{3, 5}},
		{"from the ascendancy start", "Juggernaut", []uint32{1}, 14, []uint32{13, 14}},
		{"a mastery", "Juggernaut", []uint32{1, 3}, 4, []uint32{4}},
		{"not through a mastery", "Juggernaut", []uint32{1, 3}, 30, nil},
		{"allocated", "Juggernaut", []uint32{1, 3}, 3, nil},
		{"the class start", "Juggernaut", []uint32{3}, 1, nil},
		{"another class start", "Juggernaut", []uint32{1, 3}, 2, nil},
		{"another ascendancy", "Juggernaut", []uint32{1, 3}, 11, nil},
		{"an ascendancy start", "", []uint32{1}, 20, nil},
		{"another option of a taken choice", "Juggernaut", []uint32{1, 13, 14}, 15, nil},
		{"joins nothing", "Juggernaut", []uint32{1}, 6, nil},
		{"not in the tree", "Juggernaut", []uint32{1}, 99, nil},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Ascendancy, b.Nodes = testCase.ascendancy, testCase.nodes
			got, ok := checked(t, s, b).AllocatePath(testCase.node)
			if want := testCase.want != nil; ok != want {
				t.Fatalf("got %v %v, want ok %v", got, ok, want)
			}
			if ok {
				if diff := gocmp.Diff(build.Allocate(testCase.want...), got); diff != "" {
					t.Errorf("(-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDeallocateBranch(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	testCases := []struct {
		name string
		node uint32
		want []build.Edit
	}{
		{"with a mastery", 3, []build.Edit{build.Deallocate(3, 4), build.ClearMastery(4)}},
		{"with an option", 13, []build.Edit{build.Deallocate(13, 14)}},
		{"a leaf", 10, []build.Edit{build.Deallocate(10)}},
		{"the class start", 1, []build.Edit{build.Deallocate(1)}},
		{"a node joined to nothing before", 6, []build.Edit{build.Deallocate(6)}},
		{"not allocated", 12, nil},
	}
	b := juggernaut()
	b.Nodes = []uint32{1, 3, 4, 6, 10, 13, 14}
	b.Masteries = map[uint32]uint32{4: 7}
	c := checked(t, s, b)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, ok := c.DeallocateBranch(testCase.node)
			if want := testCase.want != nil; ok != want {
				t.Fatalf("got %v %v, want ok %v", got, ok, want)
			}
			if diff := gocmp.Diff(testCase.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// realChieftain is the level 90 Chieftain of pbt-conformance, checked against the current
// pinned dataset.
func realChieftain(t *testing.T) *build.Checked {
	t.Helper()
	s := realdata.Open(t, realdata.Current)
	data, err := os.ReadFile(filepath.Join("..", "internal", "poe1", "testdata", "explain", "chieftain-L90-3.29.nodes"))
	if err != nil {
		t.Fatal(err)
	}
	b := &build.Build{Format: build.Format, Game: "poe1", Version: "3.29", Class: marauder, Ascendancy: "Chieftain", Level: 90}
	for f := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		n, err := strconv.ParseUint(f, 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		b.Nodes = append(b.Nodes, uint32(n))
	}
	return checked(t, s, b)
}

// Ascendant nodes of the pinned dataset: "Path of the Ranger" joins main-tree node 39821 and
// "Path of the Marauder" joins main-tree node 31628, both near those classes' starts.
const (
	pathOfTheRanger    = 7618
	pathOfTheMarauder  = 24755
	besideRangerPath   = 39821
	besideMarauderPath = 31628
)

// scionAscendant returns a checked Scion Ascendant with nodes allocated, each by AllocatePath.
func scionAscendant(t *testing.T, s *dataset.Store, nodes ...uint32) *build.Checked {
	t.Helper()
	c, err := build.New(s, "Metadata/Characters/StrDexInt/StrDexInt")
	if err != nil {
		t.Fatal(err)
	}
	b := c.Build()
	b.Ascendancy = "Ascendant"
	c = checked(t, s, &b)
	for _, n := range nodes {
		e, ok := c.AllocatePath(n)
		if !ok {
			t.Fatalf("no path to %d", n)
		}
		b := c.Build()
		if err := b.Apply(e); err != nil {
			t.Fatal(err)
		}
		c = checked(t, s, &b)
	}
	return c
}

// As in the game, a path leaves an ascendancy for the main tree only from an allocated node.
func TestRealAscendantPaths(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)

	t.Run("not through an unallocated Path of", func(t *testing.T) {
		t.Parallel()
		c := scionAscendant(t, s)
		e, ok := c.AllocatePath(besideRangerPath)
		if !ok {
			t.Fatal("no path")
		}
		for _, n := range c.Tree().Nodes {
			if n.Ascendancy != "" && slices.Contains(e.Nodes, n.ID) {
				t.Errorf("path %v takes node %d of %s, want main-tree nodes only", e.Nodes, n.ID, n.Ascendancy)
			}
		}
	})

	t.Run("from an allocated Path of", func(t *testing.T) {
		t.Parallel()
		c := scionAscendant(t, s, pathOfTheMarauder)
		e, ok := c.AllocatePath(besideMarauderPath)
		if diff := gocmp.Diff(build.Allocate(besideMarauderPath), e); !ok || diff != "" {
			t.Fatalf("ok %v (-want +got):\n%s", ok, diff)
		}
		b := c.Build()
		if err := b.Apply(e); err != nil {
			t.Fatal(err)
		}
		edits, _ := checked(t, s, &b).DeallocateBranch(pathOfTheMarauder)
		if len(edits) == 0 || !slices.Contains(edits[0].Nodes, besideMarauderPath) {
			t.Errorf("deallocating Path of the Marauder: got %v, want %d among them", edits, besideMarauderPath)
		}
	})
}

// bruteTree answers the questions of AllocatePath and DeallocateBranch by exhaustive search,
// with the tree's rules stated apart from the code under test.
type bruteTree struct {
	nodes     map[uint32]*dataset.Node
	starts    map[uint32]bool
	allocated map[uint32]bool
	asc       string
}

func newBruteTree(c *build.Checked) *bruteTree {
	bt := &bruteTree{nodes: map[uint32]*dataset.Node{}, starts: map[uint32]bool{c.Class().StartNode: true}, allocated: map[uint32]bool{}}
	bt.asc = c.Build().Ascendancy
	for i := range c.Tree().Nodes {
		n := &c.Tree().Nodes[i]
		bt.nodes[n.ID] = n
		if n.Kind == dataset.AscendancyStart && bt.asc != "" && n.Ascendancy == bt.asc {
			bt.starts[n.ID] = true
		}
	}
	for _, n := range c.Nodes() {
		bt.allocated[n.ID] = true
	}
	return bt
}

func (bt *bruteTree) choice(n *dataset.Node) uint32 {
	if !n.MultipleChoiceOption {
		return 0
	}
	for _, nb := range n.Neighbours {
		if bt.nodes[nb].MultipleChoice {
			return nb
		}
	}
	return 0
}

// step reports whether a path may go from u to its neighbour v: not out of a mastery, and within
// the main tree or one ascendancy, or out of the ascendancy into the main tree from an allocated
// u or a start.
func (bt *bruteTree) step(u, v uint32) bool {
	if bt.nodes[u].Kind == dataset.Mastery {
		return false
	}
	au, av := bt.nodes[u].Ascendancy, bt.nodes[v].Ascendancy
	return au == av || (av == "" && (bt.allocated[u] || bt.starts[u]))
}

func (bt *bruteTree) enterable(id uint32) bool {
	n := bt.nodes[id]
	if n.Kind == dataset.ClassStart || n.Kind == dataset.AscendancyStart || (n.Ascendancy != "" && n.Ascendancy != bt.asc) {
		return false
	}
	if p := bt.choice(n); p != 0 {
		for a := range bt.allocated {
			if a != id && bt.choice(bt.nodes[a]) == p {
				return false
			}
		}
	}
	return true
}

// distance returns how many unallocated nodes the shortest path to id takes, by relaxing every
// edge until nothing changes, or -1.
func (bt *bruteTree) distance(id uint32) int {
	dist := map[uint32]int{}
	for n := range bt.allocated {
		dist[n] = 0
	}
	for n := range bt.starts {
		dist[n] = 0
	}
	for bt.relax(dist) {
	}
	if d, ok := dist[id]; ok {
		return d
	}
	return -1
}

// relax lowers the distance of every enterable node by one of its neighbours, other than a
// mastery, and reports whether one changed.
func (bt *bruteTree) relax(dist map[uint32]int) bool {
	changed := false
	for v, n := range bt.nodes {
		if bt.allocated[v] || bt.starts[v] || !bt.enterable(v) {
			continue
		}
		for _, u := range n.Neighbours {
			du, ok := dist[u]
			if !ok || !bt.step(u, v) {
				continue
			}
			if dv, ok := dist[v]; !ok || du+1 < dv {
				dist[v], changed = du+1, true
			}
		}
	}
	return changed
}

// validPath reports whether path is a walk of enterable unallocated nodes from the allocated
// tree, through no mastery, to id.
func (bt *bruteTree) validPath(path []uint32, id uint32) bool {
	if len(path) == 0 || path[len(path)-1] != id {
		return false
	}
	prev := map[uint32]bool{}
	for a := range bt.allocated {
		prev[a] = true
	}
	for s := range bt.starts {
		prev[s] = true
	}
	for _, p := range path {
		if bt.allocated[p] || !bt.enterable(p) || !slices.ContainsFunc(bt.nodes[p].Neighbours, func(nb uint32) bool {
			return prev[nb] && bt.step(nb, p)
		}) {
			return false
		}
		prev = map[uint32]bool{p: true}
	}
	return true
}

// joinedWithout reports whether a path of allocated nodes other than gone joins id to a start,
// passing no mastery: a depth-first search from id.
func (bt *bruteTree) joinedWithout(id, gone uint32) bool {
	seen := map[uint32]bool{id: true}
	stack := []uint32{id}
	for len(stack) > 0 {
		u := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if bt.starts[u] {
			return true
		}
		for _, nb := range bt.nodes[u].Neighbours {
			// The search runs towards a start, so the path steps from nb to u; step keeps masteries
			// other than id off the stack.
			if !seen[nb] && nb != gone && (bt.allocated[nb] || bt.starts[nb]) && bt.step(nb, u) {
				seen[nb] = true
				stack = append(stack, nb)
			}
		}
	}
	return false
}

// Paths and deallocations on 100 random nodes each equal an exhaustive search, on a Chieftain
// and on an Ascendant whose main tree hangs off two "Path of" nodes.
func TestRealPathsEqualBruteForce(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		build func(t *testing.T) *build.Checked
	}{
		{"Chieftain", realChieftain},
		{"Ascendant", func(t *testing.T) *build.Checked {
			t.Helper()
			return scionAscendant(t, realdata.Open(t, realdata.Current), pathOfTheMarauder, besideMarauderPath, pathOfTheRanger, besideRangerPath)
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			c := testCase.build(t)
			bt := newBruteTree(c)
			r := rand.New(rand.NewPCG(1, 2))
			pathsEqualBruteForce(t, c, bt, r)
			deallocationsEqualBruteForce(t, c, bt, r)
		})
	}
}

func pathsEqualBruteForce(t *testing.T, c *build.Checked, bt *bruteTree, r *rand.Rand) {
	t.Helper()
	tree := c.Tree().Nodes
	var found int
	for range 100 {
		id := tree[r.IntN(len(tree))].ID
		got, ok := c.AllocatePath(id)
		want := bt.distance(id)
		switch {
		case bt.allocated[id] || want < 0:
			if ok {
				t.Errorf("node %d: path %v, want none", id, got.Nodes)
			}
		case !ok || len(got.Nodes) != want || !bt.validPath(got.Nodes, id):
			t.Errorf("node %d: path %v %v, want a valid path of %d", id, got.Nodes, ok, want)
		default:
			found++
		}
	}
	t.Logf("%d of 100 nodes reachable", found)
}

func deallocationsEqualBruteForce(t *testing.T, c *build.Checked, bt *bruteTree, r *rand.Rand) {
	t.Helper()
	allocated := c.Nodes()
	var cascades int
	for range 100 {
		gone := allocated[r.IntN(len(allocated))].ID
		want := []uint32{gone}
		for a := range bt.allocated {
			if a != gone && bt.joinedWithout(a, 0) && !bt.joinedWithout(a, gone) {
				want = append(want, a)
			}
		}
		slices.SortFunc(want, cmp.Compare)
		if len(want) > 1 {
			cascades++
		}
		got, ok := c.DeallocateBranch(gone)
		if !ok || got[0].Op != build.OpDeallocate {
			t.Fatalf("node %d: %v %v", gone, got, ok)
		}
		if diff := gocmp.Diff(want, got[0].Nodes); diff != "" {
			t.Errorf("node %d (-want +got):\n%s", gone, diff)
		}
	}
	t.Logf("%d of 100 deallocations take other nodes", cascades)
}
