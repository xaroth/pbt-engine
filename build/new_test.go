package build_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

func TestNew(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	testCases := []struct {
		name    string
		class   string
		want    *build.Build
		wantErr error
	}{
		{
			name:  "class",
			class: marauder,
			want:  &build.Build{Format: build.Format, Game: "poe1", Version: "3.29.3.3", Variant: build.DefaultVariant, Class: marauder, Level: 1},
		},
		{name: "unknown class", class: "Metadata/Characters/Nobody", wantErr: build.ErrClass},
		{name: "character not in the tree", class: "Metadata/Characters/NotInTree", wantErr: build.ErrClass},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			c, err := build.New(s, testCase.class)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("got error %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr != nil {
				return
			}
			if diff := cmp.Diff(testCase.want, new(c.Build()), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if w := c.Warnings(); len(w) > 0 {
				t.Errorf("got warnings %v, want none", w)
			}
		})
	}
}

func TestNewEveryClass(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	tree, err := s.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Classes) == 0 {
		t.Fatal("no classes in the tree")
	}
	for _, class := range tree.Classes {
		t.Run(class.Character, func(t *testing.T) {
			t.Parallel()
			c, err := build.New(s, class.Character)
			if err != nil {
				t.Fatal(err)
			}
			b := c.Build()
			if _, w, err := build.Check(&b, s); err != nil || len(w) > 0 {
				t.Errorf("Check: got warnings %v, error %v, want none", w, err)
			}
		})
	}
}
