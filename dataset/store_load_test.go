package dataset_test

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
)

// countingFS counts the files opened from it.
type countingFS struct {
	fs.FS

	mu     sync.Mutex
	opened map[string]int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.opened[name]++
	c.mu.Unlock()
	return c.FS.Open(name) //nolint:wrapcheck // an fs.FS returns the *fs.PathError of the FS it wraps as it is
}

func (c *countingFS) take() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.opened
	c.opened = map[string]int{}
	return out
}

// getAll calls every getter of s on every variant and on language en.
func getAll(s *dataset.Store) {
	_, _ = s.Stats()
	_, _ = s.Characters()
	_, _ = s.Constants()
	_, _ = s.Quests()
	for _, v := range []string{"standard", "ruthless"} {
		_, _ = s.Tree(v)
		_, _ = s.Classes(v)
		_, _ = s.Layout(v)
	}
	_, _ = s.Text("en")
	_, _ = s.Names("en")
	_, _ = s.Items()
	_, _ = s.Mods()
	_, _ = s.SkillIndex()
	_, _ = s.ItemSupports()
	_, _ = s.Buffs()
	_, _ = s.Monsters()
}

// After Load, no getter reads a file, and Load read each file of the language once.
func TestStoreLoad(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		d    *dataset.Dataset
	}{
		{name: "layout", d: withLayout()},
		{name: "buffs", d: withBuffs()},
		{name: "monsters", d: withMonsters()},
		{name: "item supports", d: withGrantedSkills()},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fsys := &countingFS{FS: written(t, testCase.d), opened: map[string]int{}}
			s, err := dataset.Open(fsys)
			if err != nil {
				t.Fatal(err)
			}
			fsys.take()
			var reports []dataset.Progress
			if err := s.Load("en", func(p dataset.Progress) { reports = append(reports, p) }); err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := map[string]int{}
			var total int64
			for _, f := range s.Manifest().Files {
				if f.Name != dataset.TextFile("zh-hant") && f.Name != dataset.NamesFile("zh-hant") {
					want[f.Name] = 1
					total += int64(f.Size)
				}
			}
			if diff := gocmp.Diff(want, fsys.take()); diff != "" {
				t.Errorf("Load read (-want +got):\n%s", diff)
			}
			getAll(s)
			if got := fsys.take(); len(got) != 0 {
				t.Errorf("getters after Load read %v, want nothing", got)
			}
			checkProgress(t, reports, slices.Collect(maps.Keys(want)), total)
		})
	}
}

// checkProgress checks that reports start at nothing, count up, report each file read and
// decoded once, and end with everything decoded.
func checkProgress(t *testing.T, reports []dataset.Progress, files []string, total int64) {
	t.Helper()
	if len(reports) != 1+2*len(files) {
		t.Fatalf("got %d reports, want %d", len(reports), 1+2*len(files))
	}
	if want := (dataset.Progress{Total: total}); reports[0] != want {
		t.Errorf("first report: got %+v, want %+v", reports[0], want)
	}
	seen := map[string]int{}
	for i, p := range reports[1:] {
		if prev := reports[i]; !countsUp(prev, p) {
			t.Errorf("report %d: got %+v after %+v, want counts up", i+1, p, prev)
		}
		seen[p.File]++
	}
	want := map[string]int{}
	for _, f := range files {
		want[f] = 2
	}
	if diff := gocmp.Diff(want, seen); diff != "" {
		t.Errorf("reports per file, read and decoded (-want +got):\n%s", diff)
	}
	if last := reports[len(reports)-1]; last.Read != total || last.Decoded != total {
		t.Errorf("last report: got %+v, want all %d bytes read and decoded", last, total)
	}
}

// countsUp reports whether p follows prev: the same total, and more read or decoded.
func countsUp(prev, p dataset.Progress) bool {
	return p.Total == prev.Total && p.Read >= prev.Read && p.Decoded >= prev.Decoded && p.Read+p.Decoded > prev.Read+prev.Decoded
}

// Load names the first file whose group failed, not the groups that need it.
func TestStoreLoadError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, file string
		d          *dataset.Dataset
	}{
		{name: "group", file: "constants.fb", d: sample()},
		{name: "group others need", file: "skills.fb", d: withMonsters()},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fsys := written(t, testCase.d)
			fsys[testCase.file] = &fstest.MapFile{Data: []byte("x")}
			s, err := dataset.Open(fsys)
			if err != nil {
				t.Fatal(err)
			}
			err = s.Load("en", nil)
			le, ok := errors.AsType[*dataset.LoadError](err)
			if !ok || le.File != testCase.file || !errors.Is(err, dataset.ErrInvalid) {
				t.Errorf("got %v, want a *LoadError of %s that is ErrInvalid", err, testCase.file)
			}
		})
	}
}
