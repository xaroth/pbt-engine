package poe1

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Every more modifier of the table has a PoB mod name, so the sheet can group it.
func TestTableMoreNames(t *testing.T) {
	t.Parallel()
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tb.Entries {
		for _, m := range e.Mods {
			if _, ok := moreNameOf(m.Context); m.Op == core.OpMore && !ok {
				t.Errorf("%s: no more name for %v", e.Stat, m.Context.In)
			}
		}
	}
}

func TestMoreNameOf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   [][]string
		want moreName
		ok   bool
	}{
		{nil, moreGeneric, true},
		{[][]string{{"type.Spell"}}, moreGeneric, true},
		{[][]string{{"type.Melee"}, {"damage.physical"}}, moreName(Physical), true},
		{[][]string{{"damage.lightning", "damage.fire", "damage.cold"}}, moreElemental, true},
		{[][]string{{"damage.fire", "damage.cold"}}, 0, false},
	} {
		got, ok := moreNameOf(semantics.ContextFilter{In: c.in})
		if got != c.want || ok != c.ok {
			t.Errorf("%v: got %v %v, want %v %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
