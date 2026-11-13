package dataset_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

func TestManifestNeeds(t *testing.T) {
	t.Parallel()
	m := dataset.Manifest{}
	for _, name := range []string{"tree-standard.fb", "quests.fb", "characters.fb", "stats.fb", "constants.fb", "text-en.fb", "layout-standard.fb"} {
		m.Files = append(m.Files, dataset.File{Name: name})
	}
	testCases := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "none", files: nil, want: nil},
		{name: "a group that reads no other", files: []string{dataset.ConstantsFile}, want: []string{"constants.fb"}},
		{name: "a tree", files: []string{dataset.TreeFile("standard")}, want: []string{"tree-standard.fb", "characters.fb", "stats.fb"}},
		{name: "a layout", files: []string{dataset.LayoutFile("standard")}, want: []string{"tree-standard.fb", "characters.fb", "stats.fb", "layout-standard.fb"}},
		{name: "a text", files: []string{dataset.TextFile("en")}, want: []string{"characters.fb", "stats.fb", "text-en.fb"}},
		{
			name:  "files the manifest lacks",
			files: []string{dataset.SkillsFile, dataset.TreeFile("ruthless"), dataset.QuestsFile},
			want:  []string{"quests.fb", "stats.fb"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if diff := gocmp.Diff(testCase.want, m.Needs(testCase.files...)); diff != "" {
				t.Errorf("Needs(%v) (-want +got):\n%s", testCase.files, diff)
			}
		})
	}
}

// TestManifestNeedsReal loads each group of the pinned datasets alone and compares the files
// the store opened with Needs.
func TestManifestNeedsReal(t *testing.T) {
	t.Parallel()
	for _, version := range realdata.Versions() {
		m := realdata.Open(t, version).Manifest()
		for _, f := range m.Files {
			t.Run(version+"/"+f.Name, func(t *testing.T) {
				t.Parallel()
				rec := realdata.Record(t, version)
				s, err := dataset.Open(rec)
				if err != nil {
					t.Fatal(err)
				}
				if err := load(s, f.Name); err != nil {
					t.Fatal(err)
				}
				want := slices.Sorted(slices.Values(m.Needs(f.Name)))
				if diff := gocmp.Diff(want, rec.Opened()); diff != "" {
					t.Errorf("files opened to load %s (-Needs +opened):\n%s", f.Name, diff)
				}
			})
		}
	}
}

var errNoLoader = errors.New("no loader")

// load loads the group in file name.
func load(s *dataset.Store, name string) error {
	discard := func(_ any, err error) error { return err }
	single := map[string]func() error{
		dataset.StatsFile:      func() error { return discard(s.Stats()) },
		dataset.CharactersFile: func() error { return discard(s.Characters()) },
		dataset.ConstantsFile:  func() error { return discard(s.Constants()) },
		dataset.QuestsFile:     func() error { return discard(s.Quests()) },
		dataset.ItemsFile:      func() error { return discard(s.Items()) },
		dataset.ModsFile:       func() error { return discard(s.Mods()) },
		dataset.SkillsFile:     func() error { return discard(s.Skills()) },
		dataset.BuffsFile:      func() error { return discard(s.Buffs()) },
		dataset.MonstersFile:   func() error { return discard(s.Monsters()) },
	}
	if f, ok := single[name]; ok {
		return f()
	}
	if variant, ok := strings.CutPrefix(name, "tree-"); ok {
		return discard(s.Tree(strings.TrimSuffix(variant, ".fb")))
	}
	if variant, ok := strings.CutPrefix(name, "layout-"); ok {
		return discard(s.Layout(strings.TrimSuffix(variant, ".fb")))
	}
	if language, ok := strings.CutPrefix(name, "text-"); ok {
		return discard(s.Text(strings.TrimSuffix(language, ".fb")))
	}
	if language, ok := strings.CutPrefix(name, "names-"); ok {
		return discard(s.Names(strings.TrimSuffix(language, ".fb")))
	}
	return fmt.Errorf("%w for %s", errNoLoader, name)
}
