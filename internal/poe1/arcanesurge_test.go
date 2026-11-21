package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// Arcane Surge grants 20% increased cast speed to every skill, scaled by its effect, while you
// have it: from the Arcane Surge support on any skill, from the config, or from the Hierophant's
// passive while you hit with a spell. The main skill is a spell cast once a second.
func TestArcaneSurge(t *testing.T) {
	t.Parallel()
	supported := spellWith(nil)
	supported.Source = resolve.GemSource{Group: 1, Gem: 0, ID: "other"}
	supported.Supports = []resolve.Support{{Source: resolve.GemSource{Group: 1, Gem: 1, ID: "SupportArcaneSurge"}, Stats: []resolve.StatValue{
		stat("support_arcane_surge_gain_buff_on_mana_use_threshold", 15), stat("support_arcane_surge_cast_speed_+%", 20),
		stat("support_arcane_surge_mana_regeneration_rate_+%", 30),
	}}}
	effect := stat("arcane_surge_effect_+%", 50)
	passive := stat("gain_arcane_surge_on_spell_hit_by_you_or_your_totems", 1)
	for _, c := range []struct {
		name   string
		tree   []resolve.StatValue
		on     bool
		skills []*resolve.Skill
		want   float64
	}{
		{"none", nil, false, []*resolve.Skill{spellWith(nil)}, 1},
		{"support on another skill", nil, false, []*resolve.Skill{spellWith(nil), supported}, 1.2},
		{"config, 50% increased effect", []resolve.StatValue{effect}, true, []*resolve.Skill{spellWith(nil)}, 1.3},
		{"passive, hitting with a spell", []resolve.StatValue{passive}, false, []*resolve.Skill{spellWith(nil)}, 1.2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var values config.Values
			must(t, config.Set(&values, poe1.ArcaneSurge, c.on))
			sh := openWith(t, []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.tree...)}, values)
			setSkills(t, sh, c.skills...)
			wantOutputs(t, sh, map[output.Key]float64{poe1.HitsPerSecond: c.want})
		})
	}
}

// The passive's Arcane Surge needs a spell that you hit with: a totem's spell does not count, as
// in PoB.
func TestArcaneSurgeNotFromTotems(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, stat("gain_arcane_surge_on_spell_hit_by_you_or_your_totems", 1))))
	for _, c := range []struct {
		name  string
		types []string
		want  float64
	}{{"self-cast", nil, 1}, {"totem", []string{"SummonsTotem"}, 0}} {
		setSkills(t, sh, spellWith(c.types))
		if got := value(t, sh, "has_arcane_surge"); got != c.want {
			t.Errorf("%s: has Arcane Surge %v, want %v", c.name, got, c.want)
		}
	}
}

// Attack and cast speed with skills of a kind applies to skills of its types: Elemental to
// Fire, Cold or Lightning, Cold and Chaos to their own, Curse Skills' cast speed to hexes and
// marks. The spell is cast once a second.
func TestSkillTypeSpeed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		stat  string
		types []string
		want  float64
	}{
		{"attack_and_cast_speed_+%_with_elemental_skills", []string{"Fire"}, 1.1},
		{"attack_and_cast_speed_+%_with_elemental_skills", []string{"Lightning"}, 1.1},
		{"attack_and_cast_speed_+%_with_elemental_skills", []string{"Chaos"}, 1},
		{"attack_and_cast_speed_+%_with_cold_skills", []string{"Cold"}, 1.1},
		{"attack_and_cast_speed_+%_with_cold_skills", []string{"Fire"}, 1},
		{"attack_and_cast_speed_+%_with_chaos_skills", []string{"Chaos"}, 1.1},
		{"attack_and_cast_speed_+%_with_chaos_skills", []string{"Fire"}, 1},
		{"curse_cast_speed_+%", []string{"Mark"}, 1.1},
		{"curse_cast_speed_+%", []string{"Fire"}, 1},
	} {
		t.Run(c.stat+" "+c.types[0], func(t *testing.T) {
			t.Parallel()
			sk := spellWith(nil)
			sk.Types = append([]string{"Spell", "Damage"}, c.types...)
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, stat(c.stat, 10))))
			setSkills(t, sh, sk)
			wantOutputs(t, sh, map[output.Key]float64{poe1.HitsPerSecond: c.want})
		})
	}
}
