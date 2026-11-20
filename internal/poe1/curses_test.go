package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// curseSkill is a curse of granted effect id and type kind ("Hex" or "Mark") with stats as its
// stat set.
func curseSkill(group int, id, kind string, stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(group, "Spell", "AppliesCurse", kind)
	sk.GrantedEffect = id
	sk.StatSets = []resolve.SkillStatSet{{Stats: stats}}
	return sk
}

func hexSkill(group int, id, stat string, v float64) *resolve.Skill {
	return curseSkill(group, id, "Hex", resolve.StatValue{Stat: stat, Value: v}, resolve.StatValue{Stat: "base_skill_effect_duration", Value: 9000})
}

func enemyResistance(t *testing.T, sh *poe1.Sheet, d poe1.DamageType) float64 {
	t.Helper()
	v, err := sh.SkillValue(0, "enemy_resistance", d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A curse changes the enemy by its stats scaled by its effect, rounded toward zero as PoB
// rounds it: Frostbite's -37% cold resistance at 110% is -40%, so the default enemy's 50%
// becomes 10%. Its own stats stay its own.
func TestCurseChangesTheEnemy(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "curse_effect_+%", Value: 10})))
	setSkills(t, sh, skill(0, "Spell", "Cold"), hexSkill(1, "Frostbite", "base_cold_damage_resistance_%", -37))
	if got := enemyResistance(t, sh, poe1.Cold); got != 10 {
		t.Errorf("cold resistance: got %v, want 10", got)
	}
	if got := value(t, sh, "enemy_is_cursed"); got != 1 {
		t.Errorf("enemy cursed: got %v, want 1", got)
	}
	if got := value(t, sh, "cold_resistance"); got != 0 {
		t.Errorf("the character's cold resistance: got %v, want 0", got)
	}
}

// Of more curses than the limit, those in earlier slots apply: with one curse Flammability
// applies and Elemental Weakness does not; with an additional curse both do.
func TestCurseLimit(t *testing.T) {
	t.Parallel()
	curses := []*resolve.Skill{
		skill(0, "Spell", "Fire"),
		hexSkill(1, "Flammability", "base_fire_damage_resistance_%", -30),
		hexSkill(2, "ElementalWeakness", "base_resist_all_elements_%", -20),
	}
	for _, c := range []struct {
		name            string
		additional      float64
		fire, lightning float64
	}{
		{"one curse", 0, 20, 50},
		{"two curses", 1, 0, 30},
	} {
		additional := resolve.StatValue{Stat: "number_of_additional_curses_allowed", Value: c.additional}
		sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, additional)))
		setSkills(t, sh, curses...)
		if got := enemyResistance(t, sh, poe1.Fire); got != c.fire {
			t.Errorf("%s: fire resistance: got %v, want %v", c.name, got, c.fire)
		}
		if got := enemyResistance(t, sh, poe1.Lightning); got != c.lightning {
			t.Errorf("%s: lightning resistance: got %v, want %v", c.name, got, c.lightning)
		}
	}
}

// The enemy has one mark: a second mark neither applies nor takes a place of the limit, so the
// hex after it applies. A mark's critical strike chance keeps two decimal places: 1.5% at 125%
// is 1.87%.
func TestOneMark(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "number_of_additional_curses_allowed", Value: 1},
		resolve.StatValue{Stat: "mark_skills_curse_effect_+%", Value: 25},
	)))
	setSkills(t, sh,
		skill(0, "Spell", "Fire"),
		curseSkill(1, "AssassinsMark", "Mark", resolve.StatValue{Stat: "enemy_additional_critical_strike_chance_against_self", Value: 150}),
		curseSkill(2, "SnipersMark", "Mark", resolve.StatValue{Stat: "projectile_damage_taken_+%", Value: 30}),
		hexSkill(3, "Flammability", "base_fire_damage_resistance_%", -30),
	)
	crit, err := sh.SkillValue(0, "critical_strike_chance")
	if err != nil {
		t.Fatal(err)
	}
	if crit != 1.87 {
		t.Errorf("critical strike chance: got %v, want 1.87", crit)
	}
	if got := enemyResistance(t, sh, poe1.Fire); got != 20 {
		t.Errorf("fire resistance: got %v, want 20", got)
	}
	if got := value(t, sh, "enemy_is_marked"); got != 1 {
		t.Errorf("enemy marked: got %v, want 1", got)
	}
}
