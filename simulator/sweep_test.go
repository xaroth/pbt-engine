package simulator_test

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// randomVariant is a mix of edits: node and mastery edits (fast path), choice options and
// shared mastery effects (full path through the check), and level changes (full path).
func randomVariant(r *rand.Rand) []build.Edit {
	var out []build.Edit
	for range 1 + r.IntN(3) {
		switch r.IntN(8) {
		case 0, 1:
			out = append(out, build.Allocate(uint32(2+r.IntN(5))))
		case 2:
			out = append(out, build.Deallocate(uint32(2+r.IntN(3))))
		case 3:
			out = append(out, build.Allocate(4), build.SetMastery(4, 7))
		case 4:
			out = append(out, build.Allocate(6), build.SetMastery(6, []uint32{7, 9}[r.IntN(2)]))
		case 5:
			out = append(out, build.Allocate(8, uint32(10+r.IntN(2))))
		case 6:
			out = append(out, build.ClearMastery(4))
		case 7:
			out = append(out, build.SetLevel(1+r.IntN(100)))
		}
	}
	return out
}

// Every variant's values equal simulating the edited build, bit for bit, whatever the number of
// workers; ranking equals sorting those values.
func TestSweepEqualsBruteForce(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	base := marauderBuild(70, 2, 4, 8, 10)
	base.Masteries = map[uint32]uint32{4: 7}
	r := rand.New(rand.NewPCG(10, 1))
	variants := make([][]build.Edit, 120)
	for i := range variants {
		variants[i] = randomVariant(r)
	}
	keys := []output.Key{poe1.Life, poe1.FireResistance, poe1.Mana}
	one, err := s.Sweep(base, variants, simulator.Outputs(keys...), simulator.Workers(1))
	if err != nil {
		t.Fatal(err)
	}
	many, err := s.Sweep(base, variants, simulator.Outputs(keys...), simulator.Workers(4))
	if err != nil {
		t.Fatal(err)
	}
	before := simulate(t, newSim(t), base)
	for i, v := range variants {
		b := base.Clone()
		if err := b.Apply(v...); err != nil {
			t.Fatal(err)
		}
		want := simulate(t, newSim(t), b)
		for _, k := range keys {
			for name, res := range map[string]simulator.Result{"1 worker": one[i], "4 workers": many[i]} {
				c := res.Changes[k]
				if res.Err != nil || res.Index != i || math.Float64bits(c.After) != math.Float64bits(get(t, want, k)) ||
					math.Float64bits(c.Before) != math.Float64bits(get(t, before, k)) {
					t.Fatalf("%s variant %d %v %s: %+v, err %v; want %v", name, i, v, k, c, res.Err, get(t, want, k))
				}
			}
		}
	}
	ranked, err := s.Sweep(base, variants, simulator.Outputs(poe1.Mana), simulator.RankBy(poe1.Life))
	if err != nil {
		t.Fatal(err)
	}
	brute := slices.Clone(one)
	slices.SortStableFunc(brute, func(a, b simulator.Result) int {
		return cmp.Compare(b.Changes[poe1.Life].Abs, a.Changes[poe1.Life].Abs)
	})
	for i := range brute {
		if ranked[i].Index != brute[i].Index {
			t.Fatalf("rank %d: variant %d, brute force %d", i, ranked[i].Index, brute[i].Index)
		}
	}
	if _, ok := ranked[0].Changes[poe1.Mana]; !ok {
		t.Error("asked output missing from ranked results")
	}
}

func TestSweepErrorsAndStreaming(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	base := marauderBuild(70, 2)
	res, err := s.Sweep(base, [][]build.Edit{{{Op: "rename"}}, {build.Allocate(99)}})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Err == nil || res[1].Err != nil || len(res[1].Warnings) == 0 || res[1].Warnings[0].Code != build.NodeUnknown {
		t.Errorf("results %+v", res)
	}
	bad := marauderBuild(70)
	bad.Class = "Metadata/Characters/Dex/Dex"
	if _, err := s.Sweep(bad, nil); err == nil {
		t.Error("unreconcilable base: no error")
	}
	variants := make([][]build.Edit, 200)
	for i := range variants {
		variants[i] = []build.Edit{build.Allocate(3)}
	}
	seq, err := s.SweepEach(base, variants, simulator.Workers(4))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for range seq {
		n++
		if n == 5 {
			break
		}
	}
	if got := simulate(t, s, base); get(t, got, poe1.Life) != get(t, simulate(t, newSim(t), base), poe1.Life) {
		t.Error("sim not usable after a stopped sweep")
	}
}

func TestSweepPaths(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	base := marauderBuild(70, 2, 4, 8, 10)
	base.Masteries = map[uint32]uint32{4: 7}
	for name, tc := range map[string]struct {
		edits []build.Edit
		fast  bool
	}{
		"allocate":              {[]build.Edit{build.Allocate(3, 5)}, true},
		"deallocate":            {[]build.Edit{build.Deallocate(2, 4)}, true},
		"other mastery effect":  {[]build.Edit{build.Allocate(6), build.SetMastery(6, 9)}, true},
		"switch mastery effect": {[]build.Edit{build.ClearMastery(4), build.Allocate(6), build.SetMastery(6, 7)}, true},
		"unknown node":          {[]build.Edit{build.Allocate(99)}, true},
		"shared effect twice":   {[]build.Edit{build.Allocate(6), build.SetMastery(6, 7)}, false},
		"choice option":         {[]build.Edit{build.Allocate(11)}, false},
		"level":                 {[]build.Edit{build.SetLevel(10)}, false},
		"config":                {[]build.Edit{build.ClearConfig("campaign")}, false},
	} {
		fast, err := simulator.FastPath(s, base, tc.edits)
		if err != nil || fast != tc.fast {
			t.Errorf("%s: fast %v, want %v (%v)", name, fast, tc.fast, err)
		}
	}
}
