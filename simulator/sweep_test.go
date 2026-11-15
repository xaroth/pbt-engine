package simulator_test

import (
	"cmp"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/benchbuilds"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/output"
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

// dirtyBases are bases the check changes: a choice on an unallocated mastery, two options of
// one choice node, one effect on two masteries, an unknown node.
func dirtyBases() map[string]*build.Build {
	stale := marauderBuild(70, 2, 4)
	stale.Masteries = map[uint32]uint32{4: 7, 6: 9}
	options := marauderBuild(70, 2, 8, 10, 11)
	shared := marauderBuild(70, 4, 6)
	shared.Masteries = map[uint32]uint32{4: 7, 6: 7}
	unknown := marauderBuild(70, 2, 99)
	return map[string]*build.Build{"stale mastery choice": stale, "two options": options, "shared effect": shared, "unknown node": unknown}
}

// The reviewer's cases: an edit brings back what the check dropped from the base.
func TestSweepDirtyBase(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	bases := dirtyBases()
	for name, tc := range map[string]struct {
		base  *build.Build
		edits []build.Edit
	}{
		"stale mastery choice comes back": {bases["stale mastery choice"], []build.Edit{build.Allocate(6)}},
		"dropped option comes back":       {bases["two options"], []build.Edit{build.Deallocate(10)}},
		"dropped repeated effect":         {bases["shared effect"], []build.Edit{build.ClearMastery(4)}},
	} {
		res, err := s.Sweep(tc.base, [][]build.Edit{tc.edits}, simulator.Outputs(poe1.Life))
		if err != nil {
			t.Fatal(err)
		}
		b := tc.base.Clone()
		if err := b.Apply(tc.edits...); err != nil {
			t.Fatal(err)
		}
		if got, want := res[0].Changes[poe1.Life].After, get(t, simulate(t, newSim(t), b), poe1.Life); got != want {
			t.Errorf("%s: sweep %v, simulate %v", name, got, want)
		}
	}
}

// Every variant's values equal simulating the edited build, bit for bit, whatever the number of
// workers, on clean and dirty bases; ranking equals sorting those values.
func TestSweepEqualsBruteForce(t *testing.T) {
	t.Parallel()
	clean := marauderBuild(70, 2, 4, 8, 10)
	clean.Masteries = map[uint32]uint32{4: 7}
	bases := dirtyBases()
	bases["clean"] = clean
	for name, base := range bases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bruteForce(t, base)
		})
	}
}

func bruteForce(t *testing.T, base *build.Build) {
	t.Helper()
	s := newSim(t)
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
		checkVariant(t, base, before, i, v, keys, map[string]simulator.Result{"1 worker": one[i], "4 workers": many[i]})
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

// checkVariant compares results of variant i with simulating the edited build.
func checkVariant(t *testing.T, base *build.Build, before *simulator.Stats, i int, v []build.Edit, keys []output.Key, results map[string]simulator.Result) {
	t.Helper()
	b := base.Clone()
	if err := b.Apply(v...); err != nil {
		t.Fatal(err)
	}
	want := simulate(t, newSim(t), b)
	for _, k := range keys {
		for name, res := range results {
			c := res.Changes[k]
			if res.Err != nil || res.Index != i || math.Float64bits(c.After) != math.Float64bits(get(t, want, k)) ||
				math.Float64bits(c.Before) != math.Float64bits(get(t, before, k)) {
				t.Fatalf("%s variant %d %v %s: %+v, err %v; want %v", name, i, v, k, c, res.Err, get(t, want, k))
			}
		}
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

// Fast and full variants report the same warnings: those about their edits.
func TestSweepWarningsAlike(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	base := marauderBuild(70, 2, 99)
	res, err := s.Sweep(base, [][]build.Edit{
		{build.Allocate(98)},                        // fast
		{build.Allocate(98), build.SetLevel(10)},    // full
		{build.SetMastery(4, 7)},                    // fast: 4 not allocated
		{build.SetMastery(4, 7), build.SetLevel(9)}, // full
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(res); i += 2 {
		if fast, full := res[i].Warnings, res[i+1].Warnings; len(fast) != 1 || !slices.Equal(fast, full) {
			t.Errorf("variants %d and %d: %v and %v", i, i+1, fast, full)
		}
	}
	if res[0].Warnings[0].Code != build.NodeUnknown || res[2].Warnings[0].Code != build.MasteryNotAllocated {
		t.Errorf("codes %v, %v", res[0].Warnings, res[2].Warnings)
	}
}

const ignitedRing = `Rarity: RARE
Doom Loop
Ruby Ring
Item Level: 85
Implicits: 1
+25% to Fire Resistance
40% increased Damage with Hits against Ignited Enemies
`

// A variant on the fast path derives the conditions again after switching its nodes: on the
// elementalist bench build, with a ring that gives damage with hits against Ignited enemies,
// deallocating node 22088 from it and allocating it on the build without it give what
// simulating the edited build gives.
func TestSweepDerivesConditions(t *testing.T) {
	t.Parallel()
	d := benchbuilds.Open(t)
	var b *build.Build
	for _, c := range benchbuilds.Builds(t, d) {
		if c.Name == "elementalist" {
			b = c.Build
		}
	}
	sim, err := simulator.New(d.Store)
	if err != nil {
		t.Fatal(err)
	}
	ring, warnings, err := sim.Data().Item(ignitedRing)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the ring: %v, %v", warnings, err)
	}
	if err := b.Apply(build.Equip(build.SlotRing1, ring)); err != nil {
		t.Fatal(err)
	}
	without := b.Clone()
	if err := without.Apply(build.Deallocate(22088)); err != nil {
		t.Fatal(err)
	}
	// Without 22088 the build's hits can ignite.
	for _, c := range []struct {
		base        *build.Build
		edit        build.Edit
		wantIgnited string
	}{
		{b, build.Deallocate(22088), "true"},
		{without, build.Allocate(22088), "false"},
	} {
		v := []build.Edit{c.edit}
		res, err := sim.Sweep(c.base, [][]build.Edit{v}, simulator.Outputs(poe1.AverageHit))
		if err != nil {
			t.Fatal(err)
		}
		if fast, err := simulator.FastPath(sim, c.base, v); err != nil || !fast {
			t.Fatalf("variant %v: fast path %v, %v", v, fast, err)
		}
		e := c.base.Clone()
		if err := e.Apply(v...); err != nil {
			t.Fatal(err)
		}
		st := simulate(t, sim, e)
		if on, _ := configOf(st, poe1.EnemyIgnited); on != c.wantIgnited {
			t.Fatalf("variant %v: ignited %s, want %s", v, on, c.wantIgnited)
		}
		want := get(t, st, poe1.AverageHit)
		if got := res[0].Changes[poe1.AverageHit].After; res[0].Err != nil || math.Float64bits(got) != math.Float64bits(want) {
			t.Errorf("variant %v: sweep %v (%v), simulate %v", v, got, res[0].Err, want)
		}
	}
}

// On each benchmark build, deallocating each allocated node singly and allocating a sample of
// the unallocated nodes next to allocated ones give, from Sweep, every output Simulate gives
// for the edited build.
func TestSweepMatchesSimulateReal(t *testing.T) {
	t.Parallel()
	d := benchbuilds.Open(t)
	for _, c := range benchbuilds.Builds(t, d) {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			s, err := simulator.New(d.Store)
			if err != nil {
				t.Fatal(err)
			}
			variants := nodeVariants(t, d.Store, c.Build)
			res, err := s.Sweep(c.Build, variants)
			if err != nil {
				t.Fatal(err)
			}
			for i, v := range variants {
				b := c.Build.Clone()
				if err := b.Apply(v...); err != nil {
					t.Fatal(err)
				}
				want := simulate(t, s, b)
				if res[i].Err != nil {
					t.Fatalf("variant %v: %v", v, res[i].Err)
				}
				for _, k := range want.Keys() {
					got := res[i].Changes[k].After
					if w := get(t, want, k); math.Float64bits(got) != math.Float64bits(w) {
						t.Errorf("variant %v %s: sweep %v, simulate %v", v, k, got, w)
					}
				}
			}
		})
	}
}

// nodeVariants returns a deallocation of each allocated node of b, then allocations of every
// third unallocated node next to an allocated one, by id.
func nodeVariants(t *testing.T, store *dataset.Store, b *build.Build) [][]build.Edit {
	t.Helper()
	c, _, err := build.Check(b, store)
	if err != nil {
		t.Fatal(err)
	}
	out := make([][]build.Edit, 0, len(b.Nodes))
	for _, id := range b.Nodes {
		out = append(out, []build.Edit{build.Deallocate(id)})
	}
	adjacent := map[uint32]bool{}
	for _, n := range c.Nodes() {
		for _, nb := range n.Neighbours {
			if !slices.Contains(b.Nodes, nb) {
				adjacent[nb] = true
			}
		}
	}
	for i, id := range slices.Sorted(maps.Keys(adjacent)) {
		if i%3 == 0 {
			out = append(out, []build.Edit{build.Allocate(id)})
		}
	}
	return out
}

// With Unnatural Instinct in place of each jewel but cluster jewels of the benchmark builds,
// allocating or deallocating each small passive in its radius gives, from Sweep on the fast
// path, every output Simulate gives for the edited build, and changes some.
func TestSweepRadiusPassivesReal(t *testing.T) {
	t.Parallel()
	d := benchbuilds.Open(t)
	for _, c := range benchbuilds.Builds(t, d) {
		base, ok := withInstinct(c.Build)
		if !ok {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			s, err := simulator.New(d.Store)
			if err != nil {
				t.Fatal(err)
			}
			variants := radiusVariants(t, d.Store, base)
			for _, v := range variants {
				if fast, err := simulator.FastPath(s, base, v); err != nil || !fast {
					t.Fatalf("variant %v: not on the fast path (%v)", v, err)
				}
			}
			if !sweepMatchesSimulate(t, s, base, variants) {
				t.Error("no variant changes an output")
			}
		})
	}
}

// withInstinct returns b with Unnatural Instinct in place of each jewel but cluster jewels,
// and whether it replaced any.
func withInstinct(b *build.Build) (*build.Build, bool) {
	jewel := build.Item{Rarity: build.Unique, Name: "Unnatural Instinct", Base: "Metadata/Items/Jewels/JewelDex", Mods: []build.ModLine{{
		Kind: build.Explicit, Text: "instinct", Stats: []build.StatValue{
			{Stat: "local_jewel_allocated_non_notable_passives_in_radius_grant_nothing", Value: 1},
			{Stat: "local_jewel_copy_stats_from_unallocated_non_notable_passives_in_radius", Value: 1},
		},
	}}}
	out := b.Clone()
	replaced := false
	for socket, it := range out.Jewels {
		if !strings.HasPrefix(it.Base, "Metadata/Items/Jewels/JewelPassiveTreeExpansion") {
			out.Jewels[socket], replaced = jewel, true
		}
	}
	return out, replaced
}

// radiusVariants returns a deallocation of each allocated small passive of b's radius passives
// and an allocation of each other.
func radiusVariants(t *testing.T, store *dataset.Store, b *build.Build) [][]build.Edit {
	t.Helper()
	c, _, err := build.Check(b, store)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]build.Edit
	for _, p := range c.RadiusPassives() {
		if slices.Contains(b.Nodes, p.Node.ID) {
			out = append(out, []build.Edit{build.Deallocate(p.Node.ID)})
		} else {
			out = append(out, []build.Edit{build.Allocate(p.Node.ID)})
		}
	}
	if len(out) == 0 {
		t.Fatal("no small passive in an Unnatural Instinct radius")
	}
	return out
}

// sweepMatchesSimulate checks that Sweep of base gives each variant every output Simulate gives
// for the edited build, and reports whether any variant changes one.
func sweepMatchesSimulate(t *testing.T, s *simulator.Sim, base *build.Build, variants [][]build.Edit) bool {
	t.Helper()
	res, err := s.Sweep(base, variants)
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for i, v := range variants {
		b := base.Clone()
		if err := b.Apply(v...); err != nil {
			t.Fatal(err)
		}
		want := simulate(t, s, b)
		if res[i].Err != nil {
			t.Fatalf("variant %v: %v", v, res[i].Err)
		}
		for _, k := range want.Keys() {
			ch := res[i].Changes[k]
			if w := get(t, want, k); math.Float64bits(ch.After) != math.Float64bits(w) {
				t.Errorf("variant %v %s: sweep %v, simulate %v", v, k, ch.After, w)
			}
			changed = changed || ch.After != ch.Before
		}
	}
	return changed
}
