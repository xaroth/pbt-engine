package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// A trap is thrown every 0.6 seconds at its throwing speed and the action speed, here 50%
// increased, 10% less and 10% more: 2.475 throws per second, each placing 3 traps. A mine is
// laid every 0.3 seconds, here 40% faster and slowed by a tenth for each of the 2 mines past the
// first, each used: 3 x 3.5 / 1.2. Cast speed applies to neither. A trap thrown faster than once
// per server tick still places 3 traps a tick.
func TestTrapAndMineHits(t *testing.T) {
	t.Parallel()
	tree := bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		stat("base_cast_speed_+%", 100), stat("action_speed_-%", -10),
		stat("trap_throwing_speed_+%", 50), stat("support_trap_and_mine_damage_trap_throwing_speed_+%_final", -10),
		stat("number_of_additional_traps_to_throw", 2),
		stat("mine_laying_speed_+%", 40), stat("number_of_additional_mines_to_place", 2),
	)
	sh := open(t, resolved(tree))
	for _, c := range []struct {
		name  string
		skill *resolve.Skill
		want  float64
	}{
		{"trap", spellWith([]string{"Trapped"}), 1 / 0.6 * 1.5 * 0.9 * 1.1 * 3},
		{"mine", spellWith([]string{"RemoteMined"}), 1 / 0.3 * 1.4 * 1.1 / 1.2 * 3},
		{"mine throwing traps", spellWith([]string{"RemoteMined", "Trapped"}), 1 / 0.3 * 1.4 * 1.1 / 1.2 * 3 * 3},
		{"trap at the cap", spellWith([]string{"Trapped"}, stat("trap_throwing_speed_+%", 5000)), 1 / 0.033 * 3},
	} {
		setSkills(t, sh, c.skill)
		average := outputOf(t, sh, poe1.AverageHit)
		wantOutputs(t, sh, map[output.Key]float64{poe1.HitsPerSecond: c.want, poe1.HitDPS: average * c.want})
	}
}

// A totem casts at the skill's cast speed without the character's action speed. It has the 1
// totem every character can have out and the 2 its support adds; each deals the hit DPS, and
// each adds 3% more damage.
func TestTotemHits(t *testing.T) {
	t.Parallel()
	tree := bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		stat("base_cast_speed_+%", 50), stat("action_speed_-%", -10), stat("base_number_of_totems_allowed", 1),
	)
	totem := spellWith([]string{"SummonsTotem"}, stat("base_number_of_totems_allowed", 2))
	sh := open(t, resolved(tree))
	setSkills(t, sh, totem)
	average := outputOf(t, sh, poe1.AverageHit)
	wantOutputs(t, sh, map[output.Key]float64{
		poe1.HitsPerSecond: 1.5, poe1.HitDPS: average * 1.5, poe1.TotemsHitDPS: average * 1.5 * 3,
	})

	sh = open(t, resolved(tree, bundle(resolve.NodeSource{ID: 2}, resolve.Global, stat("hierophant_passive_damage_+%_final_per_totem", 3))))
	setSkills(t, sh, totem)
	if got, want := outputOf(t, sh, poe1.AverageHit), average*1.09; got < want*0.999 || got > want*1.001 {
		t.Errorf("with 3%% more damage per totem: average hit %v, want about %v", got, want)
	}
}

// A second Ancestral Bond adds no totem.
func TestAncestralBondOnce(t *testing.T) {
	t.Parallel()
	totem := spellWith([]string{"SummonsTotem"})
	tree := bundle(resolve.NodeSource{ID: 1}, resolve.Global, stat("base_number_of_totems_allowed", 1), stat("keystone_ancestral_bond", 1))
	item := bundle(resolve.NodeSource{ID: 2}, resolve.Global, stat("keystone_ancestral_bond", 1))
	sh := open(t, resolved(tree, item))
	setSkills(t, sh, totem)
	if got, want := outputOf(t, sh, poe1.TotemsHitDPS), outputOf(t, sh, poe1.HitDPS)*2; got != want {
		t.Errorf("hit DPS of every totem %v, want 2 totems' %v", got, want)
	}
}
