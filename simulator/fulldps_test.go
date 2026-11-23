package simulator_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// weaponHitDPS is the hit damage per second of one Animated Weapon of guardiansWeaponBuild
// (TestGuardiansWeaponAttacksWithTheSword).
const weaponHitDPS = 219.564 * 1.76

// fullDPSBuild is guardiansWeaponBuild with a Fireball group, the main skill, added as group 2
// and the Animate Guardian's Weapon group counted in full DPS with a count of 14.
func fullDPSBuild(t *testing.T, s *simulator.Sim) *build.Build {
	t.Helper()
	b := guardiansWeaponBuild(t, s, "")
	b.Skills.Groups = append(b.Skills.Groups, build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}})
	b.Skills.MainGroup = 2
	if err := b.Apply(build.SetFullDPS(1, true), build.SetGroupCount(1, 14)); err != nil {
		t.Fatal(err)
	}
	return b
}

func near(got, want float64) bool { return math.Abs(got-want) <= 1e-9*math.Abs(want) }

// Full DPS is 14 Animated Weapons' hit DPS; Fireball, the main skill, is not counted. The weapon
// group's result gives that part without the call asking for it, and the main skill's outputs
// do not change. Without a group counted there is no full DPS.
func TestFullDPSCountsMarkedGroups(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := fullDPSBuild(t, s)
	st := simulate(t, s, b, simulator.Set(poe1.EnemyArmour, 0))
	if got := get(t, st, poe1.TotalDPS); !near(got, 14*weaponHitDPS) {
		t.Errorf("full DPS %v, want %v", got, 14*weaponHitDPS)
	}
	if got, ok := st.Skill(simulator.SkillRef{Group: 1}).Get(poe1.TotalDPS); !ok || !near(got, 14*weaponHitDPS) {
		t.Errorf("weapon group's part %v (%v), want %v", got, ok, 14*weaponHitDPS)
	}
	if err := b.Apply(build.SetFullDPS(1, false)); err != nil {
		t.Fatal(err)
	}
	unmarked := simulate(t, s, b, simulator.Set(poe1.EnemyArmour, 0))
	if v, ok := unmarked.Get(poe1.TotalDPS); ok {
		t.Errorf("full DPS %v without a group counted, want none", v)
	}
	if got, want := get(t, st, poe1.HitDPS), get(t, unmarked, poe1.HitDPS); got != want {
		t.Errorf("Fireball hit DPS %v with the weapons counted, %v without", got, want)
	}
}

// Two Fireballs counted add their hits and the larger of their ignites.
func TestFullDPSTakesTheBestIgnite(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := marauderBuild(90)
	b.Skills.Groups = []build.SkillGroup{
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}, FullDPS: true},
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 10}}, FullDPS: true, Count: 2},
	}
	st := simulate(t, s, b, simulator.EverySkill())
	want := 0.0
	ignite := 0.0
	for i, count := range []float64{1, 2} {
		sk := st.Skill(simulator.SkillRef{Group: i})
		hit, okHit := sk.Get(poe1.HitDPS)
		ig, okIgnite := sk.Get(poe1.IgniteDPS)
		if !okHit || !okIgnite || ig <= 0 {
			t.Fatalf("group %d: hit DPS %v (%v), ignite DPS %v (%v)", i, hit, okHit, ig, okIgnite)
		}
		want += count * hit
		ignite = max(ignite, ig)
	}
	want += ignite
	if got := get(t, st, poe1.TotalDPS); !near(got, want) {
		t.Errorf("full DPS %v, want %v", got, want)
	}
}

// Compare gives the change of full DPS when the count changes, Sweep ranks variants by it.
func TestFullDPSCompareAndSweep(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := fullDPSBuild(t, s)
	next := b.Clone()
	if err := next.Apply(build.SetGroupCount(1, 7)); err != nil {
		t.Fatal(err)
	}
	diff, err := s.Compare(b, next, simulator.Set(poe1.EnemyArmour, 0))
	if err != nil {
		t.Fatal(err)
	}
	if d := diff.Get(poe1.TotalDPS); !d.Has || !near(d.Abs, -7*weaponHitDPS) {
		t.Errorf("full DPS change %+v, want %v", d, -7*weaponHitDPS)
	}
	results, err := s.Sweep(b, [][]build.Edit{{build.SetGroupCount(1, 7)}, {build.SetGroupCount(1, 10)}},
		simulator.RankBy(poe1.TotalDPS), simulator.Set(poe1.EnemyArmour, 0))
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range []int{10, 7} {
		r := results[i]
		if r.Err != nil || r.Edits[0].Count != n || !near(r.Changes[poe1.TotalDPS].After, float64(n)*weaponHitDPS) {
			t.Errorf("result %d: count %d, full DPS %+v (%v), want count %d", i, r.Edits[0].Count, r.Changes[poe1.TotalDPS], r.Err, n)
		}
	}
}

// The proportional view gives each group counted its part, the marginal view what full DPS loses
// without it, and the tree view reads each part's hit DPS.
func TestFullDPSExplains(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := fullDPSBuild(t, s)
	if err := b.Apply(build.SetFullDPS(0, true)); err != nil {
		t.Fatal(err)
	}
	st := simulate(t, s, b, simulator.Set(poe1.EnemyArmour, 0))
	total := get(t, st, poe1.TotalDPS)
	for _, v := range []simulator.View{simulator.Proportional, simulator.Marginal} {
		ex, err := st.Explain(poe1.TotalDPS, v)
		if err != nil {
			t.Fatal(err)
		}
		groups, sum := map[int]float64{}, 0.0
		for _, l := range ex.Lines {
			if src, ok := l.Source.Bundle.Source.(resolve.GemSource); ok {
				groups[src.Group] = l.Value
			}
			sum += l.Value
		}
		if len(ex.Lines) != 2 || !near(sum, total) || !near(groups[1], 14*weaponHitDPS) {
			t.Errorf("%s: lines %+v, want groups 0 and 1 summing to %v", v, ex.Lines, total)
		}
	}
	ex, err := st.Explain(poe1.TotalDPS, simulator.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Tree == nil || len(ex.Tree.Reads) != 2 || ex.Tree.Value != total {
		t.Errorf("tree %+v, want two reads and value %v", ex.Tree, total)
	}
}
