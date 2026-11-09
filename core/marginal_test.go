package core_test

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

func TestMarginal(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	str := b.MustAddStat(core.StatDef{Name: "str"})
	life := b.MustAddStat(core.StatDef{Name: "life", HasMax: true, Max: 100})
	s := newSession(t, b.Build(), 1)
	addAll(t, s, []core.Modifier{
		{Stat: str, Op: core.OpBase, Value: 20, Source: 0},
		{Stat: str, Op: core.OpBase, Value: 0, Source: 4},
		{Stat: life, Op: core.OpBase, Value: 1, Source: 1, Scales: []core.Scale{{Ref: core.SelfRef(str), Step: 2}}},
		{Stat: life, Op: core.OpBase, Value: 95, Source: 2},
		{Stat: life, Op: core.OpInc, Value: 0, Source: 3},
	})
	got, err := s.Marginal(core.Node{Stat: life})
	if err != nil {
		t.Fatal(err)
	}
	// Total 105 capped at 100: strength's source loses 10 total, 5 value; the scale's too.
	want := []core.Margin{{Source: 0, Total: 10, Value: 5}, {Source: 1, Total: 10, Value: 5}, {Source: 2, Total: 95, Value: 90}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("margin %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if _, err := s.Marginal(core.Node{Stat: 9}); !errors.Is(err, core.ErrBadHandle) {
		t.Errorf("bad node: got %v", err)
	}
}

// TestMarginalEqualsRecompute checks on random sessions that each source's margin equals the
// difference to a fresh session with that source off, that sources not listed change nothing,
// and that the session is unchanged after.
func TestMarginalEqualsRecompute(t *testing.T) {
	t.Parallel()
	for seed := range uint64(20) {
		r := rand.New(rand.NewPCG(seed, 13))
		w := newRandWorld(t, r)
		s := w.session(t)
		for range 40 {
			add(t, s, w.mod(r))
		}
		before := snapshot(t, s, false)
		for range 6 {
			n := core.Node{Actor: core.ActorID(r.IntN(w.nActor)), Stat: core.StatID(r.IntN(w.nStat)), Ctx: core.CtxID(r.IntN(len(w.ctxs)))}
			checkMargins(t, w, s, n)
		}
		equalBits(t, int(seed), snapshot(t, s, true), before)
	}
}

func checkMargins(tb testing.TB, w *randWorld, s *core.Session, n core.Node) {
	tb.Helper()
	ms, err := s.Marginal(n)
	if err != nil {
		tb.Fatal(err)
	}
	listed := map[core.SourceID]core.Margin{}
	for _, m := range ms {
		listed[m.Source] = m
	}
	value := get(tb, s, n.Actor, n.Stat, n.Ctx)
	for src := range core.SourceID(nSources) {
		f := fresh(tb, w, s)
		must(tb, f.SetSourceEnabled(src, false))
		lost := value - get(tb, f, n.Actor, n.Stat, n.Ctx)
		if m := listed[src]; m.Value != lost {
			tb.Fatalf("%+v source %d: margin %v, recompute %v", n, src, m.Value, lost)
		}
	}
}
