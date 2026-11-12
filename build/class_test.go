package build_test

import (
	"encoding/json"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

func TestChangeClassAndAscendancy(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	b := juggernaut()
	b.Nodes = []uint32{1, 3, 4, 6, 10, 13, 14, 20}
	b.Masteries = map[uint32]uint32{4: 7}
	c := checked(t, s, b)
	testCases := []struct {
		name string
		edit func() ([]build.Edit, bool)
		want []build.Edit
	}{
		{"class", func() ([]build.Edit, bool) { return c.ChangeClass(ranger) },
			[]build.Edit{build.SetClass(ranger), build.SetAscendancy(""), build.Deallocate(1, 10, 13, 14, 20)}},
		{"class not in the tree", func() ([]build.Edit, bool) { return c.ChangeClass("Metadata/Characters/NotInTree") }, nil},
		{"ascendancy without a start", func() ([]build.Edit, bool) { return c.ChangeAscendancy("Berserker") },
			[]build.Edit{build.SetAscendancy("Berserker"), build.Deallocate(10, 13, 14, 20)}},
		{"no ascendancy", func() ([]build.Edit, bool) { return c.ChangeAscendancy("") },
			[]build.Edit{build.SetAscendancy(""), build.Deallocate(10, 13, 14, 20)}},
		{"ascendancy of another class", func() ([]build.Edit, bool) { return c.ChangeAscendancy("Deadeye") }, nil},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, ok := testCase.edit()
			if want := testCase.want != nil; ok != want {
				t.Fatalf("got %v %v, want ok %v", got, ok, want)
			}
			if diff := gocmp.Diff(testCase.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestChangeAscendancyAllocatesStart(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	b := juggernaut()
	b.Ascendancy, b.Nodes = "", []uint32{1, 3}
	got, ok := checked(t, s, b).ChangeAscendancy("Juggernaut")
	if diff := gocmp.Diff([]build.Edit{build.SetAscendancy("Juggernaut"), build.Allocate(20)}, got); !ok || diff != "" {
		t.Errorf("%v (-want +got):\n%s", ok, diff)
	}
}

// Class and ascendancy changes of the real Chieftain round-trip through JSON and the build file,
// and Check finds nothing wrong with the result. A node they drop does not join the new
// starts.
func TestRealChangeClassAndAscendancy(t *testing.T) {
	t.Parallel()
	c := realChieftain(t)
	if _, warnings, _ := build.Check(new(c.Build()), c.Store()); len(warnings) != 0 {
		t.Fatalf("base warnings: %v", warnings)
	}
	for _, change := range []struct {
		class, ascendancy string
	}{
		{"", "Juggernaut"},
		{"", ""},
		{"Metadata/Characters/StrDex/StrDex", ""},
		{"Metadata/Characters/StrDex/StrDex", "Slayer"},
		{"Metadata/Characters/Int/Int", "Elementalist"},
		{"Metadata/Characters/StrInt/StrInt", "Hierophant"},
		{"Metadata/Characters/StrDexInt/StrDexInt", "Ascendant"},
	} {
		t.Run(change.class+" "+change.ascendancy, func(t *testing.T) {
			t.Parallel()
			edits, from := changeEdits(t, c, change.class, change.ascendancy)
			data, err := json.Marshal(edits)
			if err != nil {
				t.Fatal(err)
			}
			var decoded []build.Edit
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			b := from.Build()
			if err := b.Apply(decoded...); err != nil {
				t.Fatal(err)
			}
			file, err := b.Encode()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := build.Parse(file)
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(b.Nodes)
			if diff := gocmp.Diff(&b, parsed, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Parse(Encode) (-want +got):\n%s", diff)
			}
			after, warnings, err := build.Check(parsed, c.Store())
			if err != nil || len(warnings) != 0 {
				t.Fatalf("Check: %v %v", err, warnings)
			}
			checkDropped(t, c, after)
			t.Logf("%s %s: %d of %d nodes kept", change.class, change.ascendancy, len(after.Nodes()), len(c.Nodes()))
		})
	}
}

// changeEdits returns the edits that change c's class, if class is set, then its ascendancy, as
// one list, and the checked build the ascendancy change was made from.
func changeEdits(t *testing.T, c *build.Checked, class, ascendancy string) ([]build.Edit, *build.Checked) {
	t.Helper()
	var edits []build.Edit
	if class != "" {
		e, ok := c.ChangeClass(class)
		if !ok {
			t.Fatalf("class %s", class)
		}
		b := c.Build()
		if err := b.Apply(e...); err != nil {
			t.Fatal(err)
		}
		c = checked(t, c.Store(), &b)
		edits = e
	}
	e, ok := c.ChangeAscendancy(ascendancy)
	if !ok {
		t.Fatalf("ascendancy %s", ascendancy)
	}
	return append(edits, e...), c
}

// checkDropped fails when a node allocated in before is gone from after although a path of
// after's nodes joins it to after's starts, and it is not another class's start or another
// ascendancy's node.
func checkDropped(t *testing.T, before, after *build.Checked) {
	t.Helper()
	bt := newBruteTree(after)
	for _, n := range before.Nodes() {
		if bt.allocated[n.ID] || n.Kind == dataset.ClassStart || (n.Ascendancy != "" && n.Ascendancy != bt.asc) {
			continue
		}
		bt.allocated[n.ID] = true
		if bt.joinedWithout(n.ID, 0) {
			t.Errorf("node %d dropped but joined", n.ID)
		}
		delete(bt.allocated, n.ID)
	}
}
