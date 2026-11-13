package build_test

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
)

const keyCampaign config.Key[string] = "campaign"

func TestClone(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Masteries = map[uint32]uint32{3: 7}
	c := b.Clone()
	c.Nodes[0] = 99
	c.Masteries[3] = 8
	c.Config["campaign"][1] = 'x'
	c.Config["new"] = json.RawMessage(`1`)
	if diff := cmp.Diff(juggernaut().Nodes, b.Nodes); diff != "" || b.Masteries[3] != 7 || string(b.Config["campaign"]) != `"acts-6-10"` || len(b.Config) != 2 {
		t.Errorf("original changed: %+v", b)
	}
}

func TestApply(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	set, err := build.SetConfig(keyCampaign, "complete")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Apply(
		build.Allocate(4, 3), build.Deallocate(10), build.SetMastery(4, 2), build.SetMastery(5, 1), build.ClearMastery(5),
		build.SetLevel(70), build.SetClass(ranger), build.SetAscendancy(""), set, build.ClearConfig("bandits"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := juggernaut()
	want.Nodes, want.Masteries, want.Level, want.Class, want.Ascendancy = []uint32{1, 3, 4}, map[uint32]uint32{4: 2}, 70, ranger, ""
	want.Config = config.Values{"campaign": json.RawMessage(`"complete"`)}
	if diff := cmp.Diff(want, b); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if err := b.Apply(build.SetLevel(1), build.Edit{Op: "rename"}); !errors.Is(err, build.ErrEditOp) || b.Level != 1 {
		t.Errorf("unknown op: got %v, level %d", err, b.Level)
	}
}

func TestEditJSON(t *testing.T) {
	t.Parallel()
	set, err := build.SetConfig(keyCampaign, "acts-1-5")
	if err != nil {
		t.Fatal(err)
	}
	edits := []build.Edit{build.Allocate(1, 2), build.SetMastery(3, 4), build.SetAscendancy(""), set, build.ClearConfig("bandits")}
	data, err := json.Marshal(edits)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"op":"allocate","nodes":[1,2]},{"op":"set-mastery","node":3,"effect":4},{"op":"set-ascendancy"},` +
		`{"op":"set-config","key":"campaign","value":"acts-1-5"},{"op":"clear-config","key":"bandits"}]`
	if string(data) != want {
		t.Errorf("got %s\nwant %s", data, want)
	}
	var back []build.Edit
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(edits, back); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
}

func randBuild(r *rand.Rand) *build.Build {
	b := &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard",
		Class: []string{marauder, ranger}[r.IntN(2)], Level: 1 + r.IntN(100),
	}
	if r.IntN(2) == 0 {
		b.Ascendancy = "Juggernaut"
	}
	if r.IntN(2) == 0 {
		b.Bloodline = "Oshabi"
	}
	for range r.IntN(8) {
		b.Nodes = append(b.Nodes, uint32(r.IntN(12)))
	}
	slices.Sort(b.Nodes)
	b.Nodes = slices.Compact(b.Nodes)
	randChoices(r, b)
	b.Skills = randSkills(r)
	b.Items = randItems(r)
	b.Jewels = randJewels(r)
	for _, k := range []string{"campaign", "bandits", "charges"} {
		if r.IntN(2) == 0 {
			if b.Config == nil {
				b.Config = config.Values{}
			}
			b.Config[k] = json.RawMessage(`"` + []string{"a", "b"}[r.IntN(2)] + `"`)
		}
	}
	return b
}

// randChoices gives b random mastery choices and overrides.
func randChoices(r *rand.Rand, b *build.Build) {
	for range r.IntN(3) {
		if b.Masteries == nil {
			b.Masteries = map[uint32]uint32{}
		}
		b.Masteries[uint32(r.IntN(4))] = uint32(r.IntN(3))
	}
	for range r.IntN(3) {
		if b.Overrides == nil {
			b.Overrides = map[uint32]string{}
		}
		b.Overrides[uint32(r.IntN(4))] = []string{"Valako4", "runegraft_8"}[r.IntN(2)]
	}
}

// Applying Diff(a, b) to a gives b, on random pairs.
func TestDiff(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(3, 5))
	for i := range 500 {
		a, b := randBuild(r), randBuild(r)
		edits, err := build.Diff(a, b)
		if err != nil {
			t.Fatal(err)
		}
		got := a.Clone()
		if err := got.Apply(edits...); err != nil {
			t.Fatal(err)
		}
		got.Version = b.Version
		if diff := cmp.Diff(b, got, cmpopts.EquateEmpty()); diff != "" {
			t.Fatalf("pair %d (-want +got):\n%s\nedits %+v", i, diff, edits)
		}
		if same, _ := build.Diff(a, a.Clone()); len(same) != 0 {
			t.Fatalf("pair %d: a build differs from its clone: %+v", i, same)
		}
	}
	other := juggernaut()
	other.Variant = "ruthless"
	if _, err := build.Diff(juggernaut(), other); !errors.Is(err, build.ErrNotComparable) {
		t.Errorf("other variant: got %v", err)
	}
}

func TestChoicesRoundTrip(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Masteries = map[uint32]uint32{10: 2, 3: 7}
	b.Overrides = map[uint32]string{3: "Valako4"}
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	b.Nodes = []uint32{1, 3, 10}
	if diff := cmp.Diff(b, got); diff != "" {
		t.Errorf("(-want +got):\n%s\n%s", diff, data)
	}
}
