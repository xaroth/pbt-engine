package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

type hits map[poe1.DamageType][2]float64

// conversionSpell is a made-up spell without critical strikes that hits for the given base
// damage of each type, with its own stats besides.
func conversionSpell(base hits, own ...resolve.StatValue) *resolve.Skill {
	sk := skill(0, "Spell", "Damage")
	sk.CastTime = 1000
	stats := own
	for d, v := range base {
		stats = append(stats,
			resolve.StatValue{Stat: "spell_minimum_base_" + d.String() + "_damage", Value: v[0]},
			resolve.StatValue{Stat: "spell_maximum_base_" + d.String() + "_damage", Value: v[1]})
	}
	sk.StatSets = []resolve.SkillStatSet{{Stats: stats}}
	return sk
}

func stat(name string, value float64) resolve.StatValue {
	return resolve.StatValue{Stat: name, Value: value}
}

// checkHits checks the smallest and largest hit of every type before the enemy against want;
// a type want leaves out is 0.
func checkHits(t *testing.T, sh *poe1.Sheet, want hits) {
	t.Helper()
	for d := poe1.Physical; d <= poe1.Chaos; d++ {
		for i, name := range []string{"minimum_hit_damage", "maximum_hit_damage"} {
			got, err := sh.SkillValue(0, name, d)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-want[d][i]) > 1e-9 {
				t.Errorf("%s %s: got %v, want %v", d, name, got, want[d][i])
			}
		}
	}
}

// Conversion and gain on made-up spells, before the enemy.
func TestConversion(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		base   hits
		own    []resolve.StatValue
		global []resolve.StatValue
		want   hits
	}{
		{
			// Physical converts wholly to lightning, half of lightning to cold, and fire gains
			// 10% of cold. Each share takes the increases of every type it passes through:
			// lightning 100 x 130%, cold half of 100 x 160%, fire a tenth of half of 100 x 200%.
			name: "order and increases carrying over",
			base: hits{poe1.Physical: {100, 100}},
			own:  []resolve.StatValue{stat("skill_physical_damage_%_to_convert_to_lightning", 100)},
			global: []resolve.StatValue{
				stat("base_lightning_damage_%_to_convert_to_cold", 50), stat("cold_damage_%_to_add_as_fire", 10),
				stat("physical_damage_+%", 10), stat("lightning_damage_+%", 20), stat("cold_damage_+%", 30), stat("fire_damage_+%", 40),
			},
			want: hits{poe1.Lightning: {65, 65}, poe1.Cold: {80, 80}, poe1.Fire: {10, 10}},
		},
		{
			// A later type never converts to an earlier one.
			name:   "no conversion backwards",
			base:   hits{poe1.Fire: {100, 100}},
			global: []resolve.StatValue{stat("base_cold_damage_%_to_convert_to_fire", 50)},
			want:   hits{poe1.Fire: {100, 100}},
		},
		{
			// The skill's 120% is scaled to 100%, and other conversion does not apply.
			name: "skill conversion above 100%",
			base: hits{poe1.Physical: {100, 100}},
			own: []resolve.StatValue{
				stat("skill_physical_damage_%_to_convert_to_cold", 60), stat("skill_physical_damage_%_to_convert_to_fire", 60),
			},
			global: []resolve.StatValue{stat("base_physical_damage_%_to_convert_to_lightning", 30)},
			want:   hits{poe1.Cold: {50, 50}, poe1.Fire: {50, 50}},
		},
		{
			// The skill's 40% applies first; the other 120% is scaled to the 60% left.
			name: "other conversion above what the skill leaves",
			base: hits{poe1.Physical: {100, 100}},
			own:  []resolve.StatValue{stat("skill_physical_damage_%_to_convert_to_lightning", 40)},
			global: []resolve.StatValue{
				stat("base_physical_damage_%_to_convert_to_cold", 40), stat("base_physical_damage_%_to_convert_to_fire", 80),
			},
			want: hits{poe1.Lightning: {40, 40}, poe1.Cold: {20, 20}, poe1.Fire: {40, 40}},
		},
		{
			// Gain as extra is not limited and takes nothing from the source.
			name: "gain beside full conversion",
			base: hits{poe1.Physical: {100, 100}},
			global: []resolve.StatValue{
				stat("physical_damage_%_to_add_as_cold", 150), stat("base_physical_damage_%_to_convert_to_fire", 100),
			},
			want: hits{poe1.Cold: {150, 150}, poe1.Fire: {100, 100}},
		},
		{
			name:   "negative conversion counts as none",
			base:   hits{poe1.Physical: {100, 100}},
			global: []resolve.StatValue{stat("base_physical_damage_%_to_convert_to_fire", -20)},
			want:   hits{poe1.Physical: {100, 100}},
		},
		{
			name:   "elemental gained as chaos",
			base:   hits{poe1.Physical: {100, 100}, poe1.Fire: {100, 100}},
			global: []resolve.StatValue{stat("elemental_damage_%_to_add_as_chaos", 10)},
			want:   hits{poe1.Physical: {100, 100}, poe1.Fire: {100, 100}, poe1.Chaos: {10, 10}},
		},
		{
			name:   "non-chaos gained as chaos",
			base:   hits{poe1.Physical: {100, 100}, poe1.Fire: {100, 100}},
			global: []resolve.StatValue{stat("non_chaos_damage_to_add_as_chaos_damage_%", 10)},
			want:   hits{poe1.Physical: {100, 100}, poe1.Fire: {100, 100}, poe1.Chaos: {20, 20}},
		},
		{
			// Fire's own 100 takes its 50% increase, rounded on its own; the 50% of physical it
			// receives takes it too: round(150) + round(50% of 150).
			name:   "own damage beside converted damage",
			base:   hits{poe1.Physical: {100, 100}, poe1.Fire: {100, 100}},
			global: []resolve.StatValue{stat("base_physical_damage_%_to_convert_to_fire", 50), stat("fire_damage_+%", 50)},
			want:   hits{poe1.Physical: {50, 50}, poe1.Fire: {225, 225}},
		},
		{
			// Shares are rounded half up when both the smallest and largest are non-zero.
			name:   "shares rounded",
			base:   hits{poe1.Physical: {1, 3}},
			global: []resolve.StatValue{stat("physical_damage_%_to_add_as_fire", 50)},
			want:   hits{poe1.Physical: {1, 3}, poe1.Fire: {1, 2}},
		},
		{
			name:   "shares not rounded with a smallest of 0",
			base:   hits{poe1.Physical: {0, 3}},
			global: []resolve.StatValue{stat("physical_damage_%_to_add_as_fire", 50)},
			want:   hits{poe1.Physical: {0, 3}, poe1.Fire: {0, 1.5}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.global...)))
			setSkills(t, sh, conversionSpell(c.base, c.own...))
			checkHits(t, sh, c.want)
		})
	}
}

// Converted damage meets the resistance of the type it converts to, and the explanations
// credit the conversion's sources with what it takes from physical and gives to lightning.
func TestConvertedHitAgainstEnemy(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, stat("base_physical_damage_%_to_convert_to_lightning", 60))))
	setSkills(t, sh, conversionSpell(hits{poe1.Physical: {1000, 1000}}))
	sh.SetEnemy(poe1.EnemyStats{LightningResistance: 50})
	if got, want := outputOf(t, sh, poe1.AverageHit), 400+600*0.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("average hit: got %v, want %v", got, want)
	}
	for k, want := range map[output.Key]float64{poe1.PhysicalMinimum: -600, poe1.LightningMinimum: 600, poe1.AverageHit: 0} {
		ex := explainOf(t, sh, k, poe1.Proportional)
		sum, credited := 0.0, 0.0
		for _, l := range ex.Lines {
			sum += l.Value
			if l.Source.String() == "node 1" {
				credited += l.Value
			}
		}
		if !near(sum, ex.Amount) {
			t.Errorf("%s: lines sum to %v, amount %v", k, sum, ex.Amount)
		}
		if !near(credited, want) {
			t.Errorf("%s: node 1 credited %v, want %v", k, credited, want)
		}
	}
}
