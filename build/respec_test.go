package build_test

import (
	"cmp"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// respecced applies edits to c's build and checks that Check keeps all of it.
func respecced(t *testing.T, c *build.Checked, edits []build.Edit) *build.Checked {
	t.Helper()
	b := c.Build()
	if err := b.Apply(edits...); err != nil {
		t.Fatal(err)
	}
	after := checked(t, c.Store(), &b)
	got := after.Build()
	if diff := gocmp.Diff(b, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Check changed the build (-applied +checked):\n%s", diff)
	}
	return after
}

// change is a change of class, ascendancy or bloodline and the edits it should give, nil when
// it should report false.
type change struct {
	name   string
	change func() ([]build.Edit, bool)
	want   []build.Edit
}

// testChanges checks each change's edits on c, and that Check keeps the build they give.
func testChanges(t *testing.T, c *build.Checked, testCases []change) {
	t.Helper()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, ok := testCase.change()
			if want := testCase.want != nil; ok != want {
				t.Fatalf("got %v %v, want ok %v", got, ok, want)
			}
			if diff := gocmp.Diff(testCase.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if ok {
				respecced(t, c, got)
			}
		})
	}
}

// Berserker's start 21 is added to testStore's tree.
func TestChangeAscendancyAndClass(t *testing.T) {
	t.Parallel()
	d := testDataset()
	nodes := &d.Trees[0].Nodes
	*nodes = append(*nodes, dataset.Node{ID: 21, Kind: dataset.AscendancyStart, Ascendancy: "Berserker", Neighbours: []uint32{11}})
	slices.SortFunc(*nodes, func(a, b dataset.Node) int { return cmp.Compare(a.ID, b.ID) })
	s := openStore(t, d)
	b := juggernaut()
	b.Nodes = []uint32{1, 3, 4, 10, 13, 14, 20}
	b.Masteries = map[uint32]uint32{4: 7}
	c := checked(t, s, b)
	testCases := []change{
		{"another ascendancy", func() ([]build.Edit, bool) { return c.ChangeAscendancy("Berserker") }, []build.Edit{
			build.SetAscendancy("Berserker"), build.Deallocate(10, 13, 14, 20), build.Allocate(21),
		}},
		{"no ascendancy", func() ([]build.Edit, bool) { return c.ChangeAscendancy("") }, []build.Edit{
			build.SetAscendancy(""), build.Deallocate(10, 13, 14, 20),
		}},
		{"the same ascendancy", func() ([]build.Edit, bool) { return c.ChangeAscendancy("Juggernaut") }, nil},
		{"another class's ascendancy", func() ([]build.Edit, bool) { return c.ChangeAscendancy("Deadeye") }, nil},
		{"another class", func() ([]build.Edit, bool) { return c.ChangeClass(ranger) }, []build.Edit{
			build.SetClass(ranger), build.SetAscendancy(""), build.Deallocate(1, 3, 4, 10, 13, 14, 20), build.ClearMastery(4), build.Allocate(2),
		}},
		{"the same class", func() ([]build.Edit, bool) { return c.ChangeClass(marauder) }, nil},
		{"a class not in the tree", func() ([]build.Edit, bool) { return c.ChangeClass("Metadata/Characters/NotInTree") }, nil},
	}
	testChanges(t, c, testCases)
}

func TestChangeBloodline(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Bloodline = "Oshabi"
	b.Nodes = append(b.Nodes, 50, 51, 52)
	c := checked(t, testStore(t), b)
	testCases := []change{
		{"another bloodline", func() ([]build.Edit, bool) { return c.ChangeBloodline("Lycia") }, []build.Edit{
			build.SetBloodline("Lycia"), build.Deallocate(50, 51, 52), build.Allocate(60),
		}},
		{"no bloodline", func() ([]build.Edit, bool) { return c.ChangeBloodline("") }, []build.Edit{
			build.SetBloodline(""), build.Deallocate(50, 51, 52),
		}},
		{"the same bloodline", func() ([]build.Edit, bool) { return c.ChangeBloodline("Oshabi") }, nil},
		{"an ascendancy", func() ([]build.Edit, bool) { return c.ChangeBloodline("Berserker") }, nil},
		{"another class", func() ([]build.Edit, bool) { return c.ChangeClass(ranger) }, []build.Edit{
			build.SetClass(ranger), build.SetAscendancy(""), build.Deallocate(1, 3, 10, 50, 51, 52), build.Allocate(2),
		}},
	}
	testChanges(t, c, testCases)
}

// The Chieftain holds its ascendancy start and not its class start; the Berserker gets its
// start in place of the Chieftain's.
func TestRealChangeAscendancyAndClass(t *testing.T) {
	t.Parallel()
	c := realChieftain(t)

	t.Run("ascendancy", func(t *testing.T) {
		t.Parallel()
		edits, ok := c.ChangeAscendancy("Berserker")
		if !ok {
			t.Fatal("not changed")
		}
		after := respecced(t, c, edits)
		var main int
		for _, n := range after.Nodes() {
			switch {
			case n.Ascendancy == "":
				main++
			case n.Kind != dataset.AscendancyStart || n.Ascendancy != "Berserker":
				t.Errorf("node %d of %s kept", n.ID, n.Ascendancy)
			}
		}
		if before, _, _ := c.Points(); main == 0 || before == 0 {
			t.Errorf("main-tree nodes: %d after, %d points before; want them kept", main, before)
		}
	})

	t.Run("class", func(t *testing.T) {
		t.Parallel()
		edits, ok := c.ChangeClass(ranger)
		if !ok {
			t.Fatal("not changed")
		}
		after := respecced(t, c, edits)
		if got := after.Nodes(); len(got) != 0 {
			t.Errorf("%d nodes kept, want none", len(got))
		}
		if b := after.Build(); b.Ascendancy != "" || len(b.Masteries) != 0 {
			t.Errorf("ascendancy %q, masteries %v; want none", b.Ascendancy, b.Masteries)
		}
	})
}
