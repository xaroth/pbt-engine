package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// matchingSocketQuality is the quality a gem gains in a socket of its own colour, from 3.29 on
// (Path of Building's MatchingSocketQualityBonus).
const matchingSocketQuality = 10

// GemRules returns the rules for the levels and quality of gems.
func GemRules() resolve.GemRules {
	return resolve.GemRules{
		Bonuses: append(gemBonuses(),
			resolve.GemBonus{Stat: "local_socketed_gems_in_red_sockets_get_level_+", Reach: resolve.Socketed, Socket: dataset.Red},
			resolve.GemBonus{Stat: "local_socketed_gems_in_green_sockets_get_quality_%", Quality: true, Reach: resolve.Socketed, Socket: dataset.Green},
		),
		MatchingSocketQuality: matchingSocketQuality,
		// 3.29 describes it as "Gems Socketed always have the Quality bonus from Socket Colour".
		AlwaysMatchingStat: "local_can_socket_gems_ignoring_colour",
		// "Socketed Support Gems can also Support Skills from your Main Hand" and "from Equipped
		// Body Armour".
		LinkedSupports: []resolve.LinkedSupport{
			{Stat: "support_gems_socketed_in_off_hand_also_support_main_hand_skills", To: build.SlotWeapon},
			{Stat: "support_gems_socketed_in_amulet_also_support_body_skills", To: build.SlotBodyArmour},
		},
	}
}

// gemBonuses returns the stats that add levels or quality to gems. Item stats name the gems
// they reach by GemTags: "skill gems" are gems that are not supports, "elemental" is fire, cold
// or lightning. Stats with a condition (influence counts, player level, filled sockets) are
// not listed.
func gemBonuses() []resolve.GemBonus {
	var out []resolve.GemBonus
	add := func(stat string, quality bool, reach resolve.GemReach, f resolve.GemFilter) {
		out = append(out, resolve.GemBonus{Stat: stat, Quality: quality, Reach: reach, Gems: f})
	}
	tag := func(kind resolve.GemKind, tags ...string) resolve.GemFilter {
		return resolve.GemFilter{Kind: kind, Tags: tags}
	}
	damage := []string{"fire", "cold", "lightning", "chaos", "physical"}
	elemental := damage[:3]
	attributes := []string{"strength", "dexterity", "intelligence"}
	const aura, curse, melee, minion, vaal = "aura", "curse", "melee", "minion", "vaal"

	add("local_socketed_gem_level_+", false, resolve.Socketed, tag(resolve.AnyGem))
	add("local_socketed_support_gem_level_+", false, resolve.Socketed, tag(resolve.SupportGem))
	add("local_socketed_active_skill_gem_level_+", false, resolve.Socketed, tag(resolve.SkillGem))
	add("local_socketed_non_vaal_gem_level_+", false, resolve.Socketed, resolve.GemFilter{NotTags: []string{vaal}})
	add("local_socketed_area_of_effect_gem_level_+", false, resolve.Socketed, tag(resolve.AnyGem, "area"))
	add("local_socketed_elemental_gem_level_+", false, resolve.Socketed, resolve.GemFilter{AnyTags: elemental})
	add("local_socketed_trap_and_mine_gem_level_+", false, resolve.Socketed, resolve.GemFilter{AnyTags: []string{"trap", "mine"}})
	for _, t := range slices.Concat([]string{
		"projectile", "spell", melee, bow, minion, aura, "movement", curse, vaal, "warcry", "golem", "herald", "duration", "hex", "trap",
	}, damage, attributes) {
		add("local_socketed_"+t+"_gem_level_+", false, resolve.Socketed, tag(resolve.AnyGem, t))
	}

	add("local_socketed_gem_quality_+", true, resolve.Socketed, tag(resolve.AnyGem))
	add("local_socketed_support_gem_quality_+", true, resolve.Socketed, tag(resolve.SupportGem))
	add("local_socketed_active_skill_gem_quality_+", true, resolve.Socketed, tag(resolve.SkillGem))
	add("local_socketed_area_of_effect_gem_quality_+", true, resolve.Socketed, tag(resolve.AnyGem, "area"))
	for _, t := range slices.Concat([]string{minion, melee, bow, aura, "projectile"}, damage[:4], attributes) {
		add("local_socketed_"+t+"_gem_quality_+", true, resolve.Socketed, tag(resolve.AnyGem, t))
	}

	add("all_skill_gem_level_+", false, resolve.Equipped, tag(resolve.SkillGem))
	add("spell_skill_gem_level_+", false, resolve.Equipped, tag(resolve.SkillGem, "spell"))
	for _, t := range damage {
		add(t+"_spell_skill_gem_level_+", false, resolve.Equipped, tag(resolve.SkillGem, t, "spell"))
	}
	for _, t := range slices.Concat([]string{minion, vaal, "totem", curse, melee, "herald", aura, "link"}, damage, attributes) {
		add(t+"_skill_gem_level_+", false, resolve.Equipped, tag(resolve.SkillGem, t))
	}
	add("critical_support_gem_level_+", false, resolve.Equipped, tag(resolve.SupportGem, "critical"))
	add("non_exceptional_support_gem_level_+", false, resolve.Equipped, resolve.GemFilter{Kind: resolve.SupportGem, NotTags: []string{"exceptional"}})
	add("all_skill_gem_quality_+", true, resolve.Equipped, tag(resolve.SkillGem))
	add("minion_skill_gem_quality_+", true, resolve.Equipped, tag(resolve.SkillGem, minion))

	add("supported_active_skill_gem_level_+", false, resolve.Supported, tag(resolve.SkillGem))
	add("supported_active_skill_gem_quality_%", true, resolve.Supported, tag(resolve.SkillGem))
	add("supported_elemental_skill_gem_level_+", false, resolve.Supported, resolve.GemFilter{Kind: resolve.SkillGem, AnyTags: elemental})
	for _, t := range append([]string{aura, curse, "strike", minion}, damage...) {
		add("supported_"+t+"_skill_gem_level_+", false, resolve.Supported, tag(resolve.SkillGem, t))
	}
	return out
}
