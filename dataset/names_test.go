package dataset_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

func TestNames(t *testing.T) {
	t.Parallel()
	en := sampleTexts()[0]
	full := written(t, sample())
	testCases := []struct {
		name string
		fsys fstest.MapFS
	}{
		{name: "from the names file alone", fsys: without(full, dataset.TextFile("en"), dataset.StatsFile, dataset.CharactersFile)},
		{name: "from the text without a names file", fsys: unlisted(t, full, dataset.NamesFile("en"))},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got, err := s.Names("en")
			if err != nil {
				t.Fatalf("Names: %v", err)
			}
			if diff := gocmp.Diff(en.Names(), *got); diff != "" {
				t.Errorf("names (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNamesReadErrors(t *testing.T) {
	t.Parallel()
	full := written(t, sample())
	testCases := []struct {
		name     string
		fsys     fstest.MapFS
		language string
		want     string
		is       error
	}{
		{name: "no such language", fsys: full, language: "fr", want: "has no fr names", is: dataset.ErrNotInDataset},
		{
			name: "file of another language", language: "en", want: `names-en.fb: holds names language "zh-hant"`, is: dataset.ErrInvalid,
			fsys: rewritten(t, full, dataset.NamesFile("en"), full[dataset.NamesFile("zh-hant")].Data),
		},
		{
			name: "not a names file", language: "en", want: "wrong file identifier", is: dataset.ErrInvalid,
			fsys: rewritten(t, full, dataset.NamesFile("en"), full[dataset.TextFile("en")].Data),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = s.Names(testCase.language)
			if err == nil || !strings.Contains(err.Error(), testCase.want) || !errors.Is(err, testCase.is) {
				t.Errorf("Names: got %v, want %q wrapping %v", err, testCase.want, testCase.is)
			}
		})
	}
}

// TestRealNames writes the current pinned dataset again and reads its names from the names file
// it gains: they match its text and the file stays small enough to fetch for a class list.
func TestRealNames(t *testing.T) {
	t.Parallel()
	src := realdata.Open(t, realdata.Current)
	text, err := src.Text("en")
	if err != nil {
		t.Fatal(err)
	}
	d, err := readAll(os.DirFS(realDir(t, realdata.Current)))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dir, dataset.NamesFile("en")))
	if err != nil {
		t.Fatal(err)
	}
	const limit = 200_000
	if fi.Size() >= limit {
		t.Errorf("names-en.fb: got %d bytes, want under %d", fi.Size(), limit)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Names("en")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(got.NodeNames) == 0 || len(got.CharacterNames) == 0 || len(got.AscendancyNames) == 0 {
		t.Errorf("names: got %d nodes, %d characters, %d ascendancies, want some of each",
			len(got.NodeNames), len(got.CharacterNames), len(got.AscendancyNames))
	}
	if diff := gocmp.Diff(text.Names(), *got); diff != "" {
		t.Errorf("names (-text +names file):\n%s", diff)
	}
}

// without returns the files of fsys less names, keeping the manifest as it is.
func without(fsys fstest.MapFS, names ...string) fstest.MapFS {
	out := maps.Clone(fsys)
	for _, n := range names {
		delete(out, n)
	}
	return out
}

// unlisted returns the files of fsys without name, and the manifest without it.
func unlisted(tb testing.TB, fsys fstest.MapFS, name string) fstest.MapFS {
	tb.Helper()
	m, err := dataset.ReadManifest(fsys)
	if err != nil {
		tb.Fatal(err)
	}
	m.Files = slices.DeleteFunc(m.Files, func(f dataset.File) bool { return f.Name == name })
	return withManifest(tb, without(fsys, name), m)
}

// withManifest returns the files of fsys with m, its id updated, as the manifest.
func withManifest(tb testing.TB, fsys fstest.MapFS, m dataset.Manifest) fstest.MapFS {
	tb.Helper()
	m.ID = dataset.ContentID(m)
	b, err := json.Marshal(m)
	if err != nil {
		tb.Fatal(err)
	}
	out := maps.Clone(fsys)
	out[dataset.ManifestName] = &fstest.MapFile{Data: b}
	return out
}
