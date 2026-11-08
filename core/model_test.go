package core_test

import (
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

func TestModelBuilderErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		add  func(b *core.ModelBuilder) error
		want error
	}{
		{"duplicate stat", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "x"})
			return err
		}, core.ErrDuplicateName},
		{"unknown include", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Includes: []core.StatID{7}})
			return err
		}, core.ErrBadHandle},
		{"places", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Places: 16})
			return err
		}, core.ErrOutOfRange},
		{"rounding", func(b *core.ModelBuilder) error {
			_, err := b.AddStat(core.StatDef{Name: "y", Round: 9})
			return err
		}, core.ErrOutOfRange},
		{"formula of unknown stat", func(b *core.ModelBuilder) error {
			return b.SetFormula(7, func(core.Reader, core.Agg) float64 { return 0 })
		}, core.ErrBadHandle},
		{"duplicate dimension", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("d", "b")
			return err
		}, core.ErrDuplicateName},
		{"duplicate dimension value", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("e", "a", "a")
			return err
		}, core.ErrDuplicateName},
		{"dimension value name taken", func(b *core.ModelBuilder) error {
			if _, err := b.AddDimension("p.q", "r"); err != nil {
				return err
			}
			_, err := b.AddDimension("p", "q.r")
			return err
		}, core.ErrDuplicateName},
		{"too many bits", func(b *core.ModelBuilder) error {
			_, err := b.AddDimension("big", make([]string, core.MaxBits)...)
			return err
		}, core.ErrOutOfRange},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var b core.ModelBuilder
			b.MustAddStat(core.StatDef{Name: "x"})
			if _, err := b.AddDimension("d", "a"); err != nil {
				t.Fatal(err)
			}
			if err := testCase.add(&b); !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestBuildCopies(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	m := b.Build()
	b.MustAddStat(core.StatDef{Name: "y"})
	setFormula(t, &b, x, func(core.Reader, core.Agg) float64 { return 9 })
	if _, err := b.AddDimension("d", "a"); err != nil {
		t.Fatal(err)
	}
	if n := m.NumStats(); n != 1 {
		t.Errorf("NumStats: got %d, want 1", n)
	}
	if _, ok := m.Stat("y"); ok {
		t.Error("Stat(y): got found, want not found")
	}
	if _, ok := m.Bit("d.a"); ok {
		t.Error("Bit(d.a): got found, want not found")
	}
	s := newSession(t, m, 1)
	add(t, s, core.Modifier{Stat: x, Value: 2})
	want(t, s, x, 2)
}
