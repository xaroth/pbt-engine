package core

import (
	"math"
	"math/rand/v2"
	"testing"
)

// sheetWorld approximates a character sheet over a full passive tree: 2400 sources with
// 1 or 2 modifiers each over attributes, pools, defences and filler stats, a tenth of them
// conditional or scaled.
type sheetWorld struct {
	m       *Model
	mods    []Modifier
	outputs []StatID
	life    StatID
	nSrc    int
}

func newSheetWorld() *sheetWorld {
	w := &sheetWorld{m: NewModel(), nSrc: 2400}
	m := w.m
	str := m.MustAddStat(StatDef{Name: "str", CycleBreak: true, Round: RoundFloor})
	dex := m.MustAddStat(StatDef{Name: "dex", CycleBreak: true, Round: RoundFloor})
	in := m.MustAddStat(StatDef{Name: "int", CycleBreak: true, Round: RoundFloor})
	pool := func(name string, attr StatID, div float64) StatID {
		id := m.MustAddStat(StatDef{Name: name, Round: RoundHalf})
		m.SetFormula(id, func(r Reader, a Agg) float64 {
			return (a.Base + math.Floor(r.Stat(attr)/div)) * (1 + a.Inc/100) * a.More
		})
		return id
	}
	w.life = pool("life", str, 2)
	mana := pool("mana", in, 2)
	es := pool("es", in, 10)
	eva := pool("evasion", dex, 5)
	arm := m.MustAddStat(StatDef{Name: "armour", Round: RoundHalf})
	ele := m.MustAddStat(StatDef{Name: "ele_res"})
	var res []StatID
	for _, n := range []string{"fire", "cold", "lightning"} {
		res = append(res, m.MustAddStat(StatDef{Name: n + "_res", Includes: []StatID{ele}, HasMax: true, Max: 75}))
	}
	regen := m.MustAddStat(StatDef{Name: "life_regen", Round: RoundHalf, Places: 1})
	m.SetFormula(regen, func(r Reader, a Agg) float64 { return r.Stat(w.life) * a.Base / 100 })
	low := m.MustAddStat(StatDef{Name: "low_life", Flag: true})
	var filler []StatID
	for i := range 60 {
		filler = append(filler, m.MustAddStat(StatDef{Name: "filler" + string(rune('a'+i/26)) + string(rune('a'+i%26))}))
	}
	w.outputs = append([]StatID{str, dex, in, w.life, mana, es, eva, arm, regen}, res...)

	r := rand.New(rand.NewPCG(1, 2))
	attrs := []StatID{str, dex, in}
	targets := append([]StatID{w.life, mana, es, eva, arm, ele, regen}, res...)
	for src := range w.nSrc {
		for range 1 + r.IntN(2) {
			mod := Modifier{Source: SourceID(src), Value: float64(5 + r.IntN(20))}
			switch k := r.IntN(10); {
			case k < 3:
				mod.Stat = attrs[r.IntN(3)]
				mod.Value = 10
			case k < 7:
				mod.Stat = targets[r.IntN(len(targets))]
				mod.Op = Op(r.IntN(2))
			default:
				mod.Stat = filler[r.IntN(len(filler))]
				mod.Op = Op(r.IntN(3))
			}
			if r.IntN(10) == 0 {
				if r.IntN(2) == 0 {
					mod.Conds = []Cond{{Ref: SelfRef(low), Cmp: CmpGE, Threshold: 1}}
				} else {
					mod.Scales = []Scale{{Ref: SelfRef(attrs[r.IntN(3)]), Step: 10, Cap: 5}}
				}
			}
			w.mods = append(w.mods, mod)
		}
	}
	for _, a := range attrs { // class base
		w.mods = append(w.mods, Modifier{Stat: a, Op: OpBase, Value: 20, Source: SourceID(w.nSrc)})
	}
	return w
}

func (w *sheetWorld) session(b *testing.B) *Session {
	s := NewSession(w.m, 2)
	for _, m := range w.mods {
		if _, err := s.AddModifier(m); err != nil {
			b.Fatal(err)
		}
	}
	s.Commit()
	return s
}

func (w *sheetWorld) sheet(b *testing.B, s *Session) {
	for _, o := range w.outputs {
		if _, err := s.Get(0, o, GlobalCtx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAssemble(b *testing.B) {
	w := newSheetWorld()
	b.ReportAllocs()
	for b.Loop() {
		w.session(b)
	}
	b.ReportMetric(float64(len(w.mods)), "mods")
}

func BenchmarkSheetCold(b *testing.B) {
	w := newSheetWorld()
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	for b.Loop() {
		s.Invalidate()
		w.sheet(b, s)
	}
}

// BenchmarkNodePower toggles every source off and on and reads life, as node power does.
func BenchmarkNodePower(b *testing.B) {
	w := newSheetWorld()
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	for b.Loop() {
		for src := range SourceID(w.nSrc) {
			cp := s.Mark()
			s.SetSourceEnabled(src, false)
			if _, err := s.Get(0, w.life, GlobalCtx); err != nil {
				b.Fatal(err)
			}
			s.Revert(cp)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*w.nSrc), "ns/whatif")
}
