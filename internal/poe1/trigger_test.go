package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// A skill a support grants is triggered by each hit of the skill it supports, the sword attack
// of accuracySheet. Without a cooldown it is used on each hit that lands: 2 hits a second at
// 100% increased attack speed, landing at 49%. At 25 hits a second that all land, its 0.5
// second cooldown, which ends at the end of the server tick it ends in, holds it to about once
// per 16 ticks, 0.528 seconds, not once per 0.5 seconds.
func TestTriggeredRate(t *testing.T) {
	t.Parallel()
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.StatSets = []resolve.SkillStatSet{{}}
	triggered := triggeredSkill(attack, "TriggeredSupportVoidShockwave")

	sh := accuracySheet(t, stat("attack_speed_+%", 100))
	setSkills(t, sh, triggered, attack)
	if got, want := outputOf(t, sh, poe1.HitsPerSecond), 2*0.49; math.Abs(got-want) > 1e-9 {
		t.Errorf("no cooldown: got %v hits per second, want %v", got, want)
	}

	sh = accuracySheet(t, stat("attack_speed_+%", 2400))
	sh.SetEnemy(poe1.EnemyStats{})
	triggered.Cooldown = 500
	setSkills(t, sh, triggered, attack)
	if got, want := outputOf(t, sh, poe1.HitsPerSecond), 1/0.528; math.Abs(got-want) > want/100 {
		t.Errorf("cooldown: got %v hits per second, want about %v", got, want)
	}
}

// Of the skills supports grant and trigger, only those triggered on each melee hit get a rate:
// Call the Pyre, which PoB triggers at 50% against ignited enemies, gets none, nor does Void
// Shockwave granted for a skill that is not melee.
func TestTriggeredRateExcluded(t *testing.T) {
	t.Parallel()
	melee := skill(0, "Attack", "Damage", "Melee")
	melee.StatSets = []resolve.SkillStatSet{{}}
	ranged := skill(0, "Attack", "Damage", "Projectile")
	ranged.StatSets = []resolve.SkillStatSet{{}}
	for name, c := range map[string]struct {
		triggering    *resolve.Skill
		grantedEffect string
	}{
		"call the pyre":       {melee, "TriggeredSupportPyre"},
		"not melee triggered": {ranged, "TriggeredSupportVoidShockwave"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sh := accuracySheet(t)
			setSkills(t, sh, triggeredSkill(c.triggering, c.grantedEffect), c.triggering)
			if got := outputOf(t, sh, poe1.HitsPerSecond); got != 0 {
				t.Errorf("got %v hits per second, want 0", got)
			}
		})
	}
}

// triggeredSkill is a melee attack with granted effect id, granted by a support for triggering.
func triggeredSkill(triggering *resolve.Skill, id string) *resolve.Skill {
	sk := skill(0, "Attack", "Damage", "Melee", "Triggered", "Cooldown")
	sk.GrantedEffect = id
	sk.Source.Gem, sk.Which = 1, 1
	sk.StatSets = []resolve.SkillStatSet{{}}
	sk.Supported = []resolve.SkillRef{{Source: triggering.Source}}
	return sk
}
