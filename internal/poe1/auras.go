package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Engine stats of the buffs aura and herald skills grant you. buff_effect, read in a skill's
// context, is the effect of its buff on you in percent: 100 with its increases and more
// multipliers. aura_skills_affecting_you and herald_skills_affecting_you, read on the sheet,
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
	statAuraCannotAffectSelf = "aura_cannot_affect_self"
)

// baseBuffEffect is a buff's effect before modifiers: 100%.
const baseBuffEffect = 100

// skillBuff is the buff an aura or herald skill grants you: the stats of its stat set that the
// buff carries, and the stat set while it affects you, empty when the game has none.
type skillBuff struct {
	stats    []string
	affected string
}

// skillBuffs are the buffs of the aura and herald skills the rules model, by granted effect.
// The game data does not say which of a skill's stats its buff carries; these are the stats
// the skill's description gives as granted to you ("You and nearby allies gain", "Buff
// grants"), not those of the skill's own area, duration or damage.
func skillBuffs() map[string]skillBuff {
	added := func(kind string) []string {
		return []string{
			"attack_minimum_added_" + kind + "_damage", "attack_maximum_added_" + kind + "_damage",
			"spell_minimum_added_" + kind + "_damage", "spell_maximum_added_" + kind + "_damage",
		}
	}
	resist := func(kind string) []string {
		return []string{"base_" + kind + "_damage_resistance_%", "base_maximum_" + kind + "_damage_resistance_%"}
	}
	return map[string]skillBuff{
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
}

// affectedStats are the affected_by_ stats of skillBuffs, sorted.
func affectedStats() []string {
	var out []string
	for _, b := range skillBuffs() {
		if b.affected != "" {
			out = append(out, b.affected)
		}
	}
	slices.Sort(out)
	return out
}

func buffMechanic() mechanic {
	stats := append([]string{
		statBuffEffect, statAuraSkillsAffectingYou, statHeraldSkillsAffectingYou, statAuraGrantedAttackCastRate,
	}, affectedStats()...)
	flags := affectedStats()
	return mechanic{name: "buffs", stats: stats, def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
		if _, ok := slices.BinarySearch(flags, def.Name); ok {
			def.Flag = true
		}
		return def
	}}
}

// buffOf returns the buff sk grants you while it is active, false for a skill whose buff the
// rules do not model, that does not reserve, or whose buff does not affect you.
func buffOf(sk *resolve.Skill) (skillBuff, bool) {
	b, ok := skillBuffs()[sk.GrantedEffect]
	if !ok || !Reserves(sk) || !slices.ContainsFunc(sk.Types, func(t string) bool { return t == typeAura || t == typeHerald }) {
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

// buffScale scales a buff's modifier of value v by the effect at ref, rounded as PoB rounds a
// scaled modifier (ModStore.lua ScaleAddMod): a whole value toward zero, another down to one
// decimal place. PoB rounds to two places first, which a whole product of whole values does
// not need.
func buffScale(ref core.Ref, v float64) core.Scale {
	sc := core.Scale{Ref: ref, Step: baseBuffEffect, Ratio: true, Round: core.RoundTrunc}
	if v != math.Trunc(v) {
		sc.Round, sc.Places = core.RoundFloor, 1
	}
	return sc
}

// addBuff adds the buff b of sk, in slot with context ctx, from its buff stats: those stats
// apply everywhere, scaled by the skill's buff effect, and the rules count the skill, set its
// affected_by_ stat and, for an aura, give the attack and cast speed auras grant. It returns
// the sources of both.
func (sh *Sheet) addBuff(sk *resolve.Skill, b skillBuff, stats []resolve.StatValue, slot int, ctx core.CtxID) ([]core.SourceID, error) {
	effect := core.Ref{Actor: 0, Stat: sh.stats[statBuffEffect], Ctx: int64(ctx)}
	bd := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: sk.Source, Stats: stats}
	buff, err := sh.addBundle(&bd, sh.table, sh.gameVersion, -1, &effect)
	if err != nil {
		return nil, err
	}
	src := sh.newSource(Source{RuleID: "skill-buff", Rule: "buff of " + sk.Source.String()})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return nil, err
	}
	aura := slices.Contains(sk.Types, typeAura)
	count := statHeraldSkillsAffectingYou
	if aura {
		count = statAuraSkillsAffectingYou
	}
	mods := []core.Modifier{
		{Stat: sh.stats[statBuffEffect], Op: core.OpBase, Value: baseBuffEffect, Source: src, Pred: pred},
		{Stat: sh.stats[count], Op: core.OpBase, Value: 1, Source: src},
	}
	if b.affected != "" {
		mods = append(mods, core.Modifier{Stat: sh.stats[b.affected], Op: core.OpFlag, Value: 1, Source: src})
	}
	if aura {
		granted := core.Scale{Ref: core.SelfRef(sh.stats[statAuraGrantedAttackCastRate]), Step: 1}
		for _, stat := range []string{statAttackSpeed, statCastSpeed} {
			mods = append(mods, core.Modifier{
				Stat: sh.stats[stat], Op: core.OpInc, Value: 1, Source: src, Scales: []core.Scale{granted, buffScale(effect, 1)},
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
