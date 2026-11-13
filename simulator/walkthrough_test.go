//nolint:thelper // the steps run as subtests, so their failures belong to their own lines
package simulator_test

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// The walkthrough is the public API as a host uses it, from loading a dataset to sweeping
// edits, in order. A step the engine can do runs against the real pinned datasets; a step it
// cannot do yet is kept as the code it should become, with the capability it waits for
// (walkthrough_pending_test.go checks those). Running steps import only public packages, as a
// host would.

// walk is the state the running steps pass along.
type walk struct {
	// fireball and inspiration are the gems the skill steps use.
	fireball, inspiration build.GemID
	reg                   *dataset.Registry
	res                   dataset.Resolution
	sim                   *simulator.Sim
	base, next            *build.Build
	stats                 *simulator.Stats
	// cut are the three nodes other than masteries that give the most life alone, by the
	// marginal explanation.
	cut []uint32
}

// step is one step of the walkthrough: run when the engine can do it, else pending.
type step struct {
	name    string
	run     func(*testing.T, *walk)
	pending *pending
}

func walkthrough() []step {
	return slices.Concat(setupSteps(), editSteps(), readSteps(), compareSteps(), actorSteps())
}

func setupSteps() []step {
	return []step{
		{name: "open the datasets the host hands in", run: openDatasets},
		{name: "resolve a league to its newest dataset", run: resolveLeague},
		{name: "make a sim", run: makeSim},
		{name: "make a sim against a pinnacle boss", run: pinnacleSim},
		{name: "configure the sim and read its config", run: configureSim},
		{name: "set the enemy's fire resistance on the sim", run: enemyResistance},
		{name: "import a Path of Building file", run: importPoB},
		{name: "import a Path of Building share code", run: importShareCode},
		{name: "parse a build file", run: parseBuildFile},
		{name: "make a build from scratch", run: newBuild},
	}
}

func editSteps() []step {
	return []step{
		{name: "clone a build to edit it", run: cloneBuild},
		{name: "turn gem names into ids", run: gemNames},
		{name: "edit the main skill group", run: editMainGroup},
		{name: "deallocate nodes", run: deallocate},
		{name: "equip an item from its text", run: equipItem},
		{name: "set charges in the build's config", pending: &pending{
			needs: "charges in config", waitsOn: []string{"poe1.PowerCharges"},
			code: `next.Config.Set(poe1.PowerCharges, 3)`,
		}},
		{name: "simulate", run: simulateNext},
		{name: "simulate against a map boss for one call", run: simulateMapBoss},
		{name: "a build the dataset cannot take is an error", run: unreconcilable},
	}
}

func readSteps() []step {
	return []step{
		{name: "read an output", run: readOutput},
		{name: "read a skill's average hit", pending: &pending{
			needs: "skills on a build", waitsOn: []string{"simulator.Stats.Skill", "poe1.AverageHit"},
			code: `stats.Skill(fireball).Get(poe1.AverageHit)`,
		}},
		{name: "read the warnings", run: readWarnings},
		{name: "warn about a gem the dataset lacks", run: unknownGem},
		{name: "warn about a condition whose source the build lacks", pending: &pending{
			needs: "conditions derived from the build", waitsOn: []string{"simulator.NoConditionSource"},
			code: `stats.Warnings() // {Code: NoConditionSource, Subject: "enemy.shocked", Message: "could not detect source, assuming external"}`,
		}},
		{name: "read where a config value came from", run: readConfigSource},
		{name: "read whether the enemy is shocked, and why", pending: &pending{
			needs: "conditions derived from the build", waitsOn: []string{"poe1.EnemyShocked", "config.Derived"},
			code: `v, _ := stats.Config(poe1.EnemyShocked)
_ = v.Value  // true
_ = v.Source // config.Derived`,
		}},
		{name: "explain an output", run: explainOutput},
		{name: "compute only the listed outputs", run: listedOutputs},
		{name: "force a condition off, then hand it back", pending: &pending{
			needs: "conditions derived from the build", waitsOn: []string{"poe1.EnemyShocked"},
			code: `sim.Configure(simulator.Set(poe1.EnemyShocked, false))
sim.Configure(simulator.Clear(poe1.EnemyShocked))`,
		}},
		{name: "clear a sim value", run: clearSimValue},
	}
}

func compareSteps() []step {
	return []step{
		{name: "compare two builds", run: compareBuilds},
		{name: "compare damage per second", pending: &pending{
			needs: "skills on a build", waitsOn: []string{"poe1.TotalDPS"},
			code: `diff, err := sim.Compare(base, next)
d := diff.Get(poe1.TotalDPS)`,
		}},
		{name: "diff one build across two leagues", run: diffAcrossLeagues},
		{name: "sweep edits ranked by an output", run: sweep},
		{name: "sweep a gem level ranked by damage per second", pending: &pending{
			needs: "skills on a build", waitsOn: []string{"build.SetGemLevel", "poe1.TotalDPS"},
			code: `results, err := sim.Sweep(base, [][]build.Edit{{build.SetGemLevel(base.Skills.MainGroup, 0, 21)}}, simulator.RankBy(poe1.TotalDPS))`,
		}},
		{name: "stream a sweep", run: sweepEach},
		{name: "make a second sim with the first one's config", run: secondSim},
	}
}

func actorSteps() []step {
	return []step{
		{name: "read a minion's life and its skill's hit", pending: &pending{
			needs: "minions", waitsOn: []string{"simulator.*.Minion"},
			code: `zombie := stats.Skill(raiseZombie).Minion()
zombie.Get(poe1.Life)
zombie.Skill(zombieSlam).Get(poe1.AverageHit)`,
		}},
		{name: "place a skill with a totem", pending: &pending{
			needs: "totems", waitsOn: []string{"poe1.TotemLife"},
			code: `group.AddSupport(spellTotem, 20, 0)
stats.Skill(fireball).Get(poe1.TotemLife)
stats.Skill(fireball).Get(poe1.TotalDPS) // includes the totem count`,
		}},
		{name: "simulate with an ally whose aura reaches the player", pending: &pending{
			needs: "a party", waitsOn: []string{"simulator.Party"},
			code: `stats, err = sim.Simulate(next, simulator.Party(ally))
stats.Explain(poe1.Life, simulator.Proportional) // sources name the ally's aura`,
		}},
	}
}

//nolint:tparallel // the steps build on each other's state, so they run in order
func TestWalkthrough(t *testing.T) {
	t.Parallel()
	for _, v := range realdata.Versions() {
		realdata.Open(t, v)
	}
	w := &walk{}
	for _, s := range walkthrough() {
		if s.run == nil {
			continue
		}
		if !t.Run(s.name, func(t *testing.T) { s.run(t, w) }) {
			return
		}
	}
}

func openDatasets(t *testing.T, w *walk) {
	reg, err := dataset.OpenRoot(os.DirFS(os.Getenv("PBT_DATASETS")))
	if reg == nil {
		t.Fatal(err)
	}
	if n := len(reg.Stores("poe1")); n < 2 {
		t.Fatalf("%d poe1 datasets, want 3.28 and 3.29", n)
	}
	w.reg = reg
}

func resolveLeague(t *testing.T, w *walk) {
	res, err := w.reg.Resolve("poe1", "3.29")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Store.Manifest().Version; res.Match == dataset.MatchExact || got != realdata.Current {
		t.Fatalf("3.29 resolved to %s (match %v), want %s by prefix", got, res.Match, realdata.Current)
	}
	w.res = res
}

func makeSim(t *testing.T, w *walk) {
	sim, err := simulator.New(w.res.Store)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []output.Key{poe1.Life, poe1.Mana, poe1.Strength, poe1.MaximumFireResistance} {
		if !slices.ContainsFunc(sim.Outputs(), func(o simulator.Output) bool { return o.Key == k }) {
			t.Errorf("sim has no output %s", k)
		}
	}
	w.sim = sim
}

// pinnacleSim makes a sim whose enemy is a pinnacle boss, the default enemy.
func pinnacleSim(t *testing.T, w *walk) {
	sim, err := simulator.New(w.res.Store, simulator.Set(poe1.Enemy, poe1.EnemyPinnacle))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(sim.Config()[poe1.Enemy.Name()]); got != `"pinnacle"` {
		t.Errorf("sim config enemy %s, want \"pinnacle\"", got)
	}
}

func enemyResistance(t *testing.T, w *walk) {
	sim, err := simulator.New(w.res.Store)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Configure(simulator.Set(poe1.EnemyFireResistance, 40)); err != nil {
		t.Fatal(err)
	}
	if got := string(sim.Config()[poe1.EnemyFireResistance.Name()]); got != "40" {
		t.Errorf("sim config enemy fire resistance %s, want 40", got)
	}
}

func configureSim(t *testing.T, w *walk) {
	if err := w.sim.Configure(simulator.Set(poe1.Bandits, poe1.BanditsHelpOak)); err != nil {
		t.Fatal(err)
	}
	if got := string(w.sim.Config()[poe1.Bandits.Name()]); got != `"help-oak"` {
		t.Errorf("sim config bandits %s, want \"help-oak\"", got)
	}
	keys, err := w.sim.ConfigKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(keys, func(k simulator.ConfigKey) bool { return k.Name == poe1.Bandits.Name() }) {
		t.Error("config keys lack bandits")
	}
}

// shareCode encodes a build file as PoB writes share codes: zlib, then base64 with the URL
// alphabet.
func shareCode(t *testing.T, file []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

// chieftainXML is the level 90 Chieftain of the explanation goldens as a Path of Building file,
// with the bandits killed and a helmet, which the import leaves out.
func chieftainXML(tb testing.TB) []byte {
	tb.Helper()
	nodes, err := os.ReadFile(filepath.Join("..", "internal", "poe1", "testdata", "explain", "chieftain-L90-3.29.nodes"))
	if err != nil {
		tb.Fatal(err)
	}
	const masteries = "{4139,47642},{11505,36313},{12125,21468},{12382,64381},{25011,31822},{27193,45653},{27733,23471},{32241,34242},{41415,38454}"
	return fmt.Appendf(nil, `<PathOfBuilding>
	<Build level="90" className="Marauder" ascendClassName="Chieftain" bandit="None"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes="%s" masteryEffects="%s"/></Tree>
	<Items><Item id="1">Rarity: RARE
Doom Crown
Royal Burgonet</Item></Items>
</PathOfBuilding>`, strings.TrimSpace(string(nodes)), masteries)
}

func importPoB(t *testing.T, w *walk) {
	b, warnings, err := pob.FromXML(chieftainXML(t), w.reg)
	if err != nil {
		t.Fatal(err)
	}
	if b.Class != "Metadata/Characters/Str/Str" || b.Ascendancy != "Chieftain" || b.Level != 90 || b.Version != "3.29" {
		t.Errorf("imported %s %s level %d on %s, want the level 90 Chieftain on 3.29", b.Class, b.Ascendancy, b.Level, b.Version)
	}
	if len(b.Nodes) < 100 || len(b.Masteries) != 9 {
		t.Errorf("imported %d nodes and %d mastery choices, want over 100 and 9", len(b.Nodes), len(b.Masteries))
	}
	if !slices.ContainsFunc(warnings, func(x build.Warning) bool { return x.Code == pob.ItemIgnored }) {
		t.Errorf("warnings %v, want the helmet ignored", warnings)
	}
	w.base = b
}

func importShareCode(t *testing.T, w *walk) {
	b, _, err := pob.FromCode(shareCode(t, chieftainXML(t)), w.reg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b, w.base) {
		t.Errorf("imported %+v, want the build of the same file", b)
	}
}

func parseBuildFile(t *testing.T, w *walk) {
	data, err := w.base.Encode()
	if err != nil {
		t.Fatal(err)
	}
	b, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Nodes, w.base.Nodes) || b.Class != w.base.Class || string(b.Config[poe1.Bandits.Name()]) != string(w.base.Config[poe1.Bandits.Name()]) {
		t.Errorf("build file round trip changed the build:\n%s", data)
	}
}

func newBuild(t *testing.T, w *walk) {
	b := &build.Build{Game: "poe1", Version: "3.29", Class: "Metadata/Characters/Int/Int", Ascendancy: "Elementalist", Level: 1}
	// A level 1 Witch with no nodes has the class's 14 strength.
	if got := get(t, simulate(t, w.sim, b), poe1.Strength); got != 14 {
		t.Errorf("level 1 Witch strength %v, want 14", got)
	}
}

func cloneBuild(t *testing.T, w *walk) {
	w.next = w.base.Clone()
	w.next.Masteries[4139] = 0
	if w.base.Masteries[4139] == 0 {
		t.Fatal("editing the clone changed the base")
	}
	w.next.Masteries[4139] = w.base.Masteries[4139]
}

// gemNames: gem names from the dataset's skills group become the ids a build holds.
func gemNames(t *testing.T, w *walk) {
	fireball, _ := w.sim.Data().Gem("Fireball")
	inspiration, _ := w.sim.Data().Gem("Inspiration")

	if fireball.ID != gemFireball || inspiration.ID != "Metadata/Items/Gems/SupportGemReducedMana" {
		t.Fatalf("Fireball %+v, Inspiration %+v", fireball, inspiration)
	}
	w.fireball, w.inspiration = fireball, inspiration
}

// editMainGroup: the imported file has no skill groups, so the step first adds a Fireball group,
// the main skill of a build with one group.
func editMainGroup(t *testing.T, w *walk) {
	w.next.Skills.AddGroup(build.SkillGroup{Slot: build.SlotHelmet, Gems: []build.Gem{{GemID: w.fireball, Level: 20}}})

	group := w.next.Skills.Main()
	group.Gem(w.fireball).SetLevel(21).SetQuality(23)
	group.AddSupport(w.inspiration, 20, 20)

	if group.Err() != nil {
		t.Fatal(group.Err())
	}
	want := []build.Gem{{GemID: w.fireball, Level: 21, Quality: 23}, {GemID: w.inspiration, Level: 20, Quality: 20, Support: true}}
	if d := gocmp.Diff(want, w.next.Skills.Groups[0].Gems); d != "" {
		t.Errorf("main group gems (-want +got):\n%s", d)
	}
	data, err := w.next.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if d := gocmp.Diff(w.next.Skills, back.Skills); d != "" {
		t.Errorf("skills after a build file round trip (-want +got):\n%s", d)
	}
}

// nodeOf returns the passive node a line is credited to.
func nodeOf(l simulator.Line) (simulator.NodeSource, bool) {
	if l.Source.Bundle == nil {
		return simulator.NodeSource{}, false
	}
	n, ok := l.Source.Bundle.Source.(simulator.NodeSource)
	return n, ok
}

// marginalLife returns the life each node other than a mastery gives alone, largest first.
func marginalLife(t *testing.T, sim *simulator.Sim, b *build.Build) ([]uint32, map[uint32]float64) {
	ex, err := simulate(t, sim, b).Explain(poe1.Life, simulator.Marginal)
	if err != nil {
		t.Fatal(err)
	}
	loss := map[uint32]float64{}
	var nodes []uint32
	for _, l := range ex.Lines {
		if n, ok := nodeOf(l); ok && b.Masteries[n.ID] == 0 {
			loss[n.ID] = l.Value
			nodes = append(nodes, n.ID)
		}
	}
	slices.SortStableFunc(nodes, func(a, b uint32) int { return cmp.Compare(loss[b], loss[a]) })
	return nodes, loss
}

func deallocate(t *testing.T, w *walk) {
	nodes, _ := marginalLife(t, w.sim, w.base)
	if len(nodes) < 3 {
		t.Fatalf("marginal life names %d nodes, want at least 3", len(nodes))
	}
	w.cut = nodes[:3]
	if err := w.next.Apply(build.Deallocate(w.cut...)); err != nil {
		t.Fatal(err)
	}
	if len(w.next.Nodes) != len(w.base.Nodes)-3 {
		t.Errorf("%d nodes after deallocating 3 of %d", len(w.next.Nodes), len(w.base.Nodes))
	}
}

func equipItem(t *testing.T, w *walk) {
	item, warnings, err := w.sim.Data().Item(rawItemText)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the item: %v, %v", warnings, err)
	}
	if err := w.next.Apply(build.Equip(build.SlotHelmet, item)); err != nil {
		t.Fatal(err)
	}
}

func simulateNext(t *testing.T, w *walk) {
	w.stats = simulate(t, w.sim, w.next)
	if got, want := w.stats.Version(), w.res.Store.Manifest().Version; got != want {
		t.Errorf("simulated on %s, want %s", got, want)
	}
}

// simulateMapBoss sets a level 83 map boss for one call only; the sim keeps its own config.
func simulateMapBoss(t *testing.T, w *walk) {
	st := simulate(t, w.sim, w.next, simulator.Set(poe1.Enemy, poe1.EnemyBoss), simulator.Set(poe1.EnemyLevel, 83))
	if len(st.Warnings()) != len(w.stats.Warnings()) {
		t.Errorf("warnings %v, want those of the plain simulation %v", st.Warnings(), w.stats.Warnings())
	}
	if _, ok := w.sim.Config()[poe1.Enemy.Name()]; ok {
		t.Error("the call's enemy stayed on the sim")
	}
}

func unreconcilable(t *testing.T, w *walk) {
	b := w.base.Clone()
	b.Class = "Metadata/Characters/NoSuchClass"
	if _, err := w.sim.Simulate(b); !errors.Is(err, simulator.ErrUnreconcilable) {
		t.Errorf("unknown class: %v, want ErrUnreconcilable", err)
	}
}

// unknownGem: a gem the dataset lacks is dropped with a warning naming it.
func unknownGem(t *testing.T, w *walk) {
	b := w.next.Clone()
	b.Skills.Main().AddSupport(build.GemID{ID: "Metadata/Items/Gems/SkillGemSupportNothing"}, 20, 0)
	stats := simulate(t, w.sim, b)
	warnings := stats.Warnings()
	i := slices.IndexFunc(warnings, func(x build.Warning) bool { return x.Code == build.GemUnknown })
	if i < 0 || !strings.Contains(warnings[i].Message, "SkillGemSupportNothing") || !strings.Contains(warnings[i].Message, "dropped") {
		t.Errorf("warnings %v, want the unknown support dropped", warnings)
	}
	if slices.ContainsFunc(warnings, func(x build.Warning) bool { return x.Code == build.SkillsUnchecked }) {
		t.Errorf("warnings %v: the skills were not checked", warnings)
	}
}

func readOutput(t *testing.T, w *walk) {
	before, after := get(t, simulate(t, w.sim, w.base), poe1.Life), get(t, w.stats, poe1.Life)
	if after >= before {
		t.Errorf("life %v after removing three life nodes, %v before", after, before)
	}
}

// readWarnings: the warnings are stats the engine leaves out or applies unchecked, nodes the
// removal cut off the tree, and the skills not simulated yet; none names a removed node.
func readWarnings(t *testing.T, w *walk) {
	known := []build.Code{
		simulator.UnmodelledStat, simulator.UncheckedCondition, build.NodesDisconnected, build.SkillsUnchecked, simulator.SkillsNotSimulated,
	}
	unmodelled := 0
	for _, x := range w.stats.Warnings() {
		if !slices.Contains(known, x.Code) || x.Message == "" {
			t.Errorf("warning %v", x)
		}
		if x.Code == simulator.UnmodelledStat {
			unmodelled++
		}
		for _, n := range w.cut {
			if strings.Contains(x.Subject, fmt.Sprintf("node %d", n)) {
				t.Errorf("warning %v names removed node %d", x, n)
			}
		}
	}
	if unmodelled == 0 {
		t.Error("no unmodelled stat on a level 90 tree")
	}
	if !slices.ContainsFunc(w.stats.Warnings(), func(x build.Warning) bool { return x.Code == simulator.SkillsNotSimulated }) {
		t.Error("no warning that the skills are not simulated")
	}
}

// configOf returns the value key k resolved to and its source.
func configOf(st *simulator.Stats, k simulator.Named) (string, config.Source) {
	e, _ := st.Config(k)
	return fmt.Sprint(e.Value), e.Source
}

func readConfigSource(t *testing.T, w *walk) {
	// The sim's help-oak wins over the build's kill-all.
	if v, src := configOf(w.stats, poe1.Bandits); v != "help-oak" || src != config.FromSim {
		t.Errorf("bandits %s from %v, want help-oak from the sim", v, src)
	}
	if v, src := configOf(w.stats, poe1.Campaign); src != config.Default {
		t.Errorf("campaign %s from %v, want the default", v, src)
	}
}

func explainOutput(t *testing.T, w *walk) {
	ex, err := w.stats.Explain(poe1.Life, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, l := range ex.Lines {
		sum += l.Value
	}
	if math.Abs(sum-ex.Amount) > 1e-6*ex.Amount || len(ex.Lines) < 10 {
		t.Errorf("%d lines sum to %v, want %v", len(ex.Lines), sum, ex.Amount)
	}
}

func listedOutputs(t *testing.T, w *walk) {
	st := simulate(t, w.sim, w.next, simulator.Outputs(poe1.Life, poe1.Mana))
	if !slices.Equal(st.Keys(), []output.Key{poe1.Life, poe1.Mana}) {
		t.Errorf("computed %v, want life and mana", st.Keys())
	}
	if _, ok := st.Get(poe1.Strength); ok {
		t.Error("strength computed though not listed")
	}
	if got, want := get(t, st, poe1.Life), get(t, w.stats, poe1.Life); got != want {
		t.Errorf("life %v with the outputs listed, %v without", got, want)
	}
}

func clearSimValue(t *testing.T, w *walk) {
	if err := w.sim.Configure(simulator.Clear(poe1.Bandits.Name())); err != nil {
		t.Fatal(err)
	}
	// Path of Building's "None" is killing all three bandits.
	if v, src := configOf(simulate(t, w.sim, w.next, simulator.Outputs(poe1.Life)), poe1.Bandits); v != "kill-all" || src != config.FromBuild {
		t.Errorf("bandits %s from %v after clearing the sim's, want kill-all from the build", v, src)
	}
}

func compareBuilds(t *testing.T, w *walk) {
	diff, err := w.sim.Compare(w.base, w.next)
	if err != nil {
		t.Fatal(err)
	}
	if d := diff.Get(poe1.Life); !d.Has || d.Abs >= 0 || d.Abs != d.After-d.Before || d.Pct != 100*d.Abs/d.Before {
		t.Errorf("life change %+v", d)
	}
	changed := diff.Changed()
	if !slices.Contains(changed, poe1.Life) || slices.Contains(changed, poe1.MaximumFireResistance) {
		t.Errorf("changed %v, want life and not maximum fire resistance", changed)
	}
	if slices.ContainsFunc(changed, func(k output.Key) bool { return diff.Get(k).Abs == 0 }) {
		t.Errorf("changed %v lists an output with no change", changed)
	}
	want := []build.Edit{
		build.Deallocate(slices.Sorted(slices.Values(w.cut))...), build.AddSkillGroup(w.next.Skills.Groups[0]),
		build.Equip(build.SlotHelmet, w.next.Items[build.SlotHelmet]),
	}
	if d := gocmp.Diff(want, diff.Edits); d != "" {
		t.Errorf("edits (-want +got):\n%s", d)
	}
	compareWarnings(t, w, diff)
}

// compareWarnings: each side of a comparison warns as simulating its build alone does.
func compareWarnings(t *testing.T, w *walk, diff *simulator.Diff) {
	if d := gocmp.Diff(simulate(t, w.sim, w.base).Warnings(), diff.Before.Warnings()); d != "" {
		t.Errorf("before warnings (-simulate +compare):\n%s", d)
	}
	if d := gocmp.Diff(w.stats.Warnings(), diff.After.Warnings()); d != "" {
		t.Errorf("after warnings (-simulate +compare):\n%s", d)
	}
}

func diffAcrossLeagues(t *testing.T, w *walk) {
	old, err := w.reg.Resolve("poe1", "3.28")
	if err != nil {
		t.Fatal(err)
	}
	simOld, err := simulator.New(old.Store)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := simulate(t, simOld, w.base).Diff(simulate(t, w.sim, w.base))
	if err != nil {
		t.Fatal(err)
	}
	if diff.Before.Version() == diff.After.Version() || len(diff.Edits) != 0 {
		t.Errorf("diff of one build on %s and %s: %d edits", diff.Before.Version(), diff.After.Version(), len(diff.Edits))
	}
	if slices.ContainsFunc(diff.DataChanges, func(c simulator.NodeChange) bool { return !slices.Contains(w.base.Nodes, c.Node) || len(c.Fields) == 0 }) {
		t.Errorf("data changes %+v: one is not an allocated node or has no field", diff.DataChanges)
	}
}

// sweep removes each of the three nodes on its own, and allocates one node next to the tree:
// each removal loses what the marginal view says, the allocation gains what simulating the
// build with the node gives, and the results rank by life.
func sweep(t *testing.T, w *walk) {
	_, loss := marginalLife(t, w.sim, w.base)
	added := unallocatedNeighbour(t, w)
	variants := make([][]build.Edit, 0, 1+len(w.cut))
	variants = append(variants, []build.Edit{build.Allocate(added)})
	for _, n := range w.cut {
		variants = append(variants, []build.Edit{build.Deallocate(n)})
	}
	results, err := w.sim.Sweep(w.base, variants, simulator.Outputs(poe1.Life), simulator.RankBy(poe1.Life))
	if err != nil {
		t.Fatal(err)
	}
	withAdded := w.base.Clone()
	withAdded.Nodes = append(withAdded.Nodes, added)
	for i, r := range results {
		e, c := r.Edits[0], r.Changes[poe1.Life]
		want := c.Before - loss[e.Nodes[0]]
		if e.Op == build.OpAllocate {
			want = get(t, simulate(t, w.sim, withAdded, simulator.Outputs(poe1.Life)), poe1.Life)
		}
		if r.Err != nil || c.After != want {
			t.Errorf("%s %v: life %v (%v), want %v", e.Op, e.Nodes, c.After, r.Err, want)
		}
		if i > 0 && c.Abs > results[i-1].Changes[poe1.Life].Abs {
			t.Errorf("result %d ranks above a larger change", i)
		}
	}
}

// unallocatedNeighbour returns the first plain node next to an allocated one that the base
// build does not allocate.
func unallocatedNeighbour(t *testing.T, w *walk) uint32 {
	tree, err := w.res.Store.Tree("standard")
	if err != nil {
		t.Fatal(err)
	}
	byID := func(id uint32) *dataset.Node {
		i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if !ok {
			return nil
		}
		return &tree.Nodes[i]
	}
	for _, id := range slices.Sorted(slices.Values(w.base.Nodes)) {
		for _, nb := range byID(id).Neighbours {
			if n := byID(nb); n != nil && n.Kind == dataset.Normal && n.Ascendancy == "" && !slices.Contains(w.base.Nodes, nb) {
				return nb
			}
		}
	}
	t.Fatal("no unallocated plain node next to the tree")
	return 0
}

func sweepEach(t *testing.T, w *walk) {
	variants := [][]build.Edit{{build.Deallocate(w.cut[0])}, {build.SetLevel(80)}}
	seq, err := w.sim.SweepEach(w.base, variants, simulator.Outputs(poe1.Life))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for r := range seq {
		if r.Err != nil || r.Changes[poe1.Life].Abs >= 0 {
			t.Errorf("variant %d: %+v, want life to drop", r.Index, r)
		}
		seen++
	}
	if seen != len(variants) {
		t.Errorf("%d results, want %d", seen, len(variants))
	}
}

func secondSim(t *testing.T, w *walk) {
	if err := w.sim.Configure(simulator.Set(poe1.Campaign, poe1.CampaignActs6To10)); err != nil {
		t.Fatal(err)
	}
	cfg := w.sim.Config()
	opts := make([]simulator.Option, 0, len(cfg))
	for _, name := range cfg.Names() {
		opts = append(opts, simulator.SetRaw(name, cfg[name]))
	}
	sim2, err := simulator.New(w.res.Store, opts...)
	if err != nil {
		t.Fatal(err)
	}
	a, b := simulate(t, w.sim, w.next, simulator.Outputs(poe1.Life)), simulate(t, sim2, w.next, simulator.Outputs(poe1.Life))
	if get(t, a, poe1.Life) != get(t, b, poe1.Life) {
		t.Errorf("life %v on the second sim, %v on the first", get(t, b, poe1.Life), get(t, a, poe1.Life))
	}
	if v, src := configOf(b, poe1.Campaign); v != "acts-6-10" || src != config.FromSim {
		t.Errorf("campaign %s from %v on the second sim", v, src)
	}
}
