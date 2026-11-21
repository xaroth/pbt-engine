package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Engine stats of the buffs aura, herald, guard and golem skills grant you. buff_effect, read
// in a skill's context, is the effect of its buff on you in percent: 100 with its increases and
// more multipliers. aura_skills_affecting_you and herald_skills_affecting_you, read on the sheet,
// count the skills whose buffs affect you. aura_granted_attack_and_cast_speed is the increased
// attack and cast speed each of your aura skills grants beside its own stats. Each
// affected_by_ stat of skillBuffs is set while that skill's buff affects you, for stats that
// hold while affected by it.
const (
	statBuffEffect                = "buff_effect"
	statAuraSkillsAffectingYou    = "aura_skills_affecting_you"
	statHeraldSkillsAffectingYou  = "herald_skills_affecting_you"
	statAuraGrantedAttackCastRate = "aura_granted_attack_and_cast_speed"
)

// Skill types and a stat that decide whether a skill's buff affects you.
const (
	typeAura                 = "Aura"
	typeHerald               = "Herald"
	typeGuard                = "Guard"
	typeGolem                = "Golem"
	statAuraCannotAffectSelf = "aura_cannot_affect_self"
)

// statAffectedByGuard is set while the buff of a guard skill affects you.
const statAffectedByGuard = "affected_by_guard_skill"

// vaalMoltenShell is the guard skill whose buff you cannot have with another guard's.
const vaalMoltenShell = "VaalMoltenShell"

// baseBuffEffect is a buff's effect before modifiers: 100%.
const baseBuffEffect = 100

// skillBuff is the buff a skill grants you: the stats of its stat set that the buff carries,
// and the stat set while it affects you, empty when the game has none.
type skillBuff struct {
	stats    []string
	affected string
}

// buffRules are the buffs of the skills the rules model, by granted effect.
type buffRules map[string]skillBuff

// skillBuffs are the buffs of the aura, herald, guard and golem skills the rules model, by
// granted effect. The game data does not say which of a skill's stats its buff carries; these
// are the stats the skill's description gives as granted to you ("You and nearby allies gain",
// "Buff grants", "Golems grant"), not those of the skill's own area, duration or damage.
func skillBuffs() buffRules {
	added := func(kind string) []string {
		return []string{
			"attack_minimum_added_" + kind + "_damage", "attack_maximum_added_" + kind + "_damage",
			"spell_minimum_added_" + kind + "_damage", "spell_maximum_added_" + kind + "_damage",
		}
	}
	resist := func(kind string) []string {
		return []string{"base_" + kind + "_damage_resistance_%", "base_maximum_" + kind + "_damage_resistance_%"}
	}
	out := buffRules{
		"Anger":             {added("fire"), "affected_by_anger"},
		"Wrath":             {append(added("lightning"), "wrath_aura_spell_lightning_damage_+%_final"), "affected_by_wrath"},
		"Envy":              {added("chaos"), ""},
		"Hatred":            {[]string{"physical_damage_%_to_add_as_cold", "hatred_aura_cold_damage_+%_final"}, "affected_by_hatred"},
		"Grace":             {[]string{"base_evasion_rating", "grace_aura_evasion_rating_+%_final"}, "affected_by_grace"},
		"Determination":     {[]string{"base_physical_damage_reduction_rating", "determination_aura_armour_+%_final"}, "affected_by_determination"},
		"Discipline":        {[]string{"base_maximum_energy_shield", "energy_shield_recharge_rate_+%"}, "affected_by_discipline"},
		"Clarity":           {[]string{"base_mana_regeneration_rate_per_minute"}, "affected_by_clarity"},
		"Vitality":          {[]string{"base_life_regeneration_rate_per_minute"}, "affected_by_vitality"},
		"Haste":             {[]string{"attack_speed_+%_granted_from_skill", "cast_speed_+%_granted_from_skill", "base_movement_velocity_+%"}, "affected_by_haste"},
		"Precision":         {[]string{"accuracy_rating", "skill_buff_grants_critical_strike_chance_+%"}, "affected_by_precision"},
		"PurityOfElements":  {[]string{"base_resist_all_elements_%"}, "affected_by_purity_of_elements"},
		"PurityOfFire":      {resist("fire"), "affected_by_purity_of_fire"},
		"PurityOfIce":       {resist("cold"), "affected_by_purity_of_ice"},
		"PurityOfLightning": {resist("lightning"), "affected_by_purity_of_lightning"},
		"Zealotry": {[]string{
			"spell_damage_aura_spell_damage_+%_final", "spell_critical_strike_chance_+%",
			"create_consecrated_ground_on_hit_%_vs_rare_or_unique_enemy",
		}, "affected_by_spell_damage_aura"},
		"Malevolence":     {[]string{"delirium_aura_damage_over_time_+%_final", "delirium_skill_effect_duration_+%"}, "affected_by_delirium"},
		"HeraldOfAsh":     {[]string{"physical_damage_%_to_add_as_fire"}, "affected_by_herald_of_ash"},
		"HeraldOfIce":     {added("cold"), "affected_by_herald_of_ice"},
		"HeraldOfThunder": {added("lightning"), "affected_by_herald_of_thunder"},
		"HeraldOfPurity":  {[]string{"herald_of_purity_physical_damage_+%_final"}, "affected_by_herald_of_purity"},
		"HeraldOfAgony": {
			[]string{"skill_buff_grants_chance_to_poison_%", "herald_of_agony_poison_damage_+%_final"}, "affected_by_herald_of_agony",
		},
		"HeraldOfTheBreach": {
			[]string{"skill_buff_grants_damage_+%_final_per_herald_skill_affecting_you"}, "affected_by_herald_of_the_breach",
		},
	}
	addGuardAndGolemBuffs(out)
	return out
}

// addGuardAndGolemBuffs adds the buffs of the guard and golem skills to r.
func addGuardAndGolemBuffs(r buffRules) {
	guard := func(stats ...string) skillBuff { return skillBuff{stats, statAffectedByGuard} }
	shell := guard(
		"base_physical_damage_reduction_rating", "molten_shell_damage_absorbed_%",
		"molten_shell_damage_absorb_limit_%_of_armour", "molten_shell_max_damage_absorbed",
	)
	quickGuard := guard("quick_guard_damage_absorbed_%", "quick_guard_damage_absorb_limit")
	golem := func(stats ...string) skillBuff { return skillBuff{stats: stats} }
	r["MoltenShell"], r["VaalMoltenShell"] = shell, shell
	r["QuickGuard"], r["BoneArmour"] = quickGuard, quickGuard
	r["ImmortalCall"] = guard("mortal_call_elemental_damage_taken_+%_final", "mortal_call_physical_damage_taken_+%_final")
	r["SummonFlameGolem"] = golem("fire_golem_grants_damage_+%", "fire_golem_grants_area_of_effect_+%")
	r["SummonIceGolem"] = golem("ice_golem_grants_critical_strike_chance_+%", "ice_golem_grants_accuracy_rating_+")
	r["SummonLightningGolem"] = golem(
		"lightning_golem_grants_attack_and_cast_speed_+%", "lightning_golem_grants_base_mana_regeneration_rate_per_minute",
	)
	r["SummonChaosGolem"] = golem("chaos_golem_grants_dot_multiplier_+", "chaos_golem_grants_chaos_resistance_%")
	r["SummonStoneGolem"] = golem("stone_golem_grants_base_life_regeneration_rate_per_minute", "stone_golem_grants_defences_+%")
}

// affectedStats are the affected_by_ stats of skillBuffs, sorted, each once.
func affectedStats() []string {
	var out []string
	for _, b := range skillBuffs() {
		if b.affected != "" {
			out = append(out, b.affected)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func buffMechanic() mechanic {
	flags := affectedStats()
	stats := append([]string{
		statBuffEffect, statAuraSkillsAffectingYou, statHeraldSkillsAffectingYou, statAuraGrantedAttackCastRate,
	}, flags...)
	return mechanic{name: "buffs", stats: stats, def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
		if _, ok := slices.BinarySearch(flags, def.Name); ok {
			def.Flag = true
		}
		return def
	}}
}

// of returns the buff sk grants you while it is active, false for a skill whose buff the
// rules do not model or does not affect you: an aura or herald that does not reserve, and a
// guard skill a totem casts, which buffs the totem.
func (r buffRules) of(sk *resolve.Skill) (skillBuff, bool) {
	b, ok := r[sk.GrantedEffect]
	switch {
	case !ok:
		return skillBuff{}, false
	case slices.Contains(sk.Types, typeGolem):
		return b, true
	case slices.Contains(sk.Types, typeGuard):
		return b, !slices.Contains(sk.Types, typeSummonsTotem)
	case !Reserves(sk) || !slices.ContainsFunc(sk.Types, func(t string) bool { return t == typeAura || t == typeHerald }):
		return skillBuff{}, false
	}
	if statOf(sk.Set().Stats, statAuraCannotAffectSelf) != 0 {
		return skillBuff{}, false
	}
	for _, sp := range sk.Supports {
		if statOf(sp.Stats, statAuraCannotAffectSelf) != 0 {
			return skillBuff{}, false
		}
	}
	return b, true
}

func statOf(stats []resolve.StatValue, name string) float64 {
	for _, sv := range stats {
		if sv.Stat == name {
			return sv.Value
		}
	}
	return 0
}

// splitBuff returns the stats of sk's stat set that are its own and those its buff b carries.
func splitBuff(stats []resolve.StatValue, b skillBuff) (own, buff []resolve.StatValue) {
	for _, sv := range stats {
		if slices.Contains(b.stats, sv.Stat) {
			buff = append(buff, sv)
		} else {
			own = append(own, sv)
		}
	}
	return own, buff
}

// carried is how the stats a buff or curse carries apply: scaled by the effect at effect, and
// held while every one of holds does.
type carried struct {
	effect core.Ref
	holds  []core.Cond
}

// apply scales mod, a modifier of engine stat, by the effect and holds it while c holds. The
// effect rounds the modifier's own value, before any count multiplies it, as PoB's ScaleAddMod
// does.
func (c *carried) apply(mod *core.Modifier, stat string) {
	mod.Scales = append([]core.Scale{buffScale(c.effect, stat, mod.Op, mod.Value)}, mod.Scales...)
	mod.Conds = append(mod.Conds, c.holds...)
}

// criticalChancePlaces are the decimal places a scaled base critical strike chance keeps.
const criticalChancePlaces = 2

// buffScale scales a buff's modifier of value v, of op on engine stat, by the effect at ref,
// rounded as PoB rounds a scaled modifier (ModStore.lua ScaleAddMod): a whole value toward
// zero, another down to one decimal place, and a base critical strike chance down to two
// (Data.lua highPrecisionMods). PoB rounds to two places first, which a whole product of whole
// values does not need.
func buffScale(ref core.Ref, stat string, op core.Op, v float64) core.Scale {
	sc := core.Scale{Ref: ref, Step: baseBuffEffect, Ratio: true, Round: core.RoundTrunc}
	switch {
	case stat == statCriticalStrikeChance && op == core.OpBase:
		sc.Round, sc.Places = core.RoundFloor, criticalChancePlaces
	case v != math.Trunc(v):
		sc.Round, sc.Places = core.RoundFloor, 1
	}
	return sc
}

// addBuff adds the buff b of sk, in slot with context ctx, from its buff stats: those stats
// apply everywhere, scaled by the skill's buff effect, and the rules set its affected_by_ stat,
// count an aura or herald and, for an aura, give the attack and cast speed auras grant. It
// returns the sources of both.
func (sh *Sheet) addBuff(sk *resolve.Skill, b skillBuff, stats []resolve.StatValue, slot int, ctx core.CtxID) ([]core.SourceID, error) {
	effect := core.Ref{Actor: int32(playerActor), Stat: sh.stats[statBuffEffect], Ctx: int64(ctx)}
	bd := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: sk.Source, Stats: stats}
	buff, err := sh.addBundle(&bd, sh.table, sh.gameVersion, -1, &carried{effect: effect})
	if err != nil {
		return nil, err
	}
	src := sh.newSource(Source{RuleID: "skill-buff", Rule: "buff of " + sk.Source.String()})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return nil, err
	}
	aura := slices.Contains(sk.Types, typeAura)
	mods := []core.Modifier{{Stat: sh.stats[statBuffEffect], Op: core.OpBase, Value: baseBuffEffect, Source: src, Pred: pred}}
	switch {
	case aura:
		mods = append(mods, core.Modifier{Stat: sh.stats[statAuraSkillsAffectingYou], Op: core.OpBase, Value: 1, Source: src})
	case slices.Contains(sk.Types, typeHerald):
		mods = append(mods, core.Modifier{Stat: sh.stats[statHeraldSkillsAffectingYou], Op: core.OpBase, Value: 1, Source: src})
	}
	if b.affected != "" {
		mods = append(mods, core.Modifier{Stat: sh.stats[b.affected], Op: core.OpFlag, Value: 1, Source: src})
	}
	if aura {
		granted := core.Scale{Ref: core.SelfRef(sh.stats[statAuraGrantedAttackCastRate]), Step: 1}
		for _, stat := range []string{statAttackSpeed, statCastSpeed} {
			mods = append(mods, core.Modifier{
				Stat: sh.stats[stat], Op: core.OpInc, Value: 1, Source: src, Scales: []core.Scale{granted, buffScale(effect, stat, core.OpInc, 1)},
			})
		}
	}
	for _, m := range mods {
		if _, err := sh.session.AddModifier(m); err != nil {
			return nil, fmt.Errorf("sheet: buff of %s: %w", sk.Source, err)
		}
	}
	return []core.SourceID{buff, src}, nil
}

// buffList picks, of skills in order, those whose buffs affect you: of the skills granting the
// same buff the first, as buffs of the same skill do not stack, and of the guard skills one,
// whose buffs you have one at a time: Vaal Molten Shell if there is one, as PoB f9f4f3b does
// (CalcPerform.lua, guardSlots), else the first. In game the guard used last applies.
type buffList struct {
	rules buffRules
	seen  []string
	guard string
}

func newBuffList(rules buffRules, skills []*resolve.Skill) *buffList {
	l := &buffList{rules: rules}
	for _, sk := range skills {
		if _, ok := rules.of(sk); ok && slices.Contains(sk.Types, typeGuard) && (l.guard == "" || sk.GrantedEffect == vaalMoltenShell) {
			l.guard = sk.GrantedEffect
		}
	}
	return l
}

// take reports whether the buff of sk affects you, given the skills taken before it.
func (l *buffList) take(sk *resolve.Skill) bool {
	_, ok := l.rules.of(sk)
	if !ok || slices.Contains(l.seen, sk.GrantedEffect) || slices.Contains(sk.Types, typeGuard) && sk.GrantedEffect != l.guard {
		return false
	}
	l.seen = append(l.seen, sk.GrantedEffect)
	return true
}
