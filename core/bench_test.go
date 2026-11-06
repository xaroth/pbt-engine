package core_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/xaroth/pbt-engine/core"
)

// sheetWorld approximates a character sheet over a full passive tree: 2400 sources with
// 1 or 2 modifiers each over attributes, pools, defences and filler stats, a tenth of them
// conditional or scaled.
type sheetWorld struct {
	m       *core.Model
	mods    []core.Modifier
	outputs []core.StatID
	life    core.StatID
	nSrc    int
}

func newSheetWorld(b *testing.B) *sheetWorld {
	w := &sheetWorld{nSrc: 2400}
	var mb core.ModelBuilder
	str := mb.MustAddStat(core.StatDef{Name: "str", CycleFamily: 1, Round: core.RoundFloor})
	dex := mb.MustAddStat(core.StatDef{Name: "dex", CycleFamily: 1, Round: core.RoundFloor})
	in := mb.MustAddStat(core.StatDef{Name: "int", CycleFamily: 1, Round: core.RoundFloor})
	pool := func(name string, attr core.StatID, div float64) core.StatID {
		id := mb.MustAddStat(core.StatDef{Name: name, Round: core.RoundHalf})
		setFormula(b, &mb, id, func(r core.Reader, a core.Agg) float64 {
			return (a.Base + math.Floor(r.Stat(attr)/div)) * (1 + a.Inc/100) * a.More
		})
		return id
	}
	w.life = pool("life", str, 2)
	mana := pool("mana", in, 2)
	es := pool("es", in, 10)
	eva := pool("evasion", dex, 5)
	arm := mb.MustAddStat(core.StatDef{Name: "armour", Round: core.RoundHalf})
	ele := mb.MustAddStat(core.StatDef{Name: "ele_res"})
	var res []core.StatID
	for _, n := range []string{"fire", "cold", "lightning"} {
		res = append(res, mb.MustAddStat(core.StatDef{Name: n + "_res", Includes: []core.StatID{ele}, HasMax: true, Max: 75}))
	}
	regen := mb.MustAddStat(core.StatDef{Name: "life_regen", Round: core.RoundHalf, Places: 1})
	setFormula(b, &mb, regen, func(r core.Reader, a core.Agg) float64 { return r.Stat(w.life) * a.Base / 100 })
	low := mb.MustAddStat(core.StatDef{Name: "low_life", Flag: true})
	var filler []core.StatID
	for i := range 60 {
		filler = append(filler, mb.MustAddStat(core.StatDef{Name: "filler" + string(rune('a'+i/26)) + string(rune('a'+i%26))}))
	}
	w.outputs = append([]core.StatID{str, dex, in, w.life, mana, es, eva, arm, regen}, res...)

	r := rand.New(rand.NewPCG(1, 2))
	attrs := []core.StatID{str, dex, in}
	targets := append([]core.StatID{w.life, mana, es, eva, arm, ele, regen}, res...)
	for src := range w.nSrc {
		for range 1 + r.IntN(2) {
			mod := core.Modifier{Source: core.SourceID(src), Value: float64(5 + r.IntN(20))}
			switch k := r.IntN(10); {
			case k < 3:
				mod.Stat = attrs[r.IntN(3)]
				mod.Value = 10
			case k < 7:
				mod.Stat = targets[r.IntN(len(targets))]
				mod.Op = core.Op(r.IntN(2))
			default:
				mod.Stat = filler[r.IntN(len(filler))]
				mod.Op = core.Op(r.IntN(3))
			}
			if r.IntN(10) == 0 {
				if r.IntN(2) == 0 {
					mod.Conds = []core.Cond{{Ref: core.SelfRef(low), Cmp: core.CmpGE, Threshold: 1}}
				} else {
					mod.Scales = []core.Scale{{Ref: core.SelfRef(attrs[r.IntN(3)]), Step: 10, Cap: 5, HasCap: true}}
				}
			}
			w.mods = append(w.mods, mod)
		}
	}
	for _, a := range attrs { // class base
		w.mods = append(w.mods, core.Modifier{Stat: a, Op: core.OpBase, Value: 20, Source: core.SourceID(w.nSrc)})
	}
	w.m = mb.Build()
	return w
}

func (w *sheetWorld) session(b *testing.B) *core.Session {
	s, err := core.NewSession(w.m, 2)
	if err != nil {
		b.Fatal(err)
	}
	for range w.nSrc + 1 {
		s.NewSource()
	}
	for _, m := range w.mods {
		if _, err := s.AddModifier(m); err != nil {
			b.Fatal(err)
		}
	}
	return s
}

func (w *sheetWorld) sheet(b *testing.B, s *core.Session) {
	for _, o := range w.outputs {
		if _, err := s.Get(0, o, core.GlobalCtx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAssemble(b *testing.B) {
	w := newSheetWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.session(b)
	}
	b.ReportMetric(float64(len(w.mods)), "mods")
}

func BenchmarkSheetCold(b *testing.B) {
	w := newSheetWorld(b)
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	for b.Loop() {
		s.Invalidate()
		w.sheet(b, s)
	}
}

// BenchmarkNodePower disables each source in turn, reads life and enables the source again,
// as node power does.
func BenchmarkNodePower(b *testing.B) {
	w := newSheetWorld(b)
	s := w.session(b)
	w.sheet(b, s)
	b.ReportAllocs()
	whatIfs := 0
	for b.Loop() {
		for src := range core.SourceID(w.nSrc) {
			if err := s.SetSourceEnabled(src, false); err != nil {
				b.Fatal(err)
			}
			if _, err := s.Get(0, w.life, core.GlobalCtx); err != nil {
				b.Fatal(err)
			}
			if err := s.SetSourceEnabled(src, true); err != nil {
				b.Fatal(err)
			}
			whatIfs++
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(whatIfs), "ns/whatif")
}
