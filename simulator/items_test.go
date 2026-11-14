package simulator_test

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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

// shieldText is a rare tower shield.
const shieldText = `Rarity: RARE
Rune Guard
Colossal Tower Shield
Item Level: 85
Implicits: 0
+80 to maximum Life
+30% to Cold Resistance
`

// A shield in the swap off hand is not in use: the sheet is the one without it. In the off hand
// it counts.
func TestSwapItemLeftOut(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	shield, warnings, err := s.Data().Item(shieldText)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the shield: %v, %v", warnings, err)
	}
	bare := marauderBuild(1)
	swapped, held := bare.Clone(), bare.Clone()
	if err := swapped.Apply(build.Equip(build.SlotOffhandSwap, shield)); err != nil {
		t.Fatal(err)
	}
	if err := held.Apply(build.Equip(build.SlotOffhand, shield)); err != nil {
		t.Fatal(err)
	}
	before := simulate(t, s, bare)
	for _, k := range []output.Key{poe1.Life, poe1.Armour, poe1.ColdResistance} {
		if got, want := get(t, simulate(t, s, swapped), k), get(t, before, k); got != want {
			t.Errorf("%s with the shield in the swap off hand %v, without %v", k, got, want)
		}
	}
	if got, want := get(t, simulate(t, s, held), poe1.Life), get(t, before, poe1.Life)+80; got != want {
		t.Errorf("life with the shield in the off hand %v, want %v", got, want)
	}
}

const ringText = `Rarity: RARE
Rune Loop
Two-Stone Ring
Item Level: 84
Implicits: 1
+16% to Fire and Lightning Resistances
+30 to Strength
+20 to Dexterity and Intelligence
+1 Prefix Modifier allowed
`

// A ring's attributes and two-element resistances reach the sheet, and its crafting-only line
// is shown without being left out.
func TestRingStatsOnSheet(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	bare := marauderBuild(1)
	it, ws, err := s.Data().Item(ringText)
	if err != nil || len(ws) != 0 {
		t.Fatalf("read the ring: %v, %v", ws, err)
	}
	with := bare.Clone()
	if err := with.Apply(build.Equip(build.SlotRing1, it)); err != nil {
		t.Fatal(err)
	}
	before, st := simulate(t, s, bare), simulate(t, s, with)
	for _, w := range st.Warnings() {
		if w.Code == simulator.UnmodelledStat && strings.Contains(w.Subject, "from item") {
			t.Errorf("%s: %s, want no ring stat left out", w.Subject, w.Message)
		}
	}
	for _, w := range []struct {
		key  output.Key
		item float64
	}{
		{poe1.Strength, 30}, {poe1.Dexterity, 20}, {poe1.Intelligence, 20},
		{poe1.FireResistance, 16}, {poe1.LightningResistance, 16}, {poe1.ColdResistance, 0},
	} {
		if got, want := get(t, st, w.key), get(t, before, w.key)+w.item; got != want {
			t.Errorf("%s with the ring %v, want %v", w.key, got, want)
		}
	}
}

// A warning about an item's stat that does not count names the item's slot and the stat as
// fields, beside its subject.
func TestUncountedItemStatWarning(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	it, _, err := s.Data().Item(ringText)
	if err != nil {
		t.Fatal(err)
	}
	const stat = "evasion_+%_if_hit_recently"
	it.Mods = append(it.Mods, build.ModLine{
		Kind: build.Explicit, Text: "10% increased Evasion Rating if you've been Hit Recently",
		Stats: []build.StatValue{{Stat: stat, Value: 10}},
	})
	b := marauderBuild(1)
	if err := b.Apply(build.Equip(build.SlotRing1, it)); err != nil {
		t.Fatal(err)
	}
	want := simulator.Warning{
		Code: simulator.UnmodelledStat, Subject: stat + " from item ring-1: Rune Loop",
		Message: "not applied: condition: hit recently, left out", Slot: build.SlotRing1, Stat: stat,
	}
	ws := simulate(t, s, b).Warnings()
	i := slices.IndexFunc(ws, func(w simulator.Warning) bool { return w.Code == want.Code && w.Stat == stat })
	if i < 0 {
		t.Fatalf("warnings %v, want %v", ws, want)
	}
	if diff := cmp.Diff(want, ws[i]); diff != "" {
		t.Errorf("warning (-want +got):\n%s", diff)
	}
}

// A ring in the third ring slot reaches the sheet when the build has The Unseen Hand of the
// King in the Mists bloodline, which opens the slot; without it the ring is left out with a
// warning.
func TestThirdRingOnSheet(t *testing.T) {
	t.Parallel()
	const start, beforeHand, unseenHand = 9920, 12354, 43902
	s := realSim(t)
	it, _, err := s.Data().Item(ringText)
	if err != nil {
		t.Fatal(err)
	}
	king := func(nodes ...uint32) *build.Build {
		b := marauderBuild(90, nodes...)
		b.Bloodline = "KingInTheMists"
		return b
	}
	for _, testCase := range []struct {
		name  string
		nodes []uint32
		item  float64
		left  bool
	}{
		{"with The Unseen Hand", []uint32{start, beforeHand, unseenHand}, 30, false},
		{"without", []uint32{start, beforeHand}, 0, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			bare := king(testCase.nodes...)
			with := bare.Clone()
			if err := with.Apply(build.Equip(build.SlotRing3, it)); err != nil {
				t.Fatal(err)
			}
			st := simulate(t, s, with)
			if got := slices.Contains(codes(st), build.ItemSlotClosed); got != testCase.left {
				t.Errorf("warnings %v, want the ring left out %v", st.Warnings(), testCase.left)
			}
			if got, want := get(t, st, poe1.Strength), get(t, simulate(t, s, bare), poe1.Strength)+testCase.item; got != want {
				t.Errorf("strength with the ring %v, want %v", got, want)
			}
		})
	}
}

// jewelSocket is a jewel socket of the 3.29 tree.
const jewelSocket = 26725

func withJewel(tb testing.TB, s *simulator.Sim, b *build.Build, node uint32) *build.Build {
	tb.Helper()
	it, _, err := s.Data().Item(
		"Rarity: Rare\nGloom Spark\nCrimson Jewel\n--------\nItem Level: 84\n--------\n6% increased maximum Life\n+10% to Fire Resistance\n",
	)
	if err != nil {
		tb.Fatal(err)
	}
	b = b.Clone()
	if err := b.Apply(build.SocketJewel(node, it)); err != nil {
		tb.Fatal(err)
	}
	return b
}

// A rare jewel in an allocated socket counts on a level 1 Marauder: 66 life 6% increased is
// 69.96, 70, and 10% fire resistance over the -60% penalty, credited to the jewel. In a socket
// not allocated it gives nothing.
func TestJewelOnSheet(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	allocated := withJewel(t, s, marauderBuild(1, jewelSocket), jewelSocket)
	st := simulate(t, s, allocated)
	if got := get(t, st, poe1.Life); got != 70 {
		t.Errorf("life %v, want 70", got)
	}
	if got := get(t, st, poe1.FireResistance); got != -50 {
		t.Errorf("fire resistance %v, want -50", got)
	}
	ex, err := st.Explain(poe1.FireResistance, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(ex.Lines, func(l simulator.Line) bool {
		return l.Source.Bundle != nil && l.Source.Bundle.Source == (simulator.JewelSource{Node: jewelSocket, Name: "Gloom Spark"}) && l.Value == 10
	}) {
		t.Errorf("fire resistance lines %+v, want 10 from the jewel", ex.Lines)
	}

	bare := simulate(t, s, marauderBuild(1))
	unallocated := simulate(t, s, withJewel(t, s, marauderBuild(1), jewelSocket))
	for _, k := range []output.Key{poe1.Life, poe1.FireResistance} {
		if got, want := get(t, unallocated, k), get(t, bare, k); got != want {
			t.Errorf("%s with the jewel in a socket not allocated %v, want %v", k, got, want)
		}
	}
}
