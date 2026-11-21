package items_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

const mainScope = "Metadata/StatDescriptions/stat_descriptions.txt"

// realReader opens the reader and the stat descriptions of the current real dataset, and skips
// without $PBT_DATASETS.
func realReader(tb testing.TB) (*items.Reader, *statdesc.Set) {
	tb.Helper()
	s := realdata.Open(tb, realdata.Current)
	r, err := items.NewReader(s)
	if err != nil {
		tb.Fatal(err)
	}
	text, err := s.Text("en")
	if err != nil {
		tb.Fatal(err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		tb.Fatal(err)
	}
	return r, set
}

// The rare helmet as Path of Building writes it, its crafted line tagged.
const foeHorn = `Rarity: RARE
Foe Horn
Conqueror's Helmet
Armour: 767
Item Level: 85
Quality: 20
Sockets: R-R-R-R
Implicits: 0
+68 to Armour
+101 to maximum Life
82% increased Armour
+33% to Fire Resistance
{crafted}+28% to Physical Damage over Time Multiplier
`

func TestParseHelmet(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	it, warnings, err := r.Parse(foeHorn)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	stat := func(kind build.ModKind, text, id string, v int32) build.ModLine {
		return build.ModLine{Kind: kind, Text: text, Stats: []build.StatValue{{Stat: id, Value: v}}}
	}
	want := build.Item{
		Rarity: build.Rare, Name: "Foe Horn", Base: "Metadata/Items/Armours/Helmets/HelmetStr13", ItemLevel: 85,
		Quality: 20, Sockets: "R-R-R-R", Armour: 767,
		// 767 is below the lowest roll: (501 + 68) * 1.82 * 1.2 = 1243.
		DefenceRolls: map[build.Defence]float64{build.DefenceArmour: 0},
		Mods: []build.ModLine{
			stat(build.Explicit, "+68 to Armour", "local_base_physical_damage_reduction_rating", 68),
			stat(build.Explicit, "+101 to maximum Life", "base_maximum_life", 101),
			stat(build.Explicit, "82% increased Armour", "local_physical_damage_reduction_rating_+%", 82),
			stat(build.Explicit, "+33% to Fire Resistance", "base_fire_damage_resistance_%", 33),
			stat(build.Crafted, "+28% to Physical Damage over Time Multiplier", "physical_dot_multiplier_+", 28),
		},
	}
	if diff := cmp.Diff(want, it); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

// A local stat reads only on an item it rolls on: accuracy rolls local on weapons, attack speed
// too, and armour on armour, so on other items these lines are the global stats.
func TestParseLocalElsewhere(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	for _, c := range []struct {
		name, text, line, stat string
	}{
		{
			name: "rare helmet",
			text: "Rarity: RARE\nRapture Visor\nLion Pelt\nEvasion Rating: 900\nItem Level: 84\nQuality: 20\nImplicits: 0\n" +
				"+90 to maximum Life\n+40 to Accuracy Rating\n80% increased Evasion Rating\n+42% to Cold Resistance\n+30 to Dexterity\n",
			line: "+40 to Accuracy Rating", stat: "accuracy_rating",
		},
		{
			name: "unique helmet",
			text: "Rarity: UNIQUE\nDevoto's Devotion\nNightmare Bascinet\nImplicits: 0\n16% increased Attack Speed\n",
			line: "16% increased Attack Speed", stat: "attack_speed_+%",
		},
		{
			name: "unique sword",
			text: "Rarity: UNIQUE\nReplica Dreamfeather\nEternal Sword\nImplicits: 0\n+190 to Armour\n",
			line: "+190 to Armour", stat: "base_physical_damage_reduction_rating",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			it, warnings, err := r.Parse(c.text)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range warnings {
				if w.Code == items.LineAmbiguous {
					t.Errorf("warning: %v", w)
				}
			}
			i := slices.IndexFunc(it.Mods, func(m build.ModLine) bool { return m.Text == c.line })
			if i < 0 {
				t.Fatalf("no line %q in %v", c.line, it.Mods)
			}
			if got := it.Mods[i].Stats; len(got) != 1 || got[0].Stat != c.stat {
				t.Errorf("%q read as %v, want %s", c.line, got, c.stat)
			}
		})
	}
}

// A line naming a passive reads as the passive's node id: Impossible Escape as Path of
// Building writes it, around The Impaler (node 57257).
func TestParsePassiveName(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	it, warnings, err := r.Parse(`Rarity: UNIQUE
Impossible Escape
Viridian Jewel
Radius: Small
Limited to: 1
Implicits: 0
Passive Skills in Radius of The Impaler can be Allocated
without being connected to your tree
Passage
Corrupted
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	want := []build.ModLine{{
		Kind: build.Explicit, Text: "Passive Skills in Radius of The Impaler can be Allocated\nwithout being connected to your tree\nPassage",
		Stats: []build.StatValue{{Stat: "local_unique_jewel_disconnected_passives_can_be_allocated_around_keystone_hash", Value: 57257}},
	}}
	if diff := cmp.Diff(want, it.Mods); diff != "" {
		t.Errorf("mods (-want +got):\n%s", diff)
	}
}

// TestSampleReadings parses every item of the statdesc sample and checks the stats and values
// each line is read as: a line with one reading whose text shows every value is read as it; any
// other line as the hand-written golden file says. The golden file lists exactly those other
// lines: lines with several readings, and lines whose text does not show a value.
func TestSampleReadings(t *testing.T) {
	t.Parallel()
	r, set := realReader(t)
	want := sampleGolden(t)
	used := map[string]bool{}
	warned := map[build.Code]int{}
	for _, s := range sampleItems(t) {
		it, warnings, err := r.Parse(s.text)
		if err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		for _, w := range warnings {
			warned[w.Code]++
			if w.Code != items.RollAssumed && w.Code != items.LineUnread {
				t.Logf("%s: %s %s", s.label, w.Code, w.Message)
			}
		}
		for _, m := range it.Mods {
			if len(m.Stats) == 0 {
				continue
			}
			key := s.label + " | " + strings.ReplaceAll(m.Text, "\n", " / ")
			expect, golden := expected(t, set, m.Text)
			if golden {
				used[key] = true
				expect = want[key]
			}
			if expect == nil {
				t.Errorf("%s: no expected stats in the golden file", key)
				continue
			}
			if !slices.Equal(m.Stats, expect) {
				t.Errorf("%s: read as %v, want %v", key, m.Stats, expect)
			}
		}
	}
	for key := range want {
		if !used[key] {
			t.Errorf("golden file line %q matches no line of the sample that needs it", key)
		}
	}
	// Lines the description reader cannot read; ranges such as "(38-43)%";
	// values no mod of the line's kind the item can have rolls; and levels the text does not show
	// that the mods a line can be differ on. The sample's items were made in Path of Building,
	// which allows lines no roll of the game gives, and its lines lost their markers such as
	// {crafted}.
	wantWarned := map[build.Code]int{
		items.LineUnread: 6, items.RollAssumed: 4, items.RollOutOfRange: 14, items.ValueAssumed: 2,
	}
	if diff := cmp.Diff(wantWarned, warned); diff != "" {
		t.Errorf("warnings by code (-want +got):\n%s", diff)
	}
}

// expected returns the stats and values a line reads as when it has one reading whose text shows
// every value, or true when the golden file must say.
func expected(t *testing.T, set *statdesc.Set, text string) ([]build.StatValue, bool) {
	t.Helper()
	split := strings.Split(text, "\n")
	readings, err := set.ReadRange(mainScope, split)
	if err != nil {
		t.Fatal(err)
	}
	readings = slices.DeleteFunc(readings, func(rd statdesc.RangeReading) bool { return rd.Lines != len(split) })
	if len(readings) != 1 {
		return nil, true
	}
	rd := readings[0]
	var out []build.StatValue
	for k, st := range rd.Stats {
		if rd.Max[k] == 0 {
			continue
		}
		if hiddenIn(set, rd, k) {
			return nil, true
		}
		out = append(out, build.StatValue{Stat: st, Value: rd.Max[k]})
	}
	return out, false
}

// hiddenIn reports whether rd's text stays the same with its k-th stat at a far other value.
func hiddenIn(set *statdesc.Set, rd statdesc.RangeReading, k int) bool {
	vals := map[string]int32{}
	for i, st := range rd.Stats {
		vals[st] = rd.Max[i]
	}
	a, errA := set.Describe(mainScope, vals)
	vals[rd.Stats[k]] = rd.Max[k]*10 + 1000
	b, errB := set.Describe(mainScope, vals)
	return errA == nil && errB == nil && slices.EqualFunc(a, b, func(x, y statdesc.Line) bool { return x.Text == y.Text })
}

// sampleGolden reads the golden file: item | line | stat=value, stat=value.
func sampleGolden(t *testing.T) map[string][]build.StatValue {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sample-lines-"+realdata.Current+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]build.StatValue{}
	for l := range strings.Lines(string(data)) {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		i := strings.LastIndex(l, " | ")
		var svs []build.StatValue
		for sv := range strings.SplitSeq(l[i+3:], ", ") {
			st, v, ok := strings.Cut(sv, "=")
			n, err := strconv.Atoi(v)
			if !ok || err != nil {
				t.Fatalf("golden line %q: stat %q without a value", l, sv)
			}
			svs = append(svs, build.StatValue{Stat: st, Value: int32(n)})
		}
		out[l[:i]] = svs
	}
	return out
}

// TestSampleBases parses every item of the statdesc sample and checks the bases of one name:
// each Two-Stone Ring is the one whose implicit its implicit line reads as.
func TestSampleBases(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	want := map[string]string{
		"rare Two-Stone Ring (Fire/Cold)": "Metadata/Items/Rings/Ring14",
		"rare Two-Stone Ring":             "Metadata/Items/Rings/Ring12",
	}
	for _, s := range sampleItems(t) {
		it, _, err := r.Parse(s.text)
		if err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		if w, ok := want[s.label]; ok && it.Base != w {
			t.Errorf("%s: base %s, want %s", s.label, it.Base, w)
		}
	}
}

type sampleItem struct {
	label, text string
}

// sampleItems returns the items of the statdesc sample as Path of Building text, labelled by
// rarity and base as the sample names them.
func sampleItems(t *testing.T) []sampleItem {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "statdesc", "testdata", "item-lines.txt"))
	if err != nil {
		t.Fatal(err)
	}
	head := regexp.MustCompile(`(?m)^# (normal|magic|rare|unique) (.+?), (\d+) implicits?((?:, [^,\n]+ Item)*)$`)
	var out []sampleItem
	for p := range strings.SplitSeq(string(data), "\n\n") {
		m := head.FindStringSubmatch(p)
		if m == nil {
			t.Fatalf("paragraph without its item: %q", p)
		}
		var mods []string
		for l := range strings.Lines(p) {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				mods = append(mods, l)
			}
		}
		name := m[2]
		if m[1] == "rare" || m[1] == "unique" {
			name = "Sample\n" + name
		}
		name += strings.ReplaceAll(m[4], ", ", "\n")
		text := fmt.Sprintf("Rarity: %s\n%s\nImplicits: %s\n%s\n", strings.ToUpper(m[1]), name, m[3], strings.Join(mods, "\n"))
		out = append(out, sampleItem{label: m[1] + " " + m[2], text: text})
	}
	return out
}
