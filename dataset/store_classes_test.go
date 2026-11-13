package dataset_test

import (
	"io/fs"
	"slices"
	"sync"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
)

// TestClassesWithNames lists a tree's classes with their names as a class list does, reading
// the tree and names files and neither the stats nor the text group.
func TestClassesWithNames(t *testing.T) {
	t.Parallel()
	want := sample()
	fsys := &recordingFS{fsys: written(t, want)}
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	classes, err := s.Classes("standard")
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	if diff := gocmp.Diff(want.Trees[0].Classes, classes); diff != "" {
		t.Errorf("Classes (-want +got):\n%s", diff)
	}
	names, err := s.Names("en")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if diff := gocmp.Diff(want.Texts[0].CharacterNames, names.CharacterNames); diff != "" {
		t.Errorf("character names (-want +got):\n%s", diff)
	}
	read := fsys.read()
	slices.Sort(read)
	wantRead := []string{dataset.ManifestName, dataset.NamesFile("en"), dataset.TreeFile("standard")}
	if diff := gocmp.Diff(wantRead, read); diff != "" {
		t.Errorf("files read (-want +got):\n%s", diff)
	}
}

func TestClassesReadErrors(t *testing.T) {
	t.Parallel()
	full := written(t, sample())
	testCases := []struct {
		name, variant, want string
		fsys                fs.FS
		is                  error
	}{
		{name: "no such variant", variant: "atlas", fsys: full, want: "has no atlas tree", is: dataset.ErrNotInDataset},
		{
			name: "file of another variant", variant: "standard", want: `tree-standard.fb: holds tree variant "ruthless"`, is: dataset.ErrInvalid,
			fsys: rewritten(t, full, dataset.TreeFile("standard"), full[dataset.TreeFile("ruthless")].Data),
		},
		{
			name: "not a tree file", variant: "standard", want: "wrong file identifier", is: dataset.ErrInvalid,
			fsys: rewritten(t, full, dataset.TreeFile("standard"), full[dataset.NamesFile("en")].Data),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = s.Classes(testCase.variant)
			checkError(t, err, testCase.is, testCase.want)
		})
	}
}

// recordingFS records the names of the files opened in it.
type recordingFS struct {
	fsys fs.FS
	mu   sync.Mutex
	seen []string
}

func (r *recordingFS) Open(name string) (fs.File, error) {
	r.mu.Lock()
	r.seen = append(r.seen, name)
	r.mu.Unlock()
	//nolint:wrapcheck // an fs.FS returns the *fs.PathError of the FS it wraps as it is
	return r.fsys.Open(name)
}

func (r *recordingFS) read() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.seen)
}
