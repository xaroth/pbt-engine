package items_test

import (
	"math"
	"os"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

func timelessStats() []string {
	return []string{
		"local_unique_jewel_alternate_tree_version", "local_unique_jewel_alternate_tree_seed",
		"local_unique_jewel_alternate_tree_keystone", "local_unique_jewel_alternate_tree_internal_revision",
	}
}

// withTimelessJewels gives d the Vaal, Templar and Eternal timeless versions with their
// conquerors' keystones, the mods of Glorious Vanity, Militant Faith and Elegant Hubris, and
// their description as 3.30 writes it, naming faction and conqueror through handlers. With
// names, d's text has the names of the factions, jewels and conquerors.
func withTimelessJewels(d *dataset.Dataset, names bool) {
	d.Trees = []dataset.Tree{{Variant: build.DefaultVariant, TimelessVersions: []dataset.TimelessVersion{
		{ID: "None"},
		{ID: "Vaal", Replacements: keystones("vaal", 1, 2, 2, 3)},
		{ID: "Karui"}, {ID: "Maraketh"},
		{ID: "Templar", Replacements: keystones("templar", 1, 1)},
		{ID: "Eternal", Replacements: keystones("eternal", 1)},
	}}}
	first := uint32(len(d.Stats))
	for _, st := range timelessStats() {
		d.Stats = append(d.Stats, dataset.Stat{ID: st})
	}
	for _, m := range []struct {
		id       string
		version  int32
		min, max int32
	}{{"VaalTimeless", 1, 100, 8000}, {"TemplarTimeless", 4, 2000, 10000}, {"EternalTimeless", 5, 2000, 8000}} {
		stat := func(i uint32, lo, hi int32) dataset.ModStat {
			return dataset.ModStat{Stat: first + i, Min: lo, Max: hi, RuthlessMin: lo, RuthlessMax: hi}
		}
		d.Mods = append(d.Mods, dataset.Mod{
			ID: m.id, Domain: 10, Generation: 3, Type: m.id, Families: []string{m.id},
			Stats: []dataset.ModStat{stat(0, m.version, m.version), stat(1, m.min, m.max), stat(2, 1, 3), stat(3, 0, 1)},
		})
	}
	en := &d.Texts[0]
	en.Descriptions[0].Descriptions = append(en.Descriptions[0].Descriptions, timelessDescription())
	if !names {
		return
	}
	en.TimelessVersionNames = []dataset.Name{{ID: "Vaal", Name: "Vaal"}, {ID: "Templar", Name: "Templars"}, {ID: "Eternal", Name: "Eternal Empire"}}
	en.TimelessJewelNames = []dataset.Name{{ID: "Vaal", Name: "Glorious Vanity"}, {ID: "Templar", Name: "Militant Faith"}, {ID: "Eternal", Name: "Elegant Hubris"}}
	en.ConquerorNames = []dataset.Name{
		{ID: "vaal_keystone_1", Name: "Xibaqua"}, {ID: "vaal_keystone_2", Name: "Zerphi"}, {ID: "vaal_keystone_2_v2", Name: "Ahuana"},
		{ID: "vaal_keystone_3", Name: "Doryani"}, {ID: "templar_keystone_1", Name: "High Templar Venarius"},
		{ID: "templar_keystone_1_v2", Name: "High Templar Maxarius"}, {ID: "eternal_keystone_1", Name: "Cadiro"},
	}
}

// keystones returns the conquerors' keystones of a version, one per index, named as the game
// names them: a repeated index is the conqueror's second revision.
func keystones(version string, indices ...uint32) []dataset.TimelessReplacement {
	out := make([]dataset.TimelessReplacement, 0, len(indices))
	for i, index := range indices {
		r := dataset.TimelessReplacement{
			ID: version + "_keystone_" + string(rune('0'+index)), Kinds: []dataset.TimelessKind{dataset.TimelessKeystone},
			Conqueror: dataset.TimelessConqueror{Index: index},
		}
		if i > 0 && indices[i-1] == index {
			r.ID += "_v2"
			r.Conqueror.Version = 1
		}
		out = append(out, r)
	}
	return out
}

// timelessDescription is the description of a timeless jewel's mod as 3.30 writes it, for the
// Vaal, Templar and Eternal versions.
func timelessDescription() statdesc.Description {
	anyValue := statdesc.Limit{Min: math.MinInt32, Max: math.MaxInt32}
	variant := func(version int32, text string, handlers ...statdesc.Handler) statdesc.Variant {
		return statdesc.Variant{
			Limits: []statdesc.Limit{{Min: version, Max: version}, anyValue, anyValue, anyValue},
			Text:   text + "\\nPassives in radius are Conquered by the {0}",
			Handlers: append([]statdesc.Handler{
				{Name: "alternate_tree_version_name"}, {Name: "alternate_tree_version_keystone", Stat: 2, Args: []int{0, 3}},
			}, handlers...),
		}
	}
	return statdesc.Description{Stats: timelessStats(), Variants: []statdesc.Variant{
		variant(1, "Bathed in the blood of {1} sacrificed in the name of {2}"),
		variant(4, "Carved to glorify {1} new faithful converted by {2}"),
		variant(5, "Commissioned {1} coins to commemorate {2}", statdesc.Handler{Name: "times_twenty", Stat: 1}),
	}}
}

func timelessReader(t *testing.T, names bool) *items.Reader {
	t.Helper()
	d := testDataset()
	withTimelessJewels(d, names)
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func vaalText(seed, conqueror string) string {
	return "Bathed in the blood of " + seed + " sacrificed in the name of " + conqueror + "\nPassives in radius are Conquered by the Vaal"
}

// The faction and conqueror a 3.30 timeless jewel's text names read to its version, keystone and
// revision, with the names the dataset gives them; the line keeps its text. A revision of 0 is
// left out, as a line leaves out every stat of 0.
func TestParseTimelessJewel(t *testing.T) {
	t.Parallel()
	r := timelessReader(t, true)
	testCases := []struct {
		name, text string
		values     []int32
	}{
		{name: "Glorious Vanity", text: vaalText("2000", "Doryani"), values: []int32{1, 2000, 3, 0}},
		{name: "a conqueror of the first revision", text: vaalText("2000", "Zerphi"), values: []int32{1, 2000, 2, 0}},
		{name: "a conqueror of a later revision", text: vaalText("8000", "Ahuana"), values: []int32{1, 8000, 2, 1}},
		{
			name: "Militant Faith", values: []int32{4, 5000, 1, 1},
			text: "Carved to glorify 5000 new faithful converted by High Templar Maxarius\nPassives in radius are Conquered by the Templars",
		},
		{
			name: "Elegant Hubris", values: []int32{5, 4000, 1, 0},
			text: "Commissioned 80000 coins to commemorate Cadiro\nPassives in radius are Conquered by the Eternal Empire",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			it, warnings, err := r.Parse("Rarity: UNIQUE\nX\nIron Jewel\nImplicits: 0\n" + testCase.text + "\n")
			if err != nil {
				t.Fatal(err)
			}
			want := build.ModLine{Kind: build.Explicit, Text: testCase.text}
			for i, st := range timelessStats() {
				if v := testCase.values[i]; v != 0 {
					want.Stats = append(want.Stats, build.StatValue{Stat: st, Value: v})
				}
			}
			if diff := gocmp.Diff([]build.ModLine{want}, it.Mods); diff != "" {
				t.Errorf("mods (-want +got):\n%s", diff)
			}
			if len(warnings) > 0 {
				t.Errorf("warnings: %v", warnings)
			}
		})
	}
}

// Without the names, a dataset's timeless jewel lines read to no stats, as before it had them.
func TestParseTimelessJewelWithoutNames(t *testing.T) {
	t.Parallel()
	r := timelessReader(t, false)
	it, warnings, err := r.Parse("Rarity: UNIQUE\nX\nIron Jewel\nImplicits: 0\n" + vaalText("2000", "Doryani") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(it.Mods) == 0 || slices.ContainsFunc(it.Mods, func(m build.ModLine) bool { return len(m.Stats) > 0 }) {
		t.Errorf("mods: got %+v, want lines without stats", it.Mods)
	}
	if !slices.ContainsFunc(warnings, func(w build.Warning) bool { return w.Code == items.LineUnread }) {
		t.Errorf("warnings: got %v, want %s", warnings, items.LineUnread)
	}
}
