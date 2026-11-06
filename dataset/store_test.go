package dataset_test

import (
	"maps"
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
		{name: "tree", want: "has no atlas tree", get: func() error { _, err := s.Tree("atlas"); return err }},
		{name: "text", want: "has no fr text", get: func() error { _, err := s.Text("fr"); return err }},
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
