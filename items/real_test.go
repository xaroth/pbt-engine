package items_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestSampleReadings parses every item of the statdesc sample and checks the stats each line is
// read as: a line with one reading is read as it; a line with several, as the hand-written
// golden file says. The golden file lists exactly the lines with several readings.
func TestSampleReadings(t *testing.T) {
	t.Parallel()
	r, set := realReader(t)
	want := ambiguousGolden(t)
	used := map[string]bool{}
	warned := map[build.Code]int{}
	for _, s := range sampleItems(t) {
		it, warnings, err := r.Parse(s.text)
		if err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		for _, w := range warnings {
			warned[w.Code]++
			if w.Code == items.LineAmbiguous || w.Code == items.RollOutOfRange {
				t.Logf("%s: %s", s.label, w.Message)
			}
		}
		for _, m := range it.Mods {
			if len(m.Stats) == 0 {
				continue
			}
			got := make([]string, len(m.Stats))
			for i, sv := range m.Stats {
				got[i] = sv.Stat
			}
			key := s.label + " | " + strings.ReplaceAll(m.Text, "\n", " / ")
			expect, several := expected(t, set, m.Text, want[key])
			if several {
				used[key] = true
			}
			if expect == nil {
				t.Errorf("%s: several readings and no expected stats in the golden file", key)
				continue
			}
			if !slices.Equal(got, expect) {
				t.Errorf("%s: read as %v, want %v", key, got, expect)
			}
		}
	}
	for key := range want {
		if !used[key] {
			t.Errorf("golden file line %q matches no ambiguous line of the sample", key)
		}
	}
	// Lines the description reader cannot read; lines left ambiguous; ranges such as "(38-43)%";
	// and values no mod of the line's kind the item can have rolls. The sample's items were made
	// in Path of Building, which allows lines no roll of the game gives, and its lines lost their
	// markers such as {crafted}.
	wantWarned := map[build.Code]int{items.LineUnread: 6, items.LineAmbiguous: 2, items.RollAssumed: 4, items.RollOutOfRange: 15}
	if diff := cmp.Diff(wantWarned, warned); diff != "" {
		t.Errorf("warnings by code (-want +got):\n%s", diff)
	}
}

// expected returns the stats a line should read as: its only reading's, or golden when it has
// several, and whether it has several.
func expected(t *testing.T, set *statdesc.Set, text string, golden []string) ([]string, bool) {
	t.Helper()
	split := strings.Split(text, "\n")
	readings, err := set.ReadRange(mainScope, split)
	if err != nil {
		t.Fatal(err)
	}
	readings = slices.DeleteFunc(readings, func(rd statdesc.RangeReading) bool { return rd.Lines != len(split) })
	if len(readings) == 1 {
		return shownStats(readings[0]), false
	}
	return golden, true
}

func shownStats(rd statdesc.RangeReading) []string {
	var out []string
	for i, st := range rd.Stats {
		if rd.Max[i] != 0 {
			out = append(out, st)
		}
	}
	return out
}

// ambiguousGolden reads the golden file: item | line | stats.
func ambiguousGolden(t *testing.T) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ambiguous-lines-"+realdata.Current+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for l := range strings.Lines(string(data)) {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		i := strings.LastIndex(l, " | ")
		out[l[:i]] = strings.Split(l[i+3:], ", ")
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
