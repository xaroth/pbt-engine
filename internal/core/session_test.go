package core_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/core"
)

// nSources is the number of sources every test session starts with, so modifiers can name
// sources 0 to nSources-1 directly.
const nSources = 10

func newSession(tb testing.TB, m *core.Model, actors int) *core.Session {
	tb.Helper()
	s, err := core.NewSession(m, actors)
	if err != nil {
		tb.Fatal(err)
	}
	for range nSources {
		s.NewSource()
	}
	return s
}

func add(tb testing.TB, s *core.Session, m core.Modifier) core.ModID {
	tb.Helper()
	id, err := s.AddModifier(m)
	if err != nil {
		tb.Fatal(err)
	}
	return id
}

func addAll(tb testing.TB, s *core.Session, mods []core.Modifier) []core.ModID {
	tb.Helper()
	ids := make([]core.ModID, len(mods))
	for i, m := range mods {
		ids[i] = add(tb, s, m)
	}
	return ids
}

func get(tb testing.TB, s *core.Session, actor core.ActorID, stat core.StatID, ctx core.CtxID) float64 {
	tb.Helper()
	v, err := s.Get(core.Node{Actor: actor, Stat: stat, Ctx: ctx})
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

// want compares bits, so -0 fails against 0.
func want(tb testing.TB, s *core.Session, stat core.StatID, exp float64) {
	tb.Helper()
	if v := get(tb, s, 0, stat, core.GlobalCtx); math.Float64bits(v) != math.Float64bits(exp) {
		def, _ := s.Model().StatDef(stat)
		tb.Errorf("%s: got %v, want %v", def.Name, v, exp)
	}
}

func must(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}

func setFormula(tb testing.TB, b *core.ModelBuilder, id core.StatID, f core.Formula) {
	tb.Helper()
	must(tb, b.SetFormula(id, f))
}

func TestNewSessionErrors(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	m := b.Build()
	for _, actors := range []int{0, math.MaxUint16 + 1} {
		if _, err := core.NewSession(m, actors); !errors.Is(err, core.ErrOutOfRange) {
			t.Errorf("%d actors: got %v, want ErrOutOfRange", actors, err)
		}
	}
}

func TestInternContext(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	s := newSession(t, b.Build(), 1)
	a := s.InternContext(core.MaskOf(1, 5))
	if got := s.InternContext(core.MaskOf(5, 1)); got != a {
		t.Errorf("same mask: got %d, want %d", got, a)
	}
	if got := s.InternContext(core.Mask{}); got != core.GlobalCtx {
		t.Errorf("empty mask: got %d, want GlobalCtx", got)
	}
	if mask, ok := s.ContextMask(a); !ok || mask != core.MaskOf(1, 5) {
		t.Errorf("ContextMask: got %v, %v, want the interned mask, true", mask, ok)
	}
	if n := s.NumContexts(); n != 2 {
		t.Errorf("NumContexts: got %d, want 2", n)
	}
}

func TestBadHandles(t *testing.T) {
	t.Parallel()
	var b core.ModelBuilder
	x := b.MustAddStat(core.StatDef{Name: "x"})
	badRead := func(name string, ref core.Ref) core.StatID {
		id := b.MustAddStat(core.StatDef{Name: name})
		setFormula(t, &b, id, func(r core.Reader, _ core.Agg) float64 { return r.Read(ref) })
		return id
	}
	readActor := badRead("read_actor", core.Ref{Actor: 3, Stat: x, Ctx: core.Self})
	// Past the range of ActorID and CtxID, these would convert to actor 0 and the global context.
	readWideActor := badRead("read_wide_actor", core.Ref{Actor: 1 << 16, Stat: x, Ctx: core.Self})
	readWideCtx := badRead("read_wide_ctx", core.Ref{Actor: core.Self, Stat: x, Ctx: 1 << 32})
	m := b.Build()
	testCases := []struct {
		name string
		call func(s *core.Session) error
	}{
		{"get actor", func(s *core.Session) error { _, err := s.Get(core.Node{Actor: 1, Stat: x}); return wrap(t, "get", err) }},
		{"get stat", func(s *core.Session) error { _, err := s.Get(core.Node{Stat: 5}); return wrap(t, "get", err) }},
		{"get context", func(s *core.Session) error { _, err := s.Get(core.Node{Stat: x, Ctx: 3}); return wrap(t, "get", err) }},
		{"explain", func(s *core.Session) error { _, err := s.Explain(core.Node{Stat: 5}); return wrap(t, "explain", err) }},
		{"explain", func(s *core.Session) error { _, err := s.Explain(core.Node{Stat: 5}); return wrap(t, "explain", err) }},
		{"formula read actor", func(s *core.Session) error { _, err := s.Get(core.Node{Stat: readActor}); return wrap(t, "get", err) }},
		{"formula read actor past ActorID", func(s *core.Session) error {
			_, err := s.Get(core.Node{Stat: readWideActor})
			return wrap(t, "get", err)
		}},
		{"formula read context past CtxID", func(s *core.Session) error {
			_, err := s.Get(core.Node{Stat: readWideCtx})
			return wrap(t, "get", err)
		}},
		{"remove zero id", func(s *core.Session) error { return s.RemoveModifier(core.ModID{}) }},
		{"value zero id", func(s *core.Session) error { return s.SetModifierValue(core.ModID{}, 1) }},
		{"modifier zero id", func(s *core.Session) error { _, _, err := s.Modifier(core.ModID{}); return wrap(t, "modifier", err) }},
		{"source", func(s *core.Session) error { return s.SetSourceEnabled(nSources, false) }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: x, Value: 2})
			if err := testCase.call(s); !errors.Is(err, core.ErrBadHandle) {
				t.Errorf("got %v, want ErrBadHandle", err)
			}
			want(t, s, x, 2)
		})
	}
}

func TestConcurrentSessions(t *testing.T) {
	t.Parallel()
	m, str, _, life := attrModel(t, nil)
	for range 4 {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			s := newSession(t, m, 1)
			add(t, s, core.Modifier{Stat: str, Op: core.OpBase, Value: 20})
			want(t, s, life, 10)
		})
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
