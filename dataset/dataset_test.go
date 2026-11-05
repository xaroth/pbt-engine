package dataset_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
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
				ID: "Metadata/Characters/Str/Str", Name: "Marauder",
				BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, BaseMaxLife: 38, BaseMaxMana: 34,
				Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: -75}},
			},
			{ID: "Metadata/Characters/Dex/Dex", Name: "Ranger"},
		},
	}
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
			got, err := dataset.Read(os.DirFS(dir))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			want := *testCase.d
			want.Manifest = m
			if diff := cmp.Diff(&want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
			if m.Format != dataset.FormatVersion || len(m.Files) != 2 {
				t.Errorf("manifest: got format %d and %d files, want %d and 2", m.Format, len(m.Files), dataset.FormatVersion)
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
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := dataset.Read(testCase.fsys); err == nil {
				t.Errorf("Read: got nil error, want an error")
			}
		})
	}
}
