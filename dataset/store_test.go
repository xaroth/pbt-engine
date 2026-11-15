package dataset_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestStoreReadsGroupsOnUse(t *testing.T) {
	t.Parallel()
	want := sample()
	fsys := written(t, want)
	damaged := func(names ...string) fstest.MapFS {
		out := maps.Clone(fsys)
		for _, n := range names {
			out[n] = &fstest.MapFile{Data: []byte("x")}
		}
		return out
	}

	testCases := []struct {
		name string
		fsys fstest.MapFS
		get  func(*dataset.Store) (any, error)
		want any
	}{
		{
			name: "stats",
			fsys: damaged("characters.fb", "constants.fb", "tree-standard.fb", "tree-ruthless.fb", "text-en.fb", "text-zh-hant.fb"),
			get:  func(s *dataset.Store) (any, error) { return s.Stats() },
			want: want.Stats,
		},
		{
			name: "constants",
			fsys: damaged("stats.fb", "characters.fb", "tree-standard.fb", "text-en.fb"),
			get:  func(s *dataset.Store) (any, error) { return s.Constants() },
			want: want.Constants,
		},
		{
			name: "tree",
			fsys: damaged("constants.fb", "tree-ruthless.fb", "text-en.fb", "text-zh-hant.fb"),
			get:  func(s *dataset.Store) (any, error) { return s.Tree("standard") },
			want: &want.Trees[0],
		},
		{
			name: "text without trees",
			fsys: damaged("constants.fb", "tree-standard.fb", "tree-ruthless.fb", "text-zh-hant.fb"),
			get:  func(s *dataset.Store) (any, error) { return s.Text("en") },
			want: &want.Texts[0],
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got, err := testCase.get(s)
			if err != nil {
				t.Fatalf("got error %v, want none", err)
			}
			if diff := gocmp.Diff(testCase.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestStoreMissingGroup(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	testCases := []struct {
		name, want string
		get        func() error
	}{
		{name: "tree", want: "has no atlas tree", get: func() error { _, err := s.Tree("atlas"); return wrap(t, "tree", err) }},
		{name: "text", want: "has no fr text", get: func() error { _, err := s.Text("fr"); return wrap(t, "text", err) }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkError(t, testCase.get(), nil, testCase.want)
		})
	}
}

func TestStoreConcurrentUse(t *testing.T) {
	t.Parallel()
	want := sample()
	s, err := dataset.Open(written(t, want))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var wg sync.WaitGroup
	got := make([]*dataset.Tree, 8)
	for i := range got {
		wg.Go(func() {
			tr, err := s.Tree("standard")
			if err != nil {
				t.Errorf("Tree: %v", err)
			}
			got[i] = tr
		})
	}
	wg.Wait()
	for _, tr := range got[1:] {
		if tr != got[0] {
			t.Fatalf("Tree: got different results, want one shared tree")
		}
	}
	if diff := gocmp.Diff(&want.Trees[0], got[0]); diff != "" {
		t.Errorf("Tree (-want +got):\n%s", diff)
	}
}

// wrap adds context to err and keeps nil as nil.
func wrap(tb testing.TB, what string, err error) error {
	tb.Helper()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}

// With SkipHash a Store checks a file's size and not its hash; without, both.
func TestStoreSkipHash(t *testing.T) {
	t.Parallel()
	good := written(t, sample())
	m, err := dataset.ReadManifest(good)
	if err != nil {
		t.Fatal(err)
	}
	withConstants := func(edit func(*dataset.File)) fstest.MapFS {
		m := m
		m.Files = slices.Clone(m.Files)
		edit(&m.Files[slices.IndexFunc(m.Files, func(f dataset.File) bool { return f.Name == dataset.ConstantsFile })])
		return withManifest(t, good, m)
	}
	// The manifest's entry is edited, not the file, so the file still decodes.
	wrongBytes := withConstants(func(f *dataset.File) { sum := sha256.Sum256([]byte("other")); f.SHA256 = hex.EncodeToString(sum[:]) })
	wrongSize := withConstants(func(f *dataset.File) { f.Size++ })

	testCases := []struct {
		name    string
		fsys    fstest.MapFS
		opts    []dataset.Option
		wantErr bool
	}{
		{name: "wrong bytes, hashed", fsys: wrongBytes, wantErr: true},
		{name: "wrong size, hashed", fsys: wrongSize, wantErr: true},
		{name: "wrong bytes, skip hash", fsys: wrongBytes, opts: []dataset.Option{dataset.SkipHash()}},
		{name: "wrong size, skip hash", fsys: wrongSize, opts: []dataset.Option{dataset.SkipHash()}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys, testCase.opts...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Constants()
			if gotErr := err != nil; gotErr != testCase.wantErr || gotErr && !errors.Is(err, dataset.ErrInvalid) {
				t.Errorf("Constants: got error %v, want error %t", err, testCase.wantErr)
			}
		})
	}
}
