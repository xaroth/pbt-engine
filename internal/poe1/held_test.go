package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// setHands puts h in sh's hands.
func setHands(t *testing.T, sh *poe1.Sheet, h poe1.Hands) {
	t.Helper()
	if err := sh.SetHands(h); err != nil {
		t.Fatal(err)
	}
}

// staff compiles a made-up staff with sockets.
func staff(sockets string) *poe1.Weapon {
	base := &dataset.ItemBase{Class: "Staff", Weapon: &dataset.WeaponValues{
		PhysicalDamage: dataset.ValueRange{Min: 10, Max: 20}, AttackTime: 800, CriticalChance: 700,
	}}
	return poe1.CompileItem(&build.Item{Name: "Staff", Sockets: sockets}, base).Weapon
}

// "+8% Elemental Resistances while holding a Shield" and "+5% chance to Block Spell Damage
// while holding a Staff or Shield" count with a shield in the off hand, as its compiled item
// puts it there, and not with an empty off hand or a weapon in it. At the Kitava penalty fire
// resistance is -60.
func TestWhileHoldingShield(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(endgame(
		resolve.StatValue{Stat: "resist_all_elements_+%_while_holding_shield", Value: 8},
		resolve.StatValue{Stat: "spell_block_chance_%_while_holding_staff_or_shield", Value: 5},
	)...))
	shield := poe1.CompileItem(&build.Item{Name: "Shield"}, &dataset.ItemBase{Class: "Shield"})
	held := poe1.HandsOf([]poe1.Item{{Slot: build.SlotOffhand, Name: "Shield", Shield: shield.Shield}}, nil)
	claw := weapon("Claw", 10, 20, 666, 500)
	for _, tc := range []struct {
		name        string
		hands       poe1.Hands
		fire, block float64
	}{
		{"shield", held, -52, 5},
		{"empty off hand", poe1.Hands{MainHand: claw}, -60, 0},
		{"weapon in the off hand", poe1.Hands{MainHand: claw, OffHand: claw}, -60, 0},
		{"shield again", held, -52, 5},
	} {
		setHands(t, sh, tc.hands)
		if got := value(t, sh, "fire_resistance"); got != tc.fire {
			t.Errorf("%s: fire resistance %v, want %v", tc.name, got, tc.fire)
		}
		if got := value(t, sh, "spell_block_chance"); got != tc.block {
			t.Errorf("%s: spell block chance %v, want %v", tc.name, got, tc.block)
		}
	}
}

// "20% increased Defences while wielding a Staff" increases 100 armour to 120 with a staff in
// the main hand, and "+5% chance to Block Spell Damage while holding a Staff or Shield" counts
// once; neither counts with another weapon or unarmed.
func TestWhileWieldingStaff(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_physical_damage_reduction_rating", Value: 100},
		resolve.StatValue{Stat: "defences_+%_while_wielding_staff", Value: 20},
		resolve.StatValue{Stat: "spell_block_chance_%_while_holding_staff_or_shield", Value: 5},
	)))
	for _, tc := range []struct {
		name          string
		hands         poe1.Hands
		armour, block float64
	}{
		{"staff", poe1.Hands{MainHand: staff("")}, 120, 5},
		{"claw", poe1.Hands{MainHand: weapon("Claw", 10, 20, 666, 500)}, 100, 0},
		{"unarmed", poe1.Hands{}, 100, 0},
	} {
		setHands(t, sh, tc.hands)
		if got := value(t, sh, "armour"); got != tc.armour {
			t.Errorf("%s: armour %v, want %v", tc.name, got, tc.armour)
		}
		if got := value(t, sh, "spell_block_chance"); got != tc.block {
			t.Errorf("%s: spell block chance %v, want %v", tc.name, got, tc.block)
		}
	}
}

// The staff socket stats count the red and blue sockets of the wielded staff, white ones not:
// 2% increased maximum life per red socket, 3% increased maximum mana per blue socket and 10%
// increased maximum life and mana if it has a red and a blue socket. 100 life and 100 mana.
func TestStaffSockets(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_maximum_life", Value: 100},
		resolve.StatValue{Stat: "base_maximum_mana", Value: 100},
		resolve.StatValue{Stat: "maximum_life_+%_per_red_socket_on_staff", Value: 2},
		resolve.StatValue{Stat: "maximum_mana_+%_per_blue_socket_on_staff", Value: 3},
		resolve.StatValue{Stat: "maximum_life_and_mana_+%_if_red_and_blue_socket_on_staff", Value: 10},
	)))
	claw := weapon("Claw", 10, 20, 666, 500)
	claw.Sockets = "R-B-B"
	for _, tc := range []struct {
		name       string
		hands      poe1.Hands
		life, mana float64
	}{
		{"red and blue", poe1.Hands{MainHand: staff("R-R-B W")}, 114, 113},
		{"red only", poe1.Hands{MainHand: staff("R-W-G")}, 102, 100},
		{"blue only", poe1.Hands{MainHand: staff("B-B")}, 100, 106},
		{"not a staff", poe1.Hands{MainHand: claw}, 100, 100},
	} {
		setHands(t, sh, tc.hands)
		if got := value(t, sh, "maximum_life"); got != tc.life {
			t.Errorf("%s: maximum life %v, want %v", tc.name, got, tc.life)
		}
		if got := value(t, sh, "maximum_mana"); got != tc.mana {
			t.Errorf("%s: maximum mana %v, want %v", tc.name, got, tc.mana)
		}
	}
}
