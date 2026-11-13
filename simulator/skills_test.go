package simulator_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// Fireball with Added Fire Damage at 20% quality, whose quality gives 10% increased Fire
// Damage to the skill it supports: the main skill's fire part takes it, its cold part does not,
// and the sheet without the skill has no skill outputs.
func TestMainSkillOutputs(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	bare := marauderBuild(90)
	b := bare.Clone()
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{
		{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20},
		{ID: "Metadata/Items/Gems/SupportGemAddedFireDamage", Level: 20, Quality: 20, Support: true},
	}})
	st := simulate(t, s, b)
	sk, ok := st.MainSkill()
	if !ok || sk.ID != "Metadata/Items/Gems/SkillGemFireball" || !slices.Contains(sk.Types, "Spell") {
		t.Fatalf("main skill %+v, %v", sk, ok)
	}
	if got := get(t, st, poe1.FireDamageModifier); math.Abs(got-110) > 1e-9 {
		t.Errorf("fire damage modifier %v, want 110", got)
	}
	if got := get(t, st, poe1.ColdDamageModifier); got != 100 {
		t.Errorf("cold damage modifier %v, want 100", got)
	}
	if _, ok := simulate(t, s, bare).Get(poe1.FireDamageModifier); ok {
		t.Error("a build without skills has a fire damage modifier")
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Stats", data)
	ex, err := st.Explain(poe1.FireDamageModifier, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Skill == nil || ex.Skill.Group != 0 || ex.Skill.Gem != 0 {
		t.Errorf("explanation skill %+v, want group 0 gem 0", ex.Skill)
	}
	if !slices.ContainsFunc(ex.Lines, func(l simulator.Line) bool { return l.Value == 10 && l.Source.Bundle != nil }) {
		t.Errorf("lines %+v, want the support's 10", ex.Lines)
	}
	data, err = json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Explanation", data)
}

// Skills that reserve take their share of the pools: a percentage of the maximum rounded up, a
// flat amount, both after the supports' multipliers (level rows: Enlighten 10 at 64%,
// Arrogance 20 at 201%, Added Fire Damage 20 at 120%). Arrogance moves the reservation to life,
// Eternal Blessing removes it, and Autoexertion's support makes Seismic Cry reserve its 20%.
// The main skill's mana cost is its level row's 20 with the support's multiplier, rounded down.
func TestReservation(t *testing.T) {
	t.Parallel()
	const gems = "Metadata/Items/Gems/"
	s := realSim(t)
	gem := func(id string, level int) build.Gem { return build.Gem{ID: gems + id, Level: level} }
	sup := func(id string, level int) build.Gem { return build.Gem{ID: gems + id, Level: level, Support: true} }
	pct := func(pool, permyriad float64) float64 { return math.Ceil(pool * permyriad / 10000) }
	testCases := []struct {
		name   string
		groups [][]build.Gem
		// life and mana reserved from the pools, given the maximum.
		life, mana func(life, mana float64) float64
	}{
		{
			name:   "percentage",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20)}},
			mana:   func(_, m float64) float64 { return pct(m, 5000) },
		},
		{
			name:   "flat and a multiplier",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20), sup("SupportGemAdditionalXP", 10)}, {gem("SkillGemPrecision", 20)}},
			mana:   func(_, m float64) float64 { return 186 + pct(m, 5000*0.64) },
		},
		{
			name:   "life instead of mana",
			groups: [][]build.Gem{{gem("SkillGemDetermination", 20), sup("SupportGemBloodMagic", 20)}},
			life:   func(l, _ float64) float64 { return pct(l, 5000*2.01) },
		},
		{
			name:   "no reservation",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20), sup("SupportGemEternalBlessing", 20)}},
		},
		{
			name:   "a support's reservation",
			groups: [][]build.Gem{{gem("SkillGemSeismicCry", 20), gem("SkillGemCallToArms", 20)}},
			mana:   func(_, m float64) float64 { return pct(m, 2000) },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := marauderBuild(90)
			b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{gem("SkillGemFireball", 20), sup("SupportGemAddedFireDamage", 20)}})
			for _, g := range testCase.groups {
				b.Skills.AddGroup(build.SkillGroup{Gems: g})
			}
			st := simulate(t, s, b)
			life, mana := get(t, st, poe1.Life), get(t, st, poe1.Mana)
			var wantLife, wantMana float64
			if testCase.life != nil {
				wantLife = testCase.life(life, mana)
			}
			if testCase.mana != nil {
				wantMana = testCase.mana(life, mana)
			}
			for _, c := range []struct {
				key  output.Key
				want float64
			}{
				{poe1.ReservedLife, wantLife}, {poe1.UnreservedLife, life - wantLife},
				{poe1.ReservedMana, wantMana}, {poe1.UnreservedMana, mana - wantMana},
				{poe1.ManaCost, 24},
			} {
				if got := get(t, st, c.key); got != c.want {
					t.Errorf("%s %v, want %v", c.key, got, c.want)
				}
			}
			for _, k := range []output.Key{poe1.UnreservedMana, poe1.ManaCost} {
				if _, err := st.Explain(k, simulator.Proportional); err != nil {
					t.Errorf("explain %s: %v", k, err)
				}
			}
		})
	}
}
