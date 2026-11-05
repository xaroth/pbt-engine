package dataset_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

func sample() *dataset.Dataset {
	return &dataset.Dataset{
		Manifest: dataset.Manifest{
			Game:    "poe1",
			Version: "3.29.3.3",
			Source:  map[string]string{"steamManifest": "1", "datSchemaCommit": "abc"},
		},
		Stats: []dataset.Stat{{ID: "level"}, {ID: "life_per_level"}, {ID: "base_maximum_all_resistances_%"}},
		Characters: []dataset.Character{
			{
				ID:           "Metadata/Characters/Str/Str",
				BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, BaseMaxLife: 38, BaseMaxMana: 34,
				Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: -75}},
			},
			{ID: "Metadata/Characters/Dex/Dex"},
		},
		Constants: []dataset.Constant{{ID: "life_per_strength", Value: 1, Per: 2}},
		Trees: []dataset.Tree{
			{
				Variant: "standard", TotalPoints: 123, AscendancyPoints: 8,
				Classes: []dataset.Class{{
					Character: "Metadata/Characters/Str/Str", BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, StartNode: 1,
					Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}},
				}},
				AlternateAscendancies: []dataset.Ascendancy{{ID: "Bloodline"}},
				Nodes: []dataset.Node{
					{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
					{
						ID: 2, Kind: dataset.Notable, Ascendancy: "Juggernaut",
						Stats:          []dataset.StatValue{{Stat: 1, Value: 10}},
						Buffs:          [][]dataset.StatValue{{{Stat: 2, Value: -5}, {Stat: 0, Value: 1}}},
						SkillPoints:    1,
						MasteryEffects: []dataset.MasteryEffect{{ID: 7, Stats: []dataset.StatValue{{Stat: 0, Value: 3}}}},
						Neighbours:     []uint32{1},
						MultipleChoice: true, MultipleChoiceOption: true,
					},
				},
				JewelSlots: []uint32{2},
			},
			{Variant: "ruthless"},
		},
		Texts: []dataset.Text{
			{
				Language: "en",
				Descriptions: []statdesc.Scope{
					{Name: "base", Descriptions: []statdesc.Description{
						{Stats: []string{"level"}},
						{Stats: []string{"life_per_level"}, Variants: []statdesc.Variant{{
							Limits:   []statdesc.Limit{{Min: math.MinInt32, Max: -1}},
							Text:     "{0} life",
							Handlers: []statdesc.Handler{{Name: "negate"}},
						}}},
					}},
					{Name: "child", Parent: "base"},
				},
				NodeNames:       []dataset.NodeName{{ID: 1, Name: "MARAUDER"}, {ID: 2, Name: "Strength"}},
				CharacterNames:  []dataset.Name{{ID: "Metadata/Characters/Str/Str", Name: "Marauder"}},
				AscendancyNames: []dataset.AscendancyName{{Variant: "standard", ID: "Juggernaut", Name: "Juggernaut"}},
			},
			{Language: "zh-hant"},
		},
	}
}

func languages(d *dataset.Dataset) []string {
	var out []string
	for _, t := range d.Texts {
		out = append(out, t.Language)
	}
	return out
}

// written writes d and returns the dataset files as a MapFS.
func written(t *testing.T, d *dataset.Dataset) fstest.MapFS {
	t.Helper()
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		fsys[e.Name()] = &fstest.MapFile{Data: b}
	}
	return fsys
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		d    *dataset.Dataset
	}{
		{name: "sample", d: sample()},
		{name: "empty", d: &dataset.Dataset{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			m, err := dataset.Write(dir, testCase.d)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := dataset.Read(os.DirFS(dir), languages(testCase.d)...)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			want := *testCase.d
			want.Manifest = m
			if diff := cmp.Diff(&want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
			if wantFiles := 3 + len(testCase.d.Trees) + len(testCase.d.Texts); m.Format != dataset.FormatVersion || len(m.Files) != wantFiles {
				t.Errorf("manifest: got format %d and %d files, want %d and %d", m.Format, len(m.Files), dataset.FormatVersion, wantFiles)
			}
		})
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	t.Parallel()
	a, b := written(t, sample()), written(t, sample())
	if len(a) != len(b) {
		t.Fatalf("got %d and %d files, want equal", len(a), len(b))
	}
	for name, f := range a {
		if !bytes.Equal(f.Data, b[name].Data) {
			t.Errorf("%s: bytes differ between two writes", name)
		}
	}
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		stats []dataset.StatValue
	}{
		{name: "unsorted", stats: []dataset.StatValue{{Stat: 1}, {Stat: 0}}},
		{name: "repeated", stats: []dataset.StatValue{{Stat: 1}, {Stat: 1}}},
		{name: "out of range", stats: []dataset.StatValue{{Stat: 3}}},
	}
	treeCases := []struct {
		name   string
		change func(d *dataset.Dataset)
	}{
		{name: "unsorted nodes", change: func(d *dataset.Dataset) {
			n := d.Trees[0].Nodes
			n[0], n[1] = n[1], n[0]
		}},
		{name: "repeated node", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].ID = 1 }},
		{name: "node stat out of range", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].Stats[0].Stat = 9 }},
		{name: "buff stat out of range", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].Buffs[0][0].Stat = 9 }},
		{name: "mastery stat out of range", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].MasteryEffects[0].Stats[0].Stat = 9 }},
		{name: "unknown neighbour", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[0].Neighbours = []uint32{5} }},
		{name: "unsorted neighbours", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[0].Neighbours = []uint32{2, 1} }},
		{name: "unknown start node", change: func(d *dataset.Dataset) { d.Trees[0].Classes[0].StartNode = 5 }},
		{name: "unknown jewel slot", change: func(d *dataset.Dataset) { d.Trees[0].JewelSlots = []uint32{5} }},
		{name: "repeated variant", change: func(d *dataset.Dataset) { d.Trees[1].Variant = "standard" }},
		{name: "empty variant", change: func(d *dataset.Dataset) { d.Trees[1].Variant = "" }},
		{name: "constant per zero", change: func(d *dataset.Dataset) { d.Constants[0].Per = 0 }},
		{name: "description parent missing", change: func(d *dataset.Dataset) { d.Texts[0].Descriptions[1].Parent = "gone" }},
		{name: "description with too many stats", change: func(d *dataset.Dataset) {
			sc := &d.Texts[0].Descriptions[0].Descriptions[0]
			sc.Stats = make([]string, 257)
			for i := range sc.Stats {
				sc.Stats[i] = strconv.Itoa(i)
			}
		}},
		{name: "variant not a tag", change: func(d *dataset.Dataset) { d.Trees[1].Variant = "../x" }},
		{name: "language not a tag", change: func(d *dataset.Dataset) { d.Texts[1].Language = "EN" }},
		{name: "repeated language", change: func(d *dataset.Dataset) { d.Texts[1].Language = "en" }},
		{name: "unknown class character", change: func(d *dataset.Dataset) { d.Trees[0].Classes[0].Character = "x" }},
		{name: "unsorted node names", change: func(d *dataset.Dataset) {
			n := d.Texts[0].NodeNames
			n[0], n[1] = n[1], n[0]
		}},
		{name: "name for unknown node", change: func(d *dataset.Dataset) { d.Texts[0].NodeNames[1].ID = 9 }},
		{name: "name for unknown character", change: func(d *dataset.Dataset) { d.Texts[0].CharacterNames[0].ID = "x" }},
		{name: "name for unknown ascendancy", change: func(d *dataset.Dataset) { d.Texts[0].AscendancyNames[0].Variant = "ruthless" }},
	}
	for _, testCase := range treeCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			testCase.change(d)
			if _, err := dataset.Write(t.TempDir(), d); err == nil {
				t.Errorf("Write: got nil error, want an error")
			}
		})
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			d.Characters[0].Stats = testCase.stats
			if _, err := dataset.Write(t.TempDir(), d); err == nil {
				t.Errorf("Write: got nil error, want an error")
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, sample())

	// edit returns a copy of good with the manifest changed by f.
	edit := func(f func(m map[string]any)) fstest.MapFS {
		out := fstest.MapFS{}
		for k, v := range good {
			out[k] = &fstest.MapFile{Data: bytes.Clone(v.Data)}
		}
		var m map[string]any
		if err := json.Unmarshal(good[dataset.ManifestName].Data, &m); err != nil {
			t.Fatal(err)
		}
		f(m)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		out[dataset.ManifestName] = &fstest.MapFile{Data: b}
		return out
	}
	files := func(m map[string]any) []any { return m["files"].([]any) }
	// replace sets file i's content to b and fixes its manifest entry.
	replace := func(i int, b []byte) fstest.MapFS {
		var name string
		fsys := edit(func(m map[string]any) {
			f := files(m)[i].(map[string]any)
			sum := sha256.Sum256(b)
			f["size"], f["sha256"] = len(b), hex.EncodeToString(sum[:])
			name = f["name"].(string)
		})
		fsys[name] = &fstest.MapFile{Data: b}
		return fsys
	}
	oneStat := &dataset.Dataset{Stats: sample().Stats[:1]}
	oneStatBytes := written(t, oneStat)["stats.fb"].Data
	charsBytes := good["characters.fb"].Data

	testCases := []struct {
		name string
		fsys fstest.MapFS
	}{
		{name: "no manifest", fsys: fstest.MapFS{}},
		{name: "manifest not json", fsys: fstest.MapFS{dataset.ManifestName: {Data: []byte("{")}}},
		{name: "unknown manifest field", fsys: edit(func(m map[string]any) { m["extra"] = 1 })},
		{name: "other format", fsys: edit(func(m map[string]any) { m["format"] = dataset.FormatVersion + 1 })},
		{name: "unknown file", fsys: edit(func(m map[string]any) { files(m)[0].(map[string]any)["name"] = "x.fb" })},
		{name: "missing group", fsys: edit(func(m map[string]any) { m["files"] = files(m)[:1] })},
		{name: "file absent", fsys: func() fstest.MapFS { f := edit(func(map[string]any) {}); delete(f, "stats.fb"); return f }()},
		{name: "hash differs", fsys: edit(func(m map[string]any) { files(m)[0].(map[string]any)["sha256"] = "00" })},
		{name: "wrong identifier", fsys: replace(0, charsBytes)},
		{name: "truncated", fsys: replace(1, charsBytes[:len(charsBytes)/2])},
		{name: "stats too few for characters", fsys: replace(0, oneStatBytes)},
		{name: "file listed twice", fsys: edit(func(m map[string]any) { m["files"] = append(files(m), files(m)[0]) })},
		{name: "truncated tree", fsys: replace(3, good["tree-standard.fb"].Data[:40])},
		{name: "truncated text", fsys: replace(5, good["text-en.fb"].Data[:40])},
		{name: "text not a text file", fsys: replace(5, good["constants.fb"].Data)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := dataset.Read(testCase.fsys, "en"); err == nil {
				t.Errorf("Read: got nil error, want an error")
			}
		})
	}
}

func TestReadLanguages(t *testing.T) {
	t.Parallel()
	fsys := written(t, sample())
	core, err := dataset.Read(fsys)
	if err != nil {
		t.Fatalf("Read without languages: %v", err)
	}
	if len(core.Texts) != 0 {
		t.Errorf("Read without languages: got %d texts, want 0", len(core.Texts))
	}
	// A text file that is not asked for is not read, so its damage goes unnoticed.
	broken := maps.Clone(fsys)
	broken["text-zh-hant.fb"] = &fstest.MapFile{Data: []byte("x")}
	got, err := dataset.Read(broken, "en")
	if err != nil {
		t.Fatalf("Read(en) with a damaged zh-hant file: %v", err)
	}
	if len(got.Texts) != 1 || got.Texts[0].Language != "en" {
		t.Errorf("Read(en): got texts %v, want only en", languages(got))
	}
	if _, err := dataset.Read(fsys, "fr"); err == nil {
		t.Errorf("Read(fr): got nil error, want an error for a missing language")
	}
}
