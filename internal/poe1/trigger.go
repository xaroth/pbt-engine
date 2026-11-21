package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Engine stats of a triggered skill's rate, read in its context. cooldown is the skill's in
// seconds, 0 for none. trigger_rate is the hits per second of the skill that triggers it and
// trigger_hit_chance that skill's chance to hit in percent, both 0 without one.
const (
	statCooldown         = "cooldown"
	statTriggerRate      = "trigger_rate"
	statTriggerHitChance = "trigger_hit_chance"
)

// triggerStats are the stats of trigger.go, in the order the hit mechanic registers them.
func triggerStats() []string {
	return []string{statCooldown, statTriggerRate, statTriggerHitChance}
}

// triggerSimulatedHits is how many hits of the triggering skill the trigger rate simulates, as
// PoB does (CalcTriggers.lua calcMultiSpellRotationImpact).
const triggerSimulatedHits = 1000

// triggeredRate returns the uses per second of a skill with cooldown seconds triggered by each
// hit of a skill hitting rate times per second with chance percent to hit. It follows PoB's
// simulation of one triggered skill (CalcTriggers.lua calcMultiSpellRotationImpact): over 1000
// hits, a hit triggers the skill when its cooldown is over, and the cooldown runs from the start
// of the server tick of the hit to the end of the tick it ends in. A chance to hit below 100%
// adds the expected wait for a hit that lands, as PoB's estimate does.
func triggeredRate(rate, chance, cooldown float64) float64 {
	if rate <= 0 || chance <= 0 {
		return 0
	}
	step := 1 / rate
	simTime := step * triggerSimulatedHits
	uses, ready := 0, 0.0
	for t := 0.0; t < simTime; t += step {
		if ready <= t {
			uses++
			ready = ceilTo(floorTo(t, serverTickSeconds)+cooldown, serverTickSeconds)
		}
	}
	return 1 / (simTime/float64(uses) + step/chance*percentScale - step)
}

// floorTo and ceilTo round x down and up to a multiple of base, as PoB's floor_b and ceil_b.
func floorTo(x, base float64) float64 { return base * math.Floor(x/base) }
func ceilTo(x, base float64) float64  { return base * math.Ceil(x/base) }

// triggerBases are sk's bases of the stats of trigger.go its granted effect gives: its
// cooldown.
func triggerBases(sk *resolve.Skill) []hitBase {
	return []hitBase{{statCooldown, core.OpBase, float64(sk.Cooldown) / millisecondsPerSecond}}
}

// supportTriggered reports whether sk is triggered by the skills it is granted for: a triggered
// skill a support grants, such as Void Shockwave.
func supportTriggered(sk *resolve.Skill) bool {
	return hasTypes(sk, typeTriggered) && len(sk.Supported) > 0
}

// triggerKey is a triggered skill's slot, the context its triggering skill's values are read
// in, and both skills.
type triggerKey struct {
	slot              int
	from              core.CtxID
	skill, triggering resolve.SkillRef
}

// setTriggers links each skill of the sheet a support grants and triggers to the first skill
// on the sheet it is granted for: its trigger_rate and trigger_hit_chance read that skill's
// hits per second and chance to hit. A link not seen yet gets its source added once; adding is
// refused while a checkpoint is open (ErrCheckpointOpen).
func (sh *Sheet) setTriggers() error {
	want := map[triggerKey]bool{}
	var links []triggerKey
	for i, sl := range sh.skills {
		if j := sh.triggerSlot(sl.skill); j >= 0 {
			k := triggerKey{
				slot: i, from: sh.session.InternContext(sh.ctx.skillMask(j, sh.skills[j].types, sh.wieldOf(j, readSkill), nil, formNone)),
				skill: skillRef(sl.skill), triggering: skillRef(sh.skills[j].skill),
			}
			want[k], links = true, append(links, k)
		}
	}
	for k, src := range sh.triggers {
		sh.setSources([]core.SourceID{src}, want[k])
	}
	for _, k := range links {
		if _, ok := sh.triggers[k]; ok {
			continue
		}
		if sh.marked {
			return fmt.Errorf("%w: trigger of skill %d", ErrCheckpointOpen, k.slot)
		}
		src, err := sh.addTrigger(k)
		if err != nil {
			return err
		}
		if sh.triggers == nil {
			sh.triggers = map[triggerKey]core.SourceID{}
		}
		sh.triggers[k] = src
	}
	return nil
}

func skillRef(sk *resolve.Skill) resolve.SkillRef {
	return resolve.SkillRef{Source: sk.Source, Which: sk.Which}
}

// triggerSlot returns the slot of the skill that triggers sk, -1 for none.
func (sh *Sheet) triggerSlot(sk *resolve.Skill) int {
	if !supportTriggered(sk) {
		return -1
	}
	for j, sl := range sh.skills {
		if slices.Contains(sk.Supported, skillRef(sl.skill)) {
			return j
		}
	}
	return -1
}

// addTrigger adds the source of link k.
func (sh *Sheet) addTrigger(k triggerKey) (core.SourceID, error) {
	src := sh.newSource(Source{
		RuleID: "skill-trigger",
		Rule:   fmt.Sprintf("%s triggered by %s", k.skill.Source, k.triggering.Source),
	})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, k.slot)
	if err != nil {
		return 0, err
	}
	for _, read := range [][2]string{{statTriggerRate, statHitsPerSecond}, {statTriggerHitChance, statHitChance}} {
		ref := core.Ref{Actor: core.Self, Stat: sh.stats[read[1]], Ctx: int64(k.from)}
		mod := core.Modifier{
			Stat: sh.stats[read[0]], Op: core.OpBase, Value: 1, Source: src, Pred: pred,
			Scales: []core.Scale{{Ref: ref, Step: 1, Ratio: true}},
		}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return 0, fmt.Errorf("sheet: skill trigger: %w", err)
		}
	}
	return src, nil
}
