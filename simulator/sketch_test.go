//nolint:thelper // the parts run as subtests, so their failures belong to their own lines
package simulator_test

import (
	"errors"
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/power"
	"github.com/xaroth/pbt-engine/simulator"
)

//nolint:gochecknoglobals // the testing package parses flags before any test runs, so the flag must exist at package init
var update = flag.Bool("update", false, "rewrite testdata/sketch-skipped.golden")

// The sketch is the public API as first laid out for v1, statement by statement in its order,
// run against the real pinned datasets. A part the engine cannot do yet keeps the sketch's code
// as text and is skipped, naming what it needs; TestSketchSkipped holds that list to
// testdata/sketch-skipped.golden and fails once a need is met.

// need is a feature a skipped part waits for. It is missing while any symbol in waitsOn is
// undeclared or probe reports true. Symbols are the sketch's names: a feature that lands under
// another name goes unnoticed until the part is updated.
type need struct {
	what    string
	waitsOn []string
	probe   func() bool
}

func (n need) missing(s symbolSet) bool {
	if n.probe != nil && n.probe() {
		return true
	}
	return slices.ContainsFunc(n.waitsOn, func(sym string) bool { return !s.has(sym) })
}

func needSkillSim(syms ...string) need { return need{what: "skill simulation", waitsOn: syms} }

func needEnemy(syms ...string) need { return need{what: "enemy", waitsOn: syms} }

func needDerived(syms ...string) need { return need{what: "derived conditions", waitsOn: syms} }

func needShareCodes() need { return need{what: "share codes", waitsOn: []string{"build.FromCode"}} }

func needBuildNew() need {
	return need{what: "build.New", waitsOn: []string{"build.New", "poe1.Witch", "poe1.Elementalist"}}
}

func needCharges() need {
	return need{what: "charge config keys", waitsOn: []string{"poe1.PowerCharges"}}
}

// needTypedClear: Clear takes a key name today, not a typed key.
func needTypedClear() need {
	return need{what: "typed Clear", probe: func() bool {
		f := reflect.TypeOf(simulator.Clear)
		return f.NumIn() != 1 || f.In(0).Kind() == reflect.String
	}}
}

func needConfigOptions() need {
	return need{what: "Config().Options()", probe: func() bool {
		m, ok := reflect.TypeFor[*simulator.Sim]().MethodByName("Config")
		if !ok || m.Type.NumOut() != 1 {
			return true
		}
		_, ok = m.Type.Out(0).MethodByName("Options")
		return !ok
	}}
}

// sketchPart is one or more statements of the sketch: run when the engine can do them, else
// code with the needs it waits for.
type sketchPart struct {
	name  string
	run   func(*testing.T, *sketchState)
	code  string
	needs []need
}

type sketchState struct {
	reg                   *dataset.Registry
	res                   dataset.Resolution
	sim                   *simulator.Sim
	base, next            *build.Build
	fireball, inspiration build.GemID
	stats                 *simulator.Stats
	diff                  *simulator.Diff
}

func sketch() []sketchPart {
	return slices.Concat(sketchSetup(), sketchLoad(), sketchEdit(), sketchRun(), sketchCompare())
}

func sketchSetup() []sketchPart {
	return []sketchPart{
		{name: "open the datasets", run: sketchOpen},
		{name: "resolve a league", run: sketchResolve},
		{name: "make a sim against a pinnacle boss", code: `sim, err := simulator.New(res.Store, simulator.Enemy(poe1.Pinnacle))`,
			needs: []need{needEnemy("simulator.Enemy", "poe1.Pinnacle")}},
		// The rest of the sketch needs a sim, so this one has no enemy.
		{name: "make a sim", run: sketchNew},
		{name: "set the enemy's fire resistance", code: `sim.Configure(simulator.Set(poe1.EnemyFireResist, 40))`,
			needs: []need{needEnemy("poe1.EnemyFireResist")}},
		{name: "read the config, its keys and the outputs", run: sketchReadConfig},
	}
}

func sketchLoad() []sketchPart {
	return []sketchPart{
		{name: "import a Path of Building file", run: sketchFromXML},
		{name: "import a share code", code: `base, warnings, err = build.FromCode(shareCode)`, needs: []need{needShareCodes()}},
		{name: "parse a build file", run: sketchParse},
		{name: "make a build from scratch", code: `base = build.New(poe1.Witch, poe1.Elementalist)`, needs: []need{needBuildNew()}},
	}
}

func sketchEdit() []sketchPart {
	return []sketchPart{
		{name: "clone and apply edits", run: sketchApply},
		{name: "equip an item from its text", run: sketchEquip},
		{name: "set charges in the build's config", code: `err = config.Set(&next.Config, poe1.PowerCharges, 3)`, needs: []need{needCharges()}},
		{name: "look gems up by name", run: sketchGems},
		{name: "edit the main skill group", run: sketchGroup},
		{name: "an edit of a missing group keeps ErrEditTarget", run: sketchEditTarget},
	}
}

func sketchRun() []sketchPart {
	return []sketchPart{
		{name: "simulate against a map boss", code: `stats, err := sim.Simulate(next, simulator.Enemy(poe1.Map(83)))`,
			needs: []need{needEnemy("simulator.Enemy", "poe1.Map")}},
		// The rest of the sketch needs stats, so this call has no enemy.
		{name: "simulate", run: sketchSimulate},
		{name: "a build the dataset cannot take is ErrUnreconcilable", run: sketchUnreconcilable},
		{name: "read an output", run: sketchGet},
		{name: "read a skill's average hit", code: `stats.Skill(fireball).Get(poe1.AverageHit)`,
			needs: []need{needSkillSim("simulator.Stats.Skill", "poe1.AverageHit")}},
		{name: "a gem not in the dataset warns", run: sketchGemWarning},
		{name: "a condition with no source warns", code: `stats.Warnings() // {Code: NoConditionSource, Subject: "enemy.shocked", ...}`,
			needs: []need{needDerived("simulator.NoConditionSource")}},
		{name: "read a derived condition and its source", code: `v, ok := stats.Config(poe1.EnemyShocked)
_ = v.Value  // true
_ = v.Source // config.Derived`, needs: []need{needDerived("poe1.EnemyShocked", "config.Derived")}},
		{name: "explain an output in each view", run: sketchExplain},
		{name: "compute only the listed outputs", run: sketchOutputs},
		{name: "force a condition off", code: `sim.Configure(simulator.Set(poe1.EnemyShocked, false))`,
			needs: []need{needDerived("poe1.EnemyShocked")}},
		{name: "hand a condition back", code: `sim.Configure(simulator.Clear(poe1.EnemyShocked))`,
			needs: []need{needDerived("poe1.EnemyShocked"), needTypedClear()}},
	}
}

func sketchCompare() []sketchPart {
	return []sketchPart{
		{name: "compare two builds", run: sketchCompareBuilds},
		{name: "read the change of damage per second", code: `d := diff.Get(poe1.TotalDPS) // d.Before, d.After, d.Abs, d.Pct`,
			needs: []need{needSkillSim("poe1.TotalDPS")}},
		{name: "read what changed, the edits and both sides' warnings", run: sketchDiffParts},
		{name: "diff one build on two datasets", run: sketchDataDiff},
		{name: "sweep variants ranked by damage per second", code: `results, err := sim.Sweep(base, [][]build.Edit{
	{build.Deallocate(clusterA...), build.Allocate(clusterB...)},
	{build.SetGemLevel(0, 0, 21)},
}, simulator.RankBy(poe1.TotalDPS), simulator.Workers(8))
_ = results[0].Edits
_ = results[0].Changes
_ = results[0].Err`, needs: []need{needSkillSim("poe1.TotalDPS")}},
		{name: "stream a sweep", run: sketchSweepEach},
		{name: "node power", run: sketchPower},
		{name: "a new sim with the first one's config", code: `sim2, err := simulator.New(res2.Store, sim.Config().Options()...)`,
			needs: []need{needConfigOptions()}},
	}
}

// skipReason is what p still needs, empty when it needs nothing.
func (p sketchPart) skipReason(s symbolSet) string {
	var out []string
	for _, n := range p.needs {
		if n.missing(s) {
			out = append(out, n.what)
		}
	}
	return strings.Join(out, ", ")
}

//nolint:tparallel // the parts build on each other's state, so they run in order
func TestSketch(t *testing.T) {
	t.Parallel()
	for _, v := range realdata.Versions() {
		realdata.Open(t, v)
	}
	symbols := sourceSymbols(t)
	s := &sketchState{}
	for _, p := range sketch() {
		ok := t.Run(p.name, func(t *testing.T) {
			if p.run == nil {
				if r := p.skipReason(symbols); r != "" {
					t.Skip("needs " + r)
				}
				t.Fatal("every need is met; turn the code into a running part")
			}
			p.run(t, s)
		})
		if !ok {
			return
		}
	}
}

// TestSketchSkipped holds the skipped parts and what each needs to the golden file, so a met
// need fails until its skip is removed or the golden updated.
func TestSketchSkipped(t *testing.T) {
	t.Parallel()
	symbols := sourceSymbols(t)
	var b strings.Builder
	names := map[string]bool{}
	for _, p := range sketch() {
		if names[p.name] {
			t.Errorf("part %q: name repeated", p.name)
		}
		names[p.name] = true
		if (p.run == nil) == (p.code == "") || (p.code == "") != (len(p.needs) == 0) {
			t.Errorf("part %q: want either run, or code and needs", p.name)
			continue
		}
		if p.run != nil {
			continue
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "", "package p\nfunc _() {\n"+p.code+"\n}", 0); err != nil {
			t.Errorf("part %q: code does not parse: %v", p.name, err)
		}
		checkWaitsOn(t, p, symbols)
		r := p.skipReason(symbols)
		if r == "" {
			t.Errorf("part %q: every need is met; turn the code into a running part", p.name)
		}
		b.WriteString(p.name + ": " + r + "\n")
	}
	path := filepath.Join("testdata", "sketch-skipped.golden")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != string(want) {
		t.Errorf("skipped parts differ from %s (run with -update after checking):\ngot:\n%swant:\n%s", path, got, want)
	}
}

// checkWaitsOn: a symbol in a package the index does not parse, or one the code does not use,
// would keep its part skipped forever.
func checkWaitsOn(t *testing.T, p sketchPart, symbols symbolSet) {
	t.Helper()
	for _, n := range p.needs {
		for _, sym := range n.waitsOn {
			if pkg, _, _ := strings.Cut(sym, "."); !symbols.has(pkg) {
				t.Errorf("part %q: waits on %s, in a package the symbol index does not parse", p.name, sym)
			}
			if !strings.Contains(p.code, sym[strings.LastIndex(sym, ".")+1:]) {
				t.Errorf("part %q: waits on %s, which its code does not use", p.name, sym)
			}
		}
	}
}

func sketchOpen(t *testing.T, s *sketchState) {
	reg, err := dataset.OpenRoot(os.DirFS(os.Getenv("PBT_DATASETS")))
	if reg == nil {
		t.Fatal(err)
	}
	s.reg = reg
}

func sketchResolve(t *testing.T, s *sketchState) {
	res, err := s.reg.Resolve("poe1", "3.29")
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Store.Manifest().Version; v != realdata.Current {
		t.Fatalf("3.29 resolved to %s, want %s", v, realdata.Current)
	}
	s.res = res
}

func sketchNew(t *testing.T, s *sketchState) {
	sim, err := simulator.New(s.res.Store)
	if err != nil {
		t.Fatal(err)
	}
	s.sim = sim
}

func sketchReadConfig(t *testing.T, s *sketchState) {
	if err := s.sim.Configure(simulator.Set(poe1.Campaign, poe1.CampaignActs6To10)); err != nil {
		t.Fatal(err)
	}
	if got := string(s.sim.Config()[poe1.Campaign.Name()]); got != `"acts-6-10"` {
		t.Errorf("sim config campaign %s", got)
	}
	keys, err := s.sim.ConfigKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(keys, func(k simulator.ConfigKey) bool { return k.Name == poe1.Campaign.Name() }) {
		t.Error("config keys lack campaign")
	}
	if !slices.ContainsFunc(s.sim.Outputs(), func(o simulator.Output) bool { return o.Key == poe1.Life }) {
		t.Error("outputs lack life")
	}
}

// sketchXML is the level 90 Chieftain of the explanation goldens with a Fireball group.
func sketchXML(t *testing.T) []byte {
	const skills = `<Skills activeSkillSet="1"><SkillSet id="1">
	<Skill enabled="true" slot="Helmet" mainActiveSkill="1">
		<Gem nameSpec="Fireball" skillId="Fireball" gemId="Metadata/Items/Gems/SkillGemFireball" level="20" quality="0" enabled="true"/>
	</Skill>
</SkillSet></Skills>
</PathOfBuilding>`
	return []byte(strings.Replace(string(chieftainXML(t)), "</PathOfBuilding>", skills, 1))
}

func sketchFromXML(t *testing.T, s *sketchState) {
	base, warnings, err := pob.FromXML(sketchXML(t), s.reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Skills.Groups) != 1 || base.Skills.Groups[0].Gems[0].ID != gemFireball {
		t.Errorf("imported skills %+v, want the Fireball group (warnings %v)", base.Skills, warnings)
	}
	s.base = base
}

func sketchParse(t *testing.T, s *sketchState) {
	jsonBytes, err := s.base.Encode()
	if err != nil {
		t.Fatal(err)
	}
	base, err := build.Parse(jsonBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, s.base) {
		t.Errorf("parsed %+v, want %+v", base, s.base)
	}
	s.base = base
}

func sketchApply(t *testing.T, s *sketchState) {
	cluster, _ := marginalLife(t, s.sim, s.base)
	cluster = cluster[:3]
	s.next = s.base.Clone()
	if err := s.next.Apply(build.Deallocate(cluster...), build.SetLevel(95)); err != nil {
		t.Fatal(err)
	}
	if s.next.Level != 95 || len(s.next.Nodes) != len(s.base.Nodes)-3 || s.base.Level != 90 {
		t.Errorf("after the edits: level %d with %d nodes, base level %d with %d", s.next.Level, len(s.next.Nodes), s.base.Level, len(s.base.Nodes))
	}
}

// rawItemText is a rare helmet as Path of Building writes it.
const rawItemText = `Rarity: RARE
Foe Horn
Conqueror's Helmet
Armour: 767
Item Level: 85
Quality: 20
Sockets: R-R-R-R
Implicits: 0
+68 to Armour
+101 to maximum Life
82% increased Armour
+33% to Fire Resistance
{crafted}+28% to Physical Damage over Time Multiplier
`

func sketchEquip(t *testing.T, s *sketchState) {
	item, warnings, err := s.sim.Data().Item(rawItemText)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range warnings {
		if x.Code != items.LineAmbiguous {
			t.Errorf("read the item: %v", x)
		}
	}
	if err := s.next.Apply(build.Equip(build.SlotHelmet, item)); err != nil {
		t.Fatal(err)
	}
	if got := s.next.Items[build.SlotHelmet]; got.Base == "" || len(got.Mods) != 5 {
		t.Errorf("equipped %+v", got)
	}
}

func sketchGems(t *testing.T, s *sketchState) {
	var err error
	if s.fireball, err = s.sim.Data().Gem("Fireball"); err != nil {
		t.Fatal(err)
	}
	if s.inspiration, err = s.sim.Data().Gem("Inspiration"); err != nil {
		t.Fatal(err)
	}
}

func sketchGroup(t *testing.T, s *sketchState) {
	group := s.next.Skills.Main()
	group.Gem(s.fireball).SetLevel(21).SetQuality(23)
	group.AddSupport(s.inspiration, 20, 20)
	if err := group.Err(); err != nil {
		t.Fatal(err)
	}
	want := []build.Gem{{GemID: s.fireball, Level: 21, Quality: 23}, {GemID: s.inspiration, Level: 20, Quality: 20, Support: true}}
	if got := s.next.Skills.Groups[0].Gems; !reflect.DeepEqual(got, want) {
		t.Errorf("main group gems %+v, want %+v", got, want)
	}
}

func sketchEditTarget(t *testing.T, s *sketchState) {
	b := s.next.Clone()
	if err := b.Skills.Group(5).Gem(s.fireball).SetLevel(1).Err(); !errors.Is(err, build.ErrEditTarget) {
		t.Errorf("edit of group 5 of 1: %v, want ErrEditTarget", err)
	}
}

func sketchSimulate(t *testing.T, s *sketchState) {
	s.stats = simulate(t, s.sim, s.next)
}

func sketchUnreconcilable(t *testing.T, s *sketchState) {
	b := s.next.Clone()
	b.Game = "poe2"
	if _, err := s.sim.Simulate(b); !errors.Is(err, simulator.ErrUnreconcilable) {
		t.Errorf("a poe2 build: %v, want ErrUnreconcilable", err)
	}
}

func sketchGet(t *testing.T, s *sketchState) {
	life, ok := s.stats.Get(poe1.Life)
	if !ok || life <= 0 {
		t.Errorf("life %v, %v", life, ok)
	}
}

func sketchGemWarning(t *testing.T, s *sketchState) {
	b := s.next.Clone()
	b.Skills.Main().AddSupport(build.GemID{ID: "Metadata/Items/Gems/SkillGemSupportNothing"}, 20, 0)
	warnings := simulate(t, s.sim, b).Warnings()
	if !slices.ContainsFunc(warnings, func(w build.Warning) bool {
		return w.Code == build.GemUnknown && strings.Contains(w.Message, "SkillGemSupportNothing")
	}) {
		t.Errorf("warnings %v, want the unknown gem", warnings)
	}
}

func sketchExplain(t *testing.T, s *sketchState) {
	life := get(t, s.stats, poe1.Life)
	for _, v := range []simulator.View{simulator.Proportional, simulator.Tree, simulator.Marginal} {
		ex, err := s.stats.Explain(poe1.Life, v)
		if err != nil {
			t.Fatal(err)
		}
		// The tree view fills Tree, the others Lines.
		if ex.Value != life || (v == simulator.Tree) != (ex.Tree != nil) || (v == simulator.Tree) != (len(ex.Lines) == 0) {
			t.Errorf("view %v: %d lines, tree %v, value %v for life %v", v, len(ex.Lines), ex.Tree != nil, ex.Value, life)
		}
	}
}

func sketchOutputs(t *testing.T, s *sketchState) {
	st := simulate(t, s.sim, s.next, simulator.Outputs(poe1.Life, poe1.Mana))
	if !slices.Equal(st.Keys(), []output.Key{poe1.Life, poe1.Mana}) {
		t.Errorf("computed %v, want life and mana", st.Keys())
	}
}

func sketchCompareBuilds(t *testing.T, s *sketchState) {
	diff, err := s.sim.Compare(s.base, s.next)
	if err != nil {
		t.Fatal(err)
	}
	s.diff = diff
}

func sketchDiffParts(t *testing.T, s *sketchState) {
	if !slices.Contains(s.diff.Changed(), poe1.Life) {
		t.Errorf("changed %v, want life", s.diff.Changed())
	}
	if len(s.diff.Edits) == 0 {
		t.Error("no edits between base and next")
	}
	if len(s.diff.Before.Warnings()) == 0 || len(s.diff.After.Warnings()) == 0 {
		t.Error("a side without warnings; both have unmodelled stats")
	}
}

func sketchDataDiff(t *testing.T, s *sketchState) {
	other, err := s.reg.Resolve("poe1", "3.28")
	if err != nil {
		t.Fatal(err)
	}
	simOtherLeague, err := simulator.New(other.Store)
	if err != nil {
		t.Fatal(err)
	}
	a, b := simulate(t, s.sim, s.base), simulate(t, simOtherLeague, s.base)
	diff, err := a.Diff(b)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Before.Version() == diff.After.Version() {
		t.Errorf("both sides on %s", diff.Before.Version())
	}
	if len(diff.DataChanges) == 0 {
		t.Error("no data changes between 3.28 and 3.29 for a level 90 tree")
	}
}

func sketchSweepEach(t *testing.T, s *sketchState) {
	variants := [][]build.Edit{{build.SetLevel(80)}, {build.SetGemLevel(0, 0, 21)}}
	seq, err := s.sim.SweepEach(s.base, variants, simulator.Outputs(poe1.Life))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for r := range seq {
		if c := r.Changes[poe1.Life]; r.Err != nil || !c.Has {
			t.Errorf("variant %d: life %+v, %v", r.Index, c, r.Err)
		}
		n++
	}
	if n != len(variants) {
		t.Errorf("%d results, want %d", n, len(variants))
	}
}

func sketchPower(t *testing.T, s *sketchState) {
	nodes, err := power.Nodes(s.sim, s.base, power.MaxDistance(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 || slices.ContainsFunc(nodes, func(n power.NodePower) bool { return len(n.Path) > 3 || n.Alone.Err != nil }) {
		t.Errorf("%d candidates, want some, none further than 3 nodes or failing", len(nodes))
	}
}
