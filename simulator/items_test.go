package simulator_test

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// helmetText is the rare helmet without the armour value of its text, so the armour resolves
// from the base.
func helmetText() string { return strings.Replace(rawItemText, "Armour: 767\n", "", 1) }

func realSim(tb testing.TB) *simulator.Sim {
	tb.Helper()
	s, err := simulator.New(realdata.Open(tb, realdata.Current))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func equipHelmet(tb testing.TB, s *simulator.Sim, b *build.Build) *build.Build {
	tb.Helper()
	it, _, err := s.Data().Item(helmetText())
	if err != nil {
		tb.Fatal(err)
	}
	b = b.Clone()
	if err := b.Apply(build.Equip(build.SlotHelmet, it)); err != nil {
		tb.Fatal(err)
	}
	return b
}

// The helmet on a level 1 Marauder without passives. Conqueror's Helmet rolls 501 to 576
// armour; without a value in the text the highest is taken: (576 + 68 flat) * (1 + 82%
// increased) * (1 + 20% quality) = 1406.5, 1406. The character has no armour of its own and no
// increases to life or fire resistance, so the helmet adds 1406 armour, 101 life and 33% fire
// resistance, each credited to the helmet in full.
func TestHelmetOnSheet(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	bare := marauderBuild(1)
	with := equipHelmet(t, s, bare)
	before, st := simulate(t, s, bare), simulate(t, s, with)
	if !slices.Contains(codes(st), simulator.ItemDefenceAssumed) {
		t.Errorf("warnings %v, want the armour roll assumed", st.Warnings())
	}
	want := []struct {
		key    output.Key
		before float64
		item   float64
	}{
		{key: poe1.Armour, before: 0, item: 1406},
		// 38 + 12 per level + 32 strength / 2, as the character's own
		{key: poe1.Life, before: 66, item: 101},
		// the default campaign stage's -60% penalty
		{key: poe1.FireResistance, before: -60, item: 33},
	}
	for _, w := range want {
		if got := get(t, before, w.key); got != w.before {
			t.Errorf("%s without the helmet %v, want %v", w.key, got, w.before)
		}
		if got := get(t, st, w.key); got != w.before+w.item {
			t.Errorf("%s with the helmet %v, want %v", w.key, got, w.before+w.item)
		}
		ex, err := st.Explain(w.key, simulator.Proportional)
		if err != nil {
			t.Fatal(err)
		}
		share := math.NaN()
		for _, l := range ex.Lines {
			if l.Source.Bundle != nil && l.Source.Bundle.Source == (simulator.ItemSource{Slot: build.SlotHelmet, Name: "Foe Horn"}) {
				share = l.Value
			}
		}
		if share != w.item {
			t.Errorf("%s: helmet's share %v, want %v (lines %+v)", w.key, share, w.item, ex.Lines)
		}
	}
}

// What-ifs with the helmet on: re-equipping and tree edits switch sources on one sheet and give
// what fresh simulations give.
func TestItemWhatIfs(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	base := chieftainWithHelmet(t, s)
	keys := []output.Key{poe1.Life, poe1.Armour, poe1.FireResistance}
	fresh := func(b *build.Build) *simulator.Stats {
		sim := realSim(t)
		return simulate(t, sim, b, simulator.Outputs(keys...))
	}

	it := base.Items[build.SlotHelmet]
	it.Armour = 767
	given := withItem(t, base.Clone(), build.SlotHelmet, it)
	unequipped := base.Clone()
	if err := unequipped.Apply(build.Unequip(build.SlotHelmet)); err != nil {
		t.Fatal(err)
	}
	for _, other := range []*build.Build{given, unequipped, base} {
		d, err := s.Compare(base, other, simulator.Outputs(keys...))
		if err != nil {
			t.Fatal(err)
		}
		want := fresh(other)
		for _, k := range keys {
			if got, w := d.Get(k).After, get(t, want, k); got != w {
				t.Errorf("%s after re-equipping: %v, fresh %v", k, got, w)
			}
		}
	}

	variants := make([][]build.Edit, 0, 20)
	for _, n := range base.Nodes[:20] {
		variants = append(variants, []build.Edit{build.Deallocate(n)})
	}
	results, err := s.Sweep(base, variants, simulator.Outputs(keys...), simulator.Workers(1))
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		b := base.Clone()
		if err := b.Apply(variants[i]...); err != nil {
			t.Fatal(err)
		}
		want := fresh(b)
		for _, k := range keys {
			if got, w := r.Changes[k].After, get(t, want, k); got != w {
				t.Errorf("variant %d %s: %v, fresh %v", i, k, got, w)
			}
		}
	}
}

func withItem(t *testing.T, b *build.Build, s build.Slot, it build.Item) *build.Build {
	t.Helper()
	if err := b.Apply(build.Equip(s, it)); err != nil {
		t.Fatal(err)
	}
	return b
}

// chieftainWithHelmet is the level 90 Chieftain with the helmet.
func chieftainWithHelmet(tb testing.TB, s *simulator.Sim) *build.Build {
	tb.Helper()
	reg, err := dataset.OpenRoot(os.DirFS(realdata.Root(tb)))
	if reg == nil {
		tb.Fatal(err)
	}
	b, _, err := pob.FromXML(chieftainXML(tb), reg)
	if err != nil {
		tb.Fatal(err)
	}
	return equipHelmet(tb, s, b)
}
