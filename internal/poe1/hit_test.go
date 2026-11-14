package poe1_test

import (
	"maps"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

func outputOf(t *testing.T, sh *poe1.Sheet, k output.Key) float64 {
	t.Helper()
	v, err := sh.Output(k)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// spell is a made-up fire and cold spell: 100 to 200 base fire and 10 to 20 base cold damage,
// 5% critical strike chance, 1 second cast time, 150% damage effectiveness.
func spell() *resolve.Skill {
	sk := skill(0, "Spell", "Damage", "Fire")
	sk.CastTime = 1000
	sk.StatSets = []resolve.SkillStatSet{{
		SpellCritChance: 500, DamageEffectiveness: 5000,
		Stats: []resolve.StatValue{
			{Stat: "spell_minimum_base_fire_damage", Value: 100}, {Stat: "spell_maximum_base_fire_damage", Value: 200},
			{Stat: "spell_minimum_base_cold_damage", Value: 10}, {Stat: "spell_maximum_base_cold_damage", Value: 20},
		},
	}}
	return sk
}

// A spell's hit against the default enemy (50% fire and cold resistance): fire is (100 + 10 x
// 150%) to (200 + 20 x 150%) base and added, 50% increased, rounded half up: 173 to 345, of which the
// enemy takes half. Cold is 10 to 20, halved. With 5% chance of a 150% critical strike, the
// average hit is the average of each type, 129.5 and 7.5, times 0.95 + 0.05 x 1.5. Cast speed
// 125% gives 1.25 hits per second.
func TestSpellHit(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 150},
		resolve.StatValue{Stat: "spell_minimum_added_fire_damage", Value: 10},
		resolve.StatValue{Stat: "spell_maximum_added_fire_damage", Value: 20},
		resolve.StatValue{Stat: "fire_damage_+%", Value: 50},
		resolve.StatValue{Stat: "base_cast_speed_+%", Value: 25},
	)))
	setSkills(t, sh, spell())
	average := (129.5 + 7.5) * (0.95 + 0.05*1.5)
	for k, want := range map[output.Key]float64{
		poe1.FireMinimum: 86.5, poe1.FireMaximum: 172.5, poe1.ColdMinimum: 5, poe1.ColdMaximum: 10,
		poe1.PhysicalMinimum: 0, poe1.CriticalStrikeChance: 5, poe1.HitChance: 100,
		poe1.AverageHit: average, poe1.HitsPerSecond: 1.25, poe1.HitDPS: average * 1.25,
	} {
		if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	sh.SetEnemy(poe1.EnemyStats{FireResistance: -20})
	if got := outputOf(t, sh, poe1.FireMinimum); math.Abs(got-173*1.2) > 1e-9 {
		t.Errorf("fire minimum against -20%% fire resistance: got %v, want %v", got, 173*1.2)
	}
}

// Armour takes armour / (armour + 5 x hit) of a physical hit, at most 75%, a critical strike's
// share measured against the critical hit.
func TestPhysicalHitAgainstArmour(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 200},
		resolve.StatValue{Stat: "critical_strike_chance_+%", Value: 900},
	)))
	sk := skill(0, "Spell", "Damage", "Physical")
	sk.StatSets = []resolve.SkillStatSet{{SpellCritChance: 500, Stats: []resolve.StatValue{
		{Stat: "spell_minimum_base_physical_damage", Value: 1000}, {Stat: "spell_maximum_base_physical_damage", Value: 1000},
	}}}
	setSkills(t, sh, sk)
	if got := outputOf(t, sh, poe1.PhysicalMinimum); got != 250 {
		t.Errorf("against the default enemy's armour: got %v, want 250, capped at 75%% taken off", got)
	}
	sh.SetEnemy(poe1.EnemyStats{Armour: 5000})
	normal, critical := 1000*(1-5000.0/10000), 2000*(1-5000.0/15000)
	want := normal*0.5 + critical*0.5
	if got := outputOf(t, sh, poe1.AverageHit); math.Abs(got-want) > 1e-9 {
		t.Errorf("average hit against 5000 armour: got %v, want %v", got, want)
	}
}

// The sheet gives a hit's damage for a spell or an attack with one weapon, its rate of hits
// only when the character uses it itself, and its chance to hit and hit DPS for a spell only.
func TestHitOutputsGiven(t *testing.T) {
	t.Parallel()
	byKey := map[output.Key]poe1.Output{}
	for _, o := range poe1.Outputs() {
		byKey[o.Key] = o
	}
	sword := &poe1.Weapon{}
	oneWeapon, twoWeapons := poe1.Hands{MainHand: sword}, poe1.Hands{MainHand: sword, OffHand: sword}
	for _, c := range []struct {
		name                 string
		skill                *resolve.Skill
		hands                poe1.Hands
		hit, rate, spellRate bool
	}{
		{"none", nil, oneWeapon, false, false, false},
		{"spell", skill(0, "Spell", "Damage"), poe1.Hands{}, true, true, true},
		{"channelled spell", skill(0, "Spell", "Damage", "Channel"), poe1.Hands{}, true, false, false},
		{"spell of a mine", skill(0, "Spell", "Damage", "RemoteMined"), poe1.Hands{}, true, false, false},
		{"attack", skill(0, "Attack", "Damage"), oneWeapon, true, true, false},
		{"triggered attack", skill(0, "Attack", "Damage", "Triggered"), oneWeapon, true, false, false},
		{"attack of a totem", skill(0, "Attack", "Damage", "SummonsTotem"), oneWeapon, false, false, false},
		{"attack unarmed", skill(0, "Attack", "Damage"), poe1.Hands{}, false, false, false},
		{"attack dual wielding", skill(0, "Attack", "Damage"), twoWeapons, false, false, false},
		{"spell without a hit", skill(0, "Spell", "DamageOverTime"), poe1.Hands{}, false, false, false},
	} {
		for k, want := range map[output.Key]bool{
			poe1.AverageHit: c.hit, poe1.HitsPerSecond: c.rate, poe1.HitDPS: c.spellRate, poe1.Life: true,
		} {
			if got := poe1.Gives(byKey[k], c.skill, c.hands); got != want {
				t.Errorf("%s: %s given %v, want %v", c.name, k, got, want)
			}
		}
	}
}

// A modifier to damage with hits reaches the skill's hits, not its damage in general.
func TestHitDamageReachesHitsOnly(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	sk := spell()
	sk.Supports = []resolve.Support{{Source: resolve.GemSource{Gem: 1, ID: "support"}, Stats: []resolve.StatValue{
		{Stat: "support_cruelty_hit_damage_+%_final", Value: 100},
	}}}
	setSkills(t, sh, sk)
	if got := outputOf(t, sh, poe1.ColdMinimum); got != 10 {
		t.Errorf("cold minimum: got %v, want 10, 10 base doubled and halved by the enemy", got)
	}
	if got := outputOf(t, sh, poe1.ColdDamageModifier); got != 100 {
		t.Errorf("cold damage modifier: got %v, want 100", got)
	}
}

// elementalSpell is a made-up spell of the given extra types that hits for 100 fire, cold,
// lightning and chaos damage, without critical strikes.
func elementalSpell(types ...string) *resolve.Skill {
	sk := skill(0, append([]string{"Spell", "Damage"}, types...)...)
	sk.CastTime = 1000
	names := []string{"fire", "cold", "lightning", "chaos"}
	stats := make([]resolve.StatValue, 0, 2*len(names))
	for _, d := range names {
		stats = append(stats,
			resolve.StatValue{Stat: "spell_minimum_base_" + d + "_damage", Value: 100},
			resolve.StatValue{Stat: "spell_maximum_base_" + d + "_damage", Value: 100})
	}
	sk.StatSets = []resolve.SkillStatSet{{Stats: stats}}
	return sk
}

// Each penetration and Exposure stat against the default enemy (50% elemental and 30% chaos
// resistance), on a spell that hits for 100 of each type: the hit's minimum of each type is
// 100 less the resistance it meets. Exposure is the strongest one, with the extras only while
// there is one; mine and trap penetration reach only those skills, attack and weapon
// penetration no spell.
func TestPenetrationAndExposure(t *testing.T) {
	t.Parallel()
	keys := map[poe1.DamageType]output.Key{
		poe1.Fire: poe1.FireMinimum, poe1.Cold: poe1.ColdMinimum, poe1.Lightning: poe1.LightningMinimum, poe1.Chaos: poe1.ChaosMinimum,
	}
	type taken map[poe1.DamageType]float64
	none := taken{poe1.Fire: 50, poe1.Cold: 50, poe1.Lightning: 50, poe1.Chaos: 70}
	with := func(changes taken) taken {
		out := maps.Clone(none)
		maps.Copy(out, changes)
		return out
	}
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	for _, c := range []struct {
		name  string
		stats []resolve.StatValue
		types []string
		want  taken
	}{
		{"fire penetration", []resolve.StatValue{stat("base_reduce_enemy_fire_resistance_%", 20)}, nil, with(taken{poe1.Fire: 70})},
		{"negative fire penetration", []resolve.StatValue{stat("base_reduce_enemy_fire_resistance_%", -10)}, nil, with(taken{poe1.Fire: 40})},
		{"cold penetration", []resolve.StatValue{stat("base_reduce_enemy_cold_resistance_%", 20)}, nil, with(taken{poe1.Cold: 70})},
		{"lightning penetration", []resolve.StatValue{stat("base_reduce_enemy_lightning_resistance_%", 20)}, nil, with(taken{poe1.Lightning: 70})},
		{"chaos penetration", []resolve.StatValue{stat("reduce_enemy_chaos_resistance_%", 20)}, nil, with(taken{poe1.Chaos: 90})},
		{
			"elemental penetration", []resolve.StatValue{stat("reduce_enemy_elemental_resistance_%", 10)}, nil,
			with(taken{poe1.Fire: 60, poe1.Cold: 60, poe1.Lightning: 60}),
		},
		{
			"elemental penetration of enemy resistances", []resolve.StatValue{stat("base_penetrate_elemental_resistances_%", 10)}, nil,
			with(taken{poe1.Fire: 60, poe1.Cold: 60, poe1.Lightning: 60}),
		},
		{"attack penetration on a spell", []resolve.StatValue{stat("base_attack_damage_penetrates_elemental_resist_%", 10)}, nil, none},
		{"weapon penetration on a spell", []resolve.StatValue{stat("reduce_enemy_elemental_resistance_with_weapons_%", 10)}, nil, none},
		{"mine penetration on a spell", []resolve.StatValue{stat("mine_damage_penetrates_%_elemental_resistance", 10)}, nil, none},
		{
			"mine penetration on a mine", []resolve.StatValue{stat("mine_damage_penetrates_%_elemental_resistance", 10)}, []string{"RemoteMined"},
			with(taken{poe1.Fire: 60, poe1.Cold: 60, poe1.Lightning: 60}),
		},
		{
			"trap penetration on a trap", []resolve.StatValue{stat("trap_damage_penetrates_%_elemental_resistance", 10)}, []string{"Trapped"},
			with(taken{poe1.Fire: 60, poe1.Cold: 60, poe1.Lightning: 60}),
		},
		{"fire exposure", []resolve.StatValue{stat("fire_exposure_on_hit_magnitude", -15)}, nil, with(taken{poe1.Fire: 65})},
		{"cold exposure", []resolve.StatValue{stat("cold_exposure_on_hit_magnitude", -15)}, nil, with(taken{poe1.Cold: 65})},
		{"lightning exposure", []resolve.StatValue{stat("lightning_exposure_on_hit_magnitude", -15)}, nil, with(taken{poe1.Lightning: 65})},
		{
			"extra exposure", []resolve.StatValue{
				stat("fire_exposure_on_hit_magnitude", -15), stat("exposure_you_inflict_applies_extra_%_to_affected_resistance", -5),
			}, nil, with(taken{poe1.Fire: 70}),
		},
		{"extra exposure without exposure", []resolve.StatValue{stat("exposure_you_inflict_applies_extra_%_to_affected_resistance", -5)}, nil, none},
		{
			"extra fire exposure", []resolve.StatValue{
				stat("fire_exposure_on_hit_magnitude", -10), stat("cold_exposure_on_hit_magnitude", -10),
				stat("fire_exposure_you_inflict_applies_extra_fire_resistance_+%", -5),
			}, nil, with(taken{poe1.Fire: 65, poe1.Cold: 60}),
		},
		{
			"extra cold exposure", []resolve.StatValue{
				stat("cold_exposure_on_hit_magnitude", -10), stat("cold_exposure_you_inflict_applies_extra_cold_resistance_+%", -5),
			}, nil, with(taken{poe1.Cold: 65}),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.stats...)))
			setSkills(t, sh, elementalSpell(c.types...))
			for d, want := range c.want {
				if got := outputOf(t, sh, keys[d]); math.Abs(got-want) > 1e-9 {
					t.Errorf("%s: got %v, want %v", keys[d], got, want)
				}
			}
		})
	}
}

// Only the strongest Exposure of a type applies.
func TestStrongestExposureApplies(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "fire_exposure_on_hit_magnitude", Value: -12}),
		bundle(resolve.NodeSource{ID: 2}, resolve.Global, resolve.StatValue{Stat: "fire_exposure_on_hit_magnitude", Value: -15}),
	))
	setSkills(t, sh, elementalSpell())
	if got := outputOf(t, sh, poe1.FireMinimum); got != 65 {
		t.Errorf("fire minimum: got %v, want 65, against 50%% less the stronger 15%%", got)
	}
}

// The enemy's resistance with Exposure is at least -200% and at most its maximum: 75%, or the
// configured resistance up to 90%. Penetration counts after both limits.
func TestHitResistanceLimits(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_reduce_enemy_fire_resistance_%", Value: 10},
		resolve.StatValue{Stat: "cold_exposure_on_hit_magnitude", Value: -10},
	)))
	setSkills(t, sh, elementalSpell())
	for _, c := range []struct {
		name       string
		enemy      poe1.EnemyStats
		fire, cold float64
	}{
		{"floor", poe1.EnemyStats{FireResistance: -250, ColdResistance: -195}, 100 * 3.1, 100 * 3},
		{"cap", poe1.EnemyStats{FireResistance: 95, ColdResistance: 95}, 100 * 0.2, 100 * 0.15},
		{"maximum at a configured 80%", poe1.EnemyStats{FireResistance: 80, ColdResistance: 80}, 100 * 0.3, 100 * 0.3},
	} {
		sh.SetEnemy(c.enemy)
		if got := outputOf(t, sh, poe1.FireMinimum); math.Abs(got-c.fire) > 1e-9 {
			t.Errorf("%s: fire minimum: got %v, want %v", c.name, got, c.fire)
		}
		if got := outputOf(t, sh, poe1.ColdMinimum); math.Abs(got-c.cold) > 1e-9 {
			t.Errorf("%s: cold minimum: got %v, want %v", c.name, got, c.cold)
		}
	}
}

// Attack and weapon penetration reach an attack made with a weapon: the axe's 16 minimum
// lightning damage against 0% resistance meets -10%.
func TestAttackPenetration(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"base_attack_damage_penetrates_elemental_resist_%", "reduce_enemy_elemental_resistance_with_weapons_%",
		"reduce_enemy_lightning_resistance_with_weapons_%",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
				resolve.StatValue{Stat: "attack_minimum_added_lightning_damage", Value: 10},
				resolve.StatValue{Stat: "attack_maximum_added_lightning_damage", Value: 10},
				resolve.StatValue{Stat: name, Value: 10},
			)))
			c := poe1.CompileItem(axe())
			if err := sh.SetItems([]poe1.Item{{Slot: build.SlotWeapon, Name: "Axe", Key: "axe", Stats: c.Stats, Weapon: c.Weapon}}); err != nil {
				t.Fatal(err)
			}
			sh.SetEnemy(poe1.EnemyStats{})
			attack := skill(0, "Attack", "Damage", "Melee")
			attack.StatSets = []resolve.SkillStatSet{{DamageEffectiveness: 5000}}
			setSkills(t, sh, attack)
			if got := outputOf(t, sh, poe1.LightningMinimum); math.Abs(got-16*1.1) > 1e-9 {
				t.Errorf("lightning minimum: got %v, want %v", got, 16*1.1)
			}
		})
	}
}
