package poe1_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// A spell that hits for 100 fire against the default enemy (50% fire resistance) deals 50
// before conditions. The Shock's effect is increased damage taken, at most 50%; Unnerve is
// 10% increased spell damage taken, Intimidate attack damage taken. A table stat holds only
// while its condition does.
func TestConditions(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	for _, c := range []struct {
		name   string
		config map[string]string
		stats  []resolve.StatValue
		want   float64
	}{
		{name: "no condition", want: 50},
		{name: "shocked", config: map[string]string{"enemy-shocked": "true", "enemy-shock-effect": "20"}, want: 60},
		{name: "shock effect above the maximum", config: map[string]string{"enemy-shocked": "true", "enemy-shock-effect": "80"}, want: 75},
		{name: "shock effect, not shocked", config: map[string]string{"enemy-shock-effect": "20"}, want: 50},
		{name: "unnerved", config: map[string]string{"enemy-unnerved": "true"}, want: 55},
		{name: "intimidated, on a spell", config: map[string]string{"enemy-intimidated": "true"}, want: 50},
		{
			name: "against chilled enemies", config: map[string]string{"enemy-chilled": "true"},
			stats: []resolve.StatValue{stat("damage_+%_vs_chilled_enemies", 100)}, want: 100,
		},
		{name: "against chilled enemies, not chilled", stats: []resolve.StatValue{stat("damage_+%_vs_chilled_enemies", 100)}, want: 50},
		{
			name: "taunted takes more", config: map[string]string{"enemy-taunted": "true"},
			stats: []resolve.StatValue{stat("taunted_enemies_damage_taken_+%", 10)}, want: 55,
		},
		{name: "against unique enemies, a pinnacle boss", stats: []resolve.StatValue{stat("slayer_damage_+%_final_against_unique_enemies", 20)}, want: 60},
		{
			name: "against unique enemies, a normal monster", config: map[string]string{"enemy": `"none"`},
			stats: []resolve.StatValue{stat("slayer_damage_+%_final_against_unique_enemies", 20)}, want: 50,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			values := config.Values{}
			for k, v := range c.config {
				values[k] = json.RawMessage(v)
			}
			sh := openWith(t, []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.stats...)}, values)
			setSkills(t, sh, elementalSpell())
			kind, _, err := config.Get(values, poe1.Enemy)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "" {
				sh.SetEnemy(poe1.EnemyStats{Kind: kind, FireResistance: 50})
			}
			if got := outputOf(t, sh, poe1.FireMinimum); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("fire minimum: got %v, want %v", got, c.want)
			}
		})
	}
}

// Fortified gives the character its maximum Fortification, which "while Fortified" stats read.
func TestFortified(t *testing.T) {
	t.Parallel()
	stats := []resolve.StatValue{{Stat: "base_max_fortification", Value: 20}}
	for _, c := range []struct {
		name      string
		fortified bool
		want      float64
	}{{"not fortified", false, 0}, {"fortified", true, 20}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var values config.Values
			if err := config.Set(&values, poe1.Fortified, c.fortified); err != nil {
				t.Fatal(err)
			}
			sh := openWith(t, []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, stats...)}, values)
			if got := value(t, sh, "fortification"); got != c.want {
				t.Errorf("fortification: got %v, want %v", got, c.want)
			}
		})
	}
}

// derive derives the conditions the build causes on sh, as a simulation does.
func derive(t *testing.T, sh *poe1.Sheet) {
	t.Helper()
	if _, _, err := sh.DeriveConditions(poe1.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
}

// spellOf is a made-up spell that hits for 100 of damage type name (fire, cold, lightning),
// once a second, with stats added to its stat set.
func spellOf(group int, name string, stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(group, "Spell", "Damage")
	sk.CastTime = 1000
	sk.StatSets = []resolve.SkillStatSet{{Stats: append([]resolve.StatValue{
		{Stat: "spell_minimum_base_" + name + "_damage", Value: 100}, {Stat: "spell_maximum_base_" + name + "_damage", Value: 100},
	}, stats...)}}
	return sk
}

// A condition the build can cause itself is on, derived, when a skill's hit inflicts it; an
// explicit value wins, off included; one set on that no skill inflicts applies with a warning.
// Each is measured by what holds while it does, on a hit of 100 against the default enemy's
// 50% resistance: a Shock of 20% effect, 100% increased damage with hits against Chilled or
// Ignited enemies.
func TestDeriveConditions(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	shocks := spellOf(0, "lightning", stat("base_chance_to_shock_%", 10))
	lightning := spellOf(0, "lightning")
	chills, fire := spellOf(0, "cold"), spellOf(0, "fire")
	ignites := spellOf(0, "fire", stat("base_chance_to_ignite_%", 10))
	shocksSecond := spellOf(1, "lightning", stat("base_chance_to_shock_%", 10))
	cannot := spellOf(0, "lightning", stat("base_chance_to_shock_%", 10), stat("cannot_inflict_status_ailments", 1))
	allChill := spellOf(0, "lightning", stat("all_damage_can_chill", 1))
	sks := func(s ...*resolve.Skill) []*resolve.Skill { return s }
	type want struct {
		on      bool
		src     config.Source
		warns   bool
		average float64
	}
	for _, c := range []struct {
		name   string
		key    config.Key[bool]
		set    string
		skills []*resolve.Skill
		read   output.Key
		want   want
	}{
		{"shocked, derived", poe1.EnemyShocked, "", sks(shocks), poe1.LightningMinimum, want{true, config.Derived, false, 60}},
		{"shocked, forced off", poe1.EnemyShocked, "false", sks(shocks), poe1.LightningMinimum, want{false, config.FromBuild, false, 50}},
		{"shocked, forced on without a source", poe1.EnemyShocked, "true", sks(lightning), poe1.LightningMinimum, want{true, config.FromBuild, true, 60}},
		{"shocked, no source", poe1.EnemyShocked, "", sks(lightning), poe1.LightningMinimum, want{false, config.Default, false, 50}},
		{"shocked by a critical strike", poe1.EnemyShocked, "", sks(spellCrit(lightning)), poe1.LightningMinimum, want{true, config.Derived, false, 60}},
		{"shocked by another skill", poe1.EnemyShocked, "", sks(lightning, shocksSecond), poe1.LightningMinimum, want{true, config.Derived, false, 60}},
		{"shocked, cannot inflict", poe1.EnemyShocked, "", sks(cannot), poe1.LightningMinimum, want{false, config.Default, false, 50}},
		{"chilled, derived", poe1.EnemyChilled, "", sks(chills), poe1.ColdMinimum, want{true, config.Derived, false, 100}},
		{"chilled, forced off", poe1.EnemyChilled, "false", sks(chills), poe1.ColdMinimum, want{false, config.FromBuild, false, 50}},
		{"chilled, forced on without a source", poe1.EnemyChilled, "true", sks(fire), poe1.FireMinimum, want{true, config.FromBuild, true, 100}},
		{"chilled, no source", poe1.EnemyChilled, "", sks(fire), poe1.FireMinimum, want{false, config.Default, false, 50}},
		{"chilled, all damage can chill", poe1.EnemyChilled, "", sks(allChill), poe1.LightningMinimum, want{true, config.Derived, false, 100}},
		{"ignited, derived", poe1.EnemyIgnited, "", sks(ignites), poe1.FireMinimum, want{true, config.Derived, false, 100}},
		{"ignited, forced off", poe1.EnemyIgnited, "false", sks(ignites), poe1.FireMinimum, want{false, config.FromBuild, false, 50}},
		{"ignited, forced on without a source", poe1.EnemyIgnited, "true", sks(fire), poe1.FireMinimum, want{true, config.FromBuild, true, 100}},
		{"ignited, no source", poe1.EnemyIgnited, "", sks(fire), poe1.FireMinimum, want{false, config.Default, false, 50}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			values := config.Values{"enemy-shock-effect": json.RawMessage(`20`)}
			if c.set != "" {
				values[c.key.Name()] = json.RawMessage(c.set)
			}
			cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: values})
			if len(problems) != 0 {
				t.Fatal(problems)
			}
			sh, err := poe1.Open(resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
				stat("damage_+%_vs_chilled_enemies", 100), stat("hit_damage_+%_vs_ignited_enemies", 100))), table(t), cfg)
			if err != nil {
				t.Fatal(err)
			}
			setSkills(t, sh, c.skills...)
			derived, warnings, err := sh.DeriveConditions(cfg)
			if err != nil {
				t.Fatal(err)
			}
			on, src := config.Lookup(derived, c.key)
			warns := slices.ContainsFunc(warnings, func(w build.Warning) bool {
				return w.Code == poe1.NoConditionSource && w.Subject == "config "+c.key.Name()
			})
			got := want{on, src, warns, outputOf(t, sh, c.read)}
			if got != c.want || len(warnings) > 1 {
				t.Errorf("got %+v, want %+v; warnings %v", got, c.want, warnings)
			}
		})
	}
}

// spellCrit returns sk with a 50% chance to strike critically.
func spellCrit(sk *resolve.Skill) *resolve.Skill {
	c := *sk
	c.StatSets = []resolve.SkillStatSet{sk.StatSets[0]}
	c.StatSets[0].SpellCritChance = 5000
	return &c
}
