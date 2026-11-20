package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/poe1"
)

// buffSkill is a reserving skill of granted effect id and type kind ("Aura" or "Herald") with
// stats as its stat set.
func buffSkill(group int, id, kind string, stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(group, "Spell", kind, "HasReservation")
	sk.GrantedEffect = id
	sk.Reservation = resolve.Reservation{ManaPercent: 5000}
	sk.StatSets = []resolve.SkillStatSet{{Stats: stats}}
	return sk
}

func purityOfElements(group int) *resolve.Skill {
	return buffSkill(group, "PurityOfElements", "Aura",
		resolve.StatValue{Stat: "base_resist_all_elements_%", Value: 20}, resolve.StatValue{Stat: "active_skill_base_radius_+", Value: 5})
}

// An aura's buff reaches the sheet scaled by its effect, rounded toward zero as PoB rounds it:
// +20% at 137% is +27%. It goes with the skill, and an aura that cannot affect you gives none.
func TestAuraBuffAffectsYou(t *testing.T) {
	t.Parallel()
	effect := bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "non_curse_aura_effect_+%", Value: 37})
	sh := open(t, resolved(effect))
	aura := purityOfElements(0)
	setSkills(t, sh, aura)
	if got := value(t, sh, "elemental_resistance"); got != 27 {
		t.Errorf("with the aura: got %v, want 27", got)
	}
	if got := value(t, sh, "affected_by_purity_of_elements"); got != 1 {
		t.Errorf("affected by it: got %v, want 1", got)
	}
	setSkills(t, sh, skill(0, "Spell"))
	if got := value(t, sh, "elemental_resistance"); got != 0 {
		t.Errorf("without the aura: got %v, want 0", got)
	}
	generous := purityOfElements(0)
	generous.Supports = []resolve.Support{{
		Source: resolve.GemSource{Gem: 1, ID: "generosity"}, Stats: []resolve.StatValue{{Stat: "aura_cannot_affect_self", Value: 1}},
	}}
	setSkills(t, sh, generous)
	if got := value(t, sh, "elemental_resistance"); got != 0 {
		t.Errorf("an aura that does not affect you: got %v, want 0", got)
	}
}

// Auras and heralds count for the stats that count them, and each aura grants the attack and
// cast speed "Auras from your Skills grant" gives, scaled by its effect: 3% at 150% is 4% each.
func TestAurasAndHeraldsAffectingYou(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "non_curse_aura_effect_+%", Value: 50},
		resolve.StatValue{Stat: "auras_grant_attack_and_cast_speed_+%_to_you_and_your_allies", Value: 3},
		resolve.StatValue{Stat: "damage_+%_per_your_aura_or_herald_skill_affecting_you", Value: 10},
		resolve.StatValue{Stat: "damage_+%_while_affected_by_a_herald", Value: 5},
	)))
	setSkills(t, sh, purityOfElements(0), buffSkill(1, "Clarity", "Aura"), buffSkill(2, "HeraldOfAsh", "Herald"))
	for stat, want := range map[string]float64{
		"aura_skills_affecting_you": 2, "herald_skills_affecting_you": 1, "cast_speed": 108, "damage": 135,
	} {
		if got := value(t, sh, stat); got != want {
			t.Errorf("%s: got %v, want %v", stat, got, want)
		}
	}
}

// A buff's effect rounds its own value before a count multiplies it: Herald of the Breach's 3%
// more damage per herald at 125% is 3%, so 6% more with two heralds, not 7%.
func TestBuffEffectRoundsBeforeCount(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "herald_effect_on_self_+%", Value: 25})))
	breach := buffSkill(0, "HeraldOfTheBreach", "Herald",
		resolve.StatValue{Stat: "skill_buff_grants_damage_+%_final_per_herald_skill_affecting_you", Value: 3})
	setSkills(t, sh, breach, buffSkill(1, "HeraldOfAsh", "Herald"))
	if got := value(t, sh, "damage"); got != 106 {
		t.Errorf("damage: got %v, want 106", got)
	}
}

// Watcher's Eye lines hold while their aura affects you. A cold spell that hits for 100 against
// 50% resistance, with a 50% chance to strike critically for 150%: Hatred's 10% cold
// penetration reaches every hit, Zealotry's 20% only critical strikes, and neither holds without
// its aura or with an aura that cannot affect you.
func TestStatsWhileAffectedByAura(t *testing.T) {
	t.Parallel()
	generous := buffSkill(1, "Hatred", "Aura")
	generous.Supports = []resolve.Support{{
		Source: resolve.GemSource{Gem: 1, ID: "generosity"}, Stats: []resolve.StatValue{{Stat: "aura_cannot_affect_self", Value: 1}},
	}}
	for _, c := range []struct {
		name             string
		aura             *resolve.Skill
		minimum, average float64
	}{
		{"no aura", nil, 50, 0.5*50 + 0.5*75},
		{"Hatred", buffSkill(1, "Hatred", "Aura"), 60, 0.5*60 + 0.5*90},
		{"Zealotry", buffSkill(1, "Zealotry", "Aura"), 50, 0.5*50 + 0.5*150*0.7},
		{"Hatred that cannot affect you", generous, 50, 0.5*50 + 0.5*75},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.ItemSource{Slot: "Jewel", Name: "Watcher's Eye"}, resolve.Global,
				resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 150},
				resolve.StatValue{Stat: "reduce_enemy_cold_resistance_%_while_affected_by_hatred", Value: 10},
				resolve.StatValue{Stat: "critical_strikes_penetrates_%_elemental_resistances_while_affected_by_zealotry", Value: 20},
			)))
			sk := spellOf(0, "cold")
			sk.StatSets[0].SpellCritChance = 5000
			skills := []*resolve.Skill{sk}
			if c.aura != nil {
				skills = append(skills, c.aura)
			}
			setSkills(t, sh, skills...)
			if got := outputOf(t, sh, poe1.ColdMinimum); math.Abs(got-c.minimum) > 1e-9 {
				t.Errorf("cold minimum: got %v, want %v", got, c.minimum)
			}
			if got := outputOf(t, sh, poe1.AverageHit); math.Abs(got-c.average) > 1e-9 {
				t.Errorf("average hit: got %v, want %v", got, c.average)
			}
		})
	}
}
