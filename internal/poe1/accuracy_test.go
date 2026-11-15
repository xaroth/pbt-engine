package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// accuracySheet is a level 50 character with 100 Dexterity, +100 accuracy rating and 50%
// increased accuracy rating, attacking with a sword that deals 10 physical damage once a
// second with a 10% critical strike chance and adds 3 accuracy rating to its attacks.
func accuracySheet(t *testing.T, stats ...resolve.StatValue) *poe1.Sheet {
	t.Helper()
	sh := open(t, resolved(
		bundle(resolve.CharacterSource{ID: "c"}, resolve.Global,
			resolve.StatValue{Stat: "base_dexterity", Value: 100},
			resolve.StatValue{Stat: "accuracy_rating_per_level", Value: 2},
			resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 150},
		),
		bundle(resolve.LevelSource{}, resolve.Global, resolve.StatValue{Stat: "level", Value: 50}),
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, append(stats,
			resolve.StatValue{Stat: "accuracy_rating", Value: 100},
			resolve.StatValue{Stat: "accuracy_rating_+%", Value: 50},
		)...),
	))
	sword := &poe1.Weapon{AttackRate: 1, CriticalChance: 10, Accuracy: 3}
	sword.Damage[poe1.Physical] = poe1.DamageRange{Min: 10, Max: 10}
	if err := sh.SetItems([]poe1.Item{{Slot: build.SlotWeapon, Name: "Sword", Key: "sword", Weapon: sword}}); err != nil {
		t.Fatal(err)
	}
	sh.SetEnemy(poe1.EnemyStats{Evasion: 10143})
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.StatSets = []resolve.SkillStatSet{{}}
	setSkills(t, sh, attack)
	return sh
}

// Accuracy is 2 per level after the first, 2 per Dexterity, the sword's 3 and +100, 50%
// increased, rounded down: 401 x 1.5 = 601. Against 10,143 evasion it hits 601 / (601 +
// 2028.6^0.9) x 125% = 48.5%, rounded to 49%. A critical strike counts only on a hit, so 4.9%
// of uses strike critically; the average hit weighs those, and hit DPS counts the hits that
// land.
func TestAttackHitChance(t *testing.T) {
	t.Parallel()
	sh := accuracySheet(t)
	average := 10*(1-0.049) + 15*0.049
	for k, want := range map[output.Key]float64{
		poe1.AccuracyRating: 601, poe1.HitChance: 49, poe1.CriticalStrikeChance: 4.9,
		poe1.AverageHit: average, poe1.HitDPS: average * 0.49,
	} {
		if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	sh.SetEnemy(poe1.EnemyStats{Evasion: 1e9})
	if got := outputOf(t, sh, poe1.HitChance); got != 5 {
		t.Errorf("against high evasion: got %v, want the 5%% minimum", got)
	}
	sh.SetEnemy(poe1.EnemyStats{})
	if got := outputOf(t, sh, poe1.HitChance); got != 100 {
		t.Errorf("against no evasion: got %v, want 100", got)
	}
}

// Hits that can't be evaded always hit, from the passive or from the weapon; Resolute Technique
// also never deals critical strikes. A spell always hits.
func TestAlwaysHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		stats []resolve.StatValue
		crit  float64
	}{
		{"global", []resolve.StatValue{{Stat: "global_always_hit", Value: 1}}, 10},
		{"resolute technique", []resolve.StatValue{{Stat: "resolute_technique", Value: 1}}, 0},
	} {
		sh := accuracySheet(t, c.stats...)
		if got := outputOf(t, sh, poe1.HitChance); got != 100 {
			t.Errorf("%s: hit chance %v, want 100", c.name, got)
		}
		if got := outputOf(t, sh, poe1.CriticalStrikeChance); got != c.crit {
			t.Errorf("%s: critical strike chance %v, want %v", c.name, got, c.crit)
		}
	}
	sh := accuracySheet(t)
	unevadable := &poe1.Weapon{AttackRate: 1, CannotBeEvaded: true}
	if err := sh.SetItems([]poe1.Item{{Slot: build.SlotWeapon, Name: "Sword", Key: "sword", Weapon: unevadable}}); err != nil {
		t.Fatal(err)
	}
	if got := outputOf(t, sh, poe1.HitChance); got != 100 {
		t.Errorf("weapon whose hits can't be evaded: got %v, want 100", got)
	}
	setSkills(t, sh, spell())
	if got := outputOf(t, sh, poe1.HitChance); got != 100 {
		t.Errorf("spell: got %v, want 100", got)
	}
}
