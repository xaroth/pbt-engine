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
	triggered := skill(0, "Attack", "Damage", "Melee", "Triggered", "Cooldown")
	triggered.Source.Gem, triggered.Which = 1, 1
	triggered.StatSets = []resolve.SkillStatSet{{}}
	triggered.Supported = []resolve.SkillRef{{Source: attack.Source}}

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
