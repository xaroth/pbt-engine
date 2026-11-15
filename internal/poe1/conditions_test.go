package poe1_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
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
