package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

const (
	witheringTouch = "Metadata/Items/Gems/SupportGemChaosAttacks"
	heavyStrike    = "Metadata/Items/Gems/SkillGemHeavyStrike"
	raiseZombie    = "Metadata/Items/Gems/SkillGemRaiseZombie"
	// witherEffectNode is a passive with 20% increased Effect of Withered.
	witherEffectNode = 26620
)

// witheredSheet opens the sheet of b with items on the real dataset of minionVersion, with
// stacks Withered stacks on the enemy, and switches and derives it as a simulation does.
func witheredSheet(t *testing.T, b *build.Build, items []poe1.Item, stacks int) *poe1.Sheet {
	t.Helper()
	b.Format, b.Game, b.Version, b.Class, b.Level = 1, "poe1", minionVersion, "Metadata/Characters/Str/Str", 90
	var v config.Values
	if err := config.Set(&v, poe1.EnemyWitheredStacks, poe1.WitheredStacks(stacks)); err != nil {
		t.Fatal(err)
	}
	cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	c, _, err := build.Check(b, realdata.Open(t, minionVersion))
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	sw, err := poe1.SwitchOf(c, cfg, items, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := open(t, r)
	if err := sh.Switch(&sw); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sh.DeriveConditions(cfg); err != nil {
		t.Fatal(err)
	}
	return sh
}

func group(gems ...string) build.SkillGroup {
	g := build.SkillGroup{}
	for i, id := range gems {
		g.Gems = append(g.Gems, build.Gem{ID: id, Level: 20, Support: i > 0})
	}
	return g
}

// Each Withered stack makes the enemy take 6% increased chaos damage, up to 15 stacks, while a
// skill of the build inflicts Withered; the effect of Withered raises it, rounded down as PoB
// does: 20% increased effect gives 7% a stack. Other damage types take none.
func TestWitheredChaosDamageTaken(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		gems          []string
		nodes         []uint32
		stacks        int
		chaos, others float64
	}{
		{name: "15 stacks", gems: []string{heavyStrike, witheringTouch}, stacks: 15, chaos: 190, others: 100},
		{name: "above 15", gems: []string{heavyStrike, witheringTouch}, stacks: 20, chaos: 190, others: 100},
		{name: "5 stacks", gems: []string{heavyStrike, witheringTouch}, stacks: 5, chaos: 130, others: 100},
		{name: "effect", gems: []string{heavyStrike, witheringTouch}, nodes: []uint32{witherEffectNode}, stacks: 15, chaos: 205, others: 100},
		{name: "no source", gems: []string{heavyStrike}, stacks: 15, chaos: 100, others: 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := build.Build{Nodes: c.nodes, Skills: build.Skills{Groups: []build.SkillGroup{group(c.gems...)}}}
			sh := witheredSheet(t, &b, nil, c.stacks)
			for d, want := range map[poe1.DamageType]float64{poe1.Chaos: c.chaos, poe1.Physical: c.others, poe1.Fire: c.others} {
				got, err := sh.SkillValue(0, "enemy_damage_taken", d)
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(got-want) > 1e-9 {
					t.Errorf("enemy damage taken of type %d %v, want %v", d, got, want)
				}
			}
		})
	}
}

// Withering Touch linked to a minion's gem makes the minion's hits inflict Withered, and the
// enemy the minion hits takes the increased chaos damage. Without the support it takes none.
func TestMinionWithers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		gems       []string
		can, chaos float64
	}{
		{"withering touch", []string{raiseZombie, witheringTouch}, 1, 190},
		{"none", []string{raiseZombie}, 0, 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := build.Build{Skills: build.Skills{Groups: []build.SkillGroup{group(c.gems...)}}}
			sh := witheredSheet(t, &b, nil, 15)
			can, err := sh.MinionAttackValue(0, "can_wither")
			if err != nil {
				t.Fatal(err)
			}
			if can != c.can {
				t.Errorf("minion can wither %v, want %v", can, c.can)
			}
			got, err := sh.MinionAttackValue(0, "enemy_damage_taken", poe1.Chaos)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.chaos {
				t.Errorf("enemy chaos damage taken from the minion's hit %v, want %v", got, c.chaos)
			}
		})
	}
}

// "Minions have +10% to Critical Strike Multiplier per Withered Debuff on Enemy" reads the
// configured stacks, at most 15, with or without a skill that inflicts Withered: a zombie's
// 130% from Monster.ot gains 10% a stack.
func TestMinionCriticalStrikeMultiplierPerWitheredStack(t *testing.T) {
	t.Parallel()
	item := poe1.Item{Slot: build.SlotRing1, Name: "Ring", Key: "ring", Stats: []resolve.StatValue{
		{Stat: "minions_have_+%_critical_strike_multiplier_per_wither_on_enemies", Value: 10},
	}}
	for stacks, want := range map[int]float64{0: 130, 4: 170, 15: 280, 20: 280} {
		b := build.Build{Skills: build.Skills{Groups: []build.SkillGroup{group(raiseZombie)}}}
		sh := witheredSheet(t, &b, []poe1.Item{item}, stacks)
		got, err := sh.MinionAttackValue(0, "critical_strike_multiplier")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%d stacks: minion critical strike multiplier %v, want %v", stacks, got, want)
		}
	}
}
