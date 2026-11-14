package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

func support(id string, stats ...resolve.StatValue) resolve.Support {
	return resolve.Support{Source: resolve.GemSource{Gem: 1, ID: id}, Stats: stats}
}

// More multipliers multiply exactly per PoB mod name and are rounded half up to hundredths:
// two 39% more Damage from supports give 1.93, not 1.9321. More FireDamage is another name,
// and the character's more Damage another mod store, each rounded alone.
func TestMoreRoundsPerName(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "support_multiple_projectile_damage_+%_final", Value: 39},
	)))
	sk := spell()
	sk.Supports = []resolve.Support{
		support("cascade", resolve.StatValue{Stat: "support_spell_cascade_damage_+%_final", Value: 39}),
		support("totem", resolve.StatValue{Stat: "support_totem_damage_+%_final", Value: 39}),
		support("ignite", resolve.StatValue{Stat: "support_chance_to_ignite_fire_damage_+%_final", Value: 39}),
	}
	setSkills(t, sh, sk)
	for k, want := range map[output.Key]float64{
		poe1.ColdDamageModifier: 100 * 1.93 * 1.39,
		poe1.FireDamageModifier: 100 * (1.93 * 1.39) * 1.39,
	} {
		if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
}

// Melee Physical Damage's 10% less and Pulverise's 15% less attack speed give 0.765, rounded to
// 0.77. With 10% increased the speed is 0.847, rounded to 0.85 (0.84 from 0.765), so the axe's
// 1.38 attacks per second give 1.173.
func TestMoreRoundsAttackSpeed(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "attack_speed_+%", Value: 10},
	)))
	c := poe1.CompileItem(axe())
	if err := sh.SetItems([]poe1.Item{{Slot: build.SlotWeapon, Name: "Axe", Key: "axe", Stats: c.Stats, Weapon: c.Weapon}}); err != nil {
		t.Fatal(err)
	}
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.Supports = []resolve.Support{
		support("melee physical", resolve.StatValue{Stat: "support_melee_physical_damage_attack_speed_+%_final", Value: -10}),
		support("pulverise", resolve.StatValue{Stat: "support_pulverise_attack_speed_+%_final", Value: -15}),
	}
	setSkills(t, sh, attack)
	if got, want := outputOf(t, sh, poe1.HitsPerSecond), 1.38*0.85; math.Abs(got-want) > 1e-9 {
		t.Errorf("hits per second: got %v, want %v", got, want)
	}
}

// The supports' cost multipliers are PoB's one more name of higher precision: their product is
// rounded down to four places. 115% and 130% multiply to 1.4949999... in floating point, which
// gives 1.4949, so a 1000 mana cost becomes 1494.
func TestMoreSupportMultiplierRoundsDown(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	sk := spell()
	sk.Costs = []dataset.Cost{{Type: "Mana", Amount: 1000}}
	sk.Supports = []resolve.Support{support("a"), support("b")}
	sk.Supports[0].CostMultiplier, sk.Supports[1].CostMultiplier = 115, 130
	setSkills(t, sh, sk)
	if got := outputOf(t, sh, poe1.ManaCost); got != 1494 {
		t.Errorf("mana cost: got %v, want 1494", got)
	}
}
