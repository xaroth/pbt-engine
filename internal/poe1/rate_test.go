package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// spellWith is spell() with types added and stats on its stat set.
func spellWith(types []string, stats ...resolve.StatValue) *resolve.Skill {
	sk := spell()
	sk.Types = append(sk.Types, types...)
	sk.StatSets[0].Stats = append(sk.StatSets[0].Stats, stats...)
	return sk
}

func wantOutputs(t *testing.T, sh *poe1.Sheet, want map[output.Key]float64) {
	t.Helper()
	for k, w := range want {
		if got := outputOf(t, sh, k); math.Abs(got-w) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, w)
		}
	}
}

// A brand hits each activation: every 0.5 seconds at 140% activation frequency, 15% from its
// own increase and 25% from cast speed, which the skill says applies to it, is 2.8 hits per
// second; cast speed's more multipliers do not apply. Each of the 2 brands attached to the enemy
// hits it, 1 of the skill's and 1 from Runebinder, under the 3 the character can have out.
func TestBrandHits(t *testing.T) {
	t.Parallel()
	tree := bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		stat("base_critical_strike_multiplier", 150), stat("base_cast_speed_+%", 25),
		stat("sigil_repeat_frequency_+%", 15), stat("keystone_runebinder", 1), stat("base_total_number_of_sigils_allowed", 3),
		stat("support_multicast_cast_speed_+%_final", 50),
	)
	brand := spellWith([]string{"Brand"},
		stat("base_sigil_repeat_frequency_ms", 500), stat("base_number_of_sigils_allowed_per_target", 1),
		stat("additive_cast_speed_modifiers_apply_to_sigil_repeat_frequency", 1),
	)
	sh := open(t, resolved(tree))
	setSkills(t, sh, brand)
	average := outputOf(t, sh, poe1.AverageHit)
	wantOutputs(t, sh, map[output.Key]float64{poe1.HitsPerSecond: 2.8, poe1.HitDPS: average * 2.8 * 2})

	// Without the skill saying so cast speed does not apply; Penance Brand hits every 20th
	// activation.
	brand = spellWith([]string{"Brand"}, stat("base_sigil_repeat_frequency_ms", 500), stat("base_number_of_sigils_allowed_per_target", 1))
	brand.GrantedEffect = "PenanceBrand"
	setSkills(t, sh, brand)
	wantOutputs(t, sh, map[output.Key]float64{poe1.HitsPerSecond: 1.15 / 0.5 / 20})
}

// A use is capped at one per server tick, one more for each repeat; a channelled skill is not
// capped.
func TestHitRateCap(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	for _, c := range []struct {
		name  string
		skill *resolve.Skill
		want  float64
	}{
		{"self-cast", spellWith(nil), 1 / 0.033},
		{"repeating", spellWith(nil, stat("base_spell_repeat_count", 1)), 2 / 0.033},
		{"channelled", spellWith([]string{"Channel"}), 100},
		{"channelled with a repeat", spellWith([]string{"Channel"}, stat("base_spell_repeat_count", 1)), 100},
	} {
		c.skill.CastTime = 10
		setSkills(t, sh, c.skill)
		if got := outputOf(t, sh, poe1.HitsPerSecond); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Winter Orb fires every 1.2 seconds, 20% faster per stage beyond the first and 80% more while
// channelled. Its gem's stages count up to the skill's maximum of 8, which it has when the gem
// gives none.
func TestStagesDefaultToTheMaximum(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	for _, c := range []struct {
		stages int
		want   float64
	}{
		{0, 2.4 * 1.8 / 1.2},
		{3, 1.4 * 1.8 / 1.2},
		{12, 2.4 * 1.8 / 1.2},
	} {
		orb := spellWith([]string{"Channel"},
			stat("frost_fury_base_fire_interval_ms", 1200), stat("frost_fury_max_number_of_stages", 8),
			stat("frost_fury_fire_speed_+%_per_stage", 20), stat("frost_fury_fire_speed_+%_final_while_channelling", 80),
		)
		orb.Stages = c.stages
		setSkills(t, sh, orb)
		if got := outputOf(t, sh, poe1.HitsPerSecond); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%d stages: got %v, want %v", c.stages, got, c.want)
		}
	}
}

// Repeats average their more damage over the use and its repeats: 20% more per repeat before
// grows to 40% on the second repeat, 20% on average over 2 repeats; 22% and 44% more on the
// first and second repeat average to 22% over the use and both. The two multiply, 1.464 rounded
// to hundredths as more multipliers are. A channelled skill does not repeat.
func TestRepeatDamageAverages(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	repeats := []resolve.StatValue{
		stat("base_spell_repeat_count", 2), stat("support_greater_spell_echo_spell_damage_+%_final_per_repeat", 20),
		stat("multistrike_damage_+%_final_on_first_repeat", 22), stat("multistrike_damage_+%_final_on_second_repeat", 44),
	}
	for _, c := range []struct {
		name  string
		types []string
		want  float64
	}{
		{"repeating", nil, 146},
		{"channelled", []string{"Channel"}, 100},
	} {
		setSkills(t, sh, spellWith(c.types, repeats...))
		if got := skillValue(t, sh, 0, poe1.Fire); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: damage got %v, want %v", c.name, got, c.want)
		}
	}
}
