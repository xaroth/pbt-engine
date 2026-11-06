package poe1

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/statdesc"
)

// sheetStats are the engine stats of the character sheet, named after the game's words.
// Resistances are uncapped values; the maximum_ stats are their caps. Regeneration is per
// second: life_regenerated_per_second is a flat amount ("Regenerate 10 Life per second"),
// percent_of_life_regenerated_per_second a share of maximum life ("Regenerate 1% of Life per
// second"), and life_regeneration_rate holds the increases and more multipliers of the
// "Life Regeneration rate"; the same for mana. They live here until rules/poe1 defines them
// as core stats.
var sheetStats = []string{
	"armour",
	"chaos_resistance",
	"cold_resistance",
	"dexterity",
	"evasion_rating",
	"fire_resistance",
	"intelligence",
	"level",
	"life_regenerated_per_second",
	"life_regeneration_rate",
	"lightning_resistance",
	"mana_regenerated_per_second",
	"mana_regeneration_rate",
	"maximum_chaos_resistance",
	"maximum_cold_resistance",
	"maximum_energy_shield",
	"maximum_fire_resistance",
	"maximum_life",
	"maximum_lightning_resistance",
	"maximum_mana",
	"percent_of_life_regenerated_per_second",
	"percent_of_mana_regenerated_per_second",
	"strength",
}

// sheetRules are the rules the table refers to for mechanics a modifier cannot express. They
// live here until rules/poe1 implements them.
var sheetRules = []string{
	// The bonuses of strength, dexterity and intelligence, which some stats turn off.
	"attribute_bonuses",
	// Maximum resistances and the caps they put on resistances.
	"resistance_caps",
	"chaos_resistance_doubled",
	"convert_evasion_rating_to_armour",
	"convert_life_to_energy_shield",
	"energy_shield_from_spell_block_chance",
	"fire_resistance_modifiers_also_apply",
	// "Gain X% of a value as Extra Y": a share of one final value added to another.
	"gain_as_extra",
	// Whether and where life regeneration recovers.
	"life_regeneration",
}

// Vocabulary is the engine stats and rules of the character sheet, for semantics.Load.
type Vocabulary struct{}

// IsEngineStat reports whether name is an engine stat of the character sheet.
func (Vocabulary) IsEngineStat(name string) bool { return slices.Contains(sheetStats, name) }

// IsRule reports whether name is a rule of the character sheet.
func (Vocabulary) IsRule(name string) bool { return slices.Contains(sheetRules, name) }

// Description files the passive tree's stats render with, by resolve scope.
const (
	passiveDescriptions = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
	auraDescriptions    = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
)

// DescriptionFiles returns the description file the passive tree renders each resolve scope
// with, for semantics.Table.Check.
func DescriptionFiles() map[resolve.Scope]string {
	return map[resolve.Scope]string{resolve.Global: passiveDescriptions, resolve.Buff: auraDescriptions}
}

// rule is a named pattern FeedsSheet decides with. Each has its own test case.
type rule struct {
	name string
	re   *regexp.Regexp
}

func newRules(pairs ...string) []rule {
	out := make([]rule, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, rule{pairs[i], regexp.MustCompile(pairs[i+1])})
	}
	return out
}

func firstMatch(rules []rule, s string) (rule, bool) {
	for _, r := range rules {
		if r.re.MatchString(s) {
			return r, true
		}
	}
	return rule{}, false
}

// Stat ids decide first where an id pattern is reliable: every stat with text it matches in
// 3.28.0.16 and 3.29.3.3 is judged the same way by that text, and a wording change in a patch
// cannot move it.
var (
	// feedsByID matches stats that change a sheet value, most of them without description text.
	feedsByID = newRules(
		"level", `^level$`,
		"attribute, life or mana per level", `^(?:strength|dexterity|intelligence|life|mana)_per_level$`,
		"base maximum resistance", `^base_maximum_\w*resistances?_%$`,
	)
	// notTheirsByID matches stats of someone other than the player.
	notTheirsByID = newRules(
		"minion, totem or golem stat", `^(?:minion|totems?|golem)_`,
		"enemy stat", `^(?:enemies|marked_enemies|nearby_enemies)_`,
	)
	// notSheetByID matches stats of quantities the sheet does not show.
	notSheetByID = newRules(
		"mana cost", `mana_cost`,
		"reservation", `reservation_efficiency`,
		"penetration", `penetrat|^(?:base_)?reduce_enemy_\w*resistance`,
		"recovery on events", `_gain(?:ed)?_on_|_gain_per_target|^recover_`,
		"energy shield recharge and recovery", `^energy_shield_(?:recharge|recovery)`,
		"flask recovery", `flask_\w*to_recover`,
		"exposure", `exposure_you_inflict`,
	)
)

// notSheet are stats whose text matches sheetLines but that change no sheet value, with the
// reason.
var notSheet = map[string]string{
	"maximum_physical_damage_reduction_%": "reads \"to maximum Armour\", but its id and Character.ot value (90) " +
		"make it the cap on physical damage reduction from armour",
	"consecrated_ground_allies_recover_es_as_well_as_life_from_life_regeneration": "life regeneration also " +
		"recovering energy shield is recovery, not a sheet value",
}

const quantity = `(?:Strength|Dexterity|Intelligence|Attributes|Life|Mana|Energy Shield|Evasion Rating|Armour|` +
	`(?:all )?(?:maximum |Maximum )?(?:Elemental |Fire |Cold |Lightning |Chaos )?Resistances?)`

const maximum = `(?:maximum |Maximum )?`

// sheetLines match a description line that changes a sheet value of whoever it applies to.
var sheetLines = newRules(
	"flat to quantity", `(?:\}|\d)%? to (?:all )?`+maximum+quantity+`\b`,
	"increased or more quantity", `\b(?:increased|reduced|more|less) `+maximum+quantity+`\b`,
	"value of quantity", `\}%? `+maximum+quantity+`\b`,
	"regenerate life or mana", `Regenerat\w*\b.*\b(?:Life|Mana)\b`,
	"life or mana regeneration", `\b(?:Life|Mana) Regeneration\b`,
	"gain as extra", `as Extra (?:Maximum )?`+quantity+`\b`,
	"converted to quantity", `Converted to `+quantity,
	"quantity increased or doubled", `\b`+maximum+quantity+` is (?:increased|doubled)`,
	"modifiers to quantity", `Modifiers to `+maximum+quantity,
	"grants quantity", `\bGrants `+maximum+quantity,
	"gain quantity instead", `\bGain `+maximum+quantity+` instead of`,
	"defences from equipped items", `Defences from Equipped|\b`+quantity+` from Equipped`,
	"how regeneration recovers", `Recovery from Regeneration|Regeneration is applied`,
	"removes all mana or energy shield", `Removes all (?:Mana|Energy Shield)`,
	"maximum life becomes", `Maximum Life becomes`,
	"resistance doubled", `Resistance is doubled`,
	"no attribute bonus", `provides no (?:inherent )?bonus to`,
	"evasion rating converted to armour", `Converts all Evasion Rating to Armour`,
	"resistances capped", `Resistances are capped`,
)

// subject is the start of a line up to the word naming whom it applies to.
const subject = `^(?:\{[^}]*\}%? )?(?:Nearby |Your |Cursed |Burning |Bleeding |Poisoned |Chilled |Shocked |Ignited )?(?:\w+ )?`

// Exclusions of lines that match sheetLines. Each matches what a line changes, not a
// condition it holds under, so "increased Evasion Rating if Energy Shield Recharge has started"
// still feeds the sheet.
var (
	// notTheirLines match a line whose subject is not the player.
	notTheirLines = newRules(
		"enemies", subject+`(?:Enemies|Enemy)\b|\bEnemies (?:\w+ )*(?:have|take|deal)\b`,
		"minions and totems", subject+`(?:Minions|Totems|Golems|Spectres|Zombies|Skeletons|Mercenary|Sentinel|Animated)\b|`+
			`\bMinions (?:have|deal|take)\b`,
	)
	// notSheetLines match a line that changes a quantity the sheet does not show.
	notSheetLines = newRules(
		"mana cost", `Mana Cost|\bCosts? of\b`,
		"reservation", `Reservation`,
		"penetration", `Penetrat`,
		"mana burn", `Mana Burn`,
		"exposure", `Exposure you inflict|\binflict Exposure\b|Exposure inflicted`,
		"recovery on events", `\b(?:Gain|Lose|Recover) \{[^}]*\}%? (?:of )?(?:Life|Mana|Energy Shield)\b|\bRecover \d|gained when`,
		"flask recovery", `Recovery from Flasks`,
		"energy shield recharge and recovery", `Energy Shield Recharge Rate|Energy Shield Recovery|Regenerate \{[^}]*\}%? (?:of )?Energy Shield`,
		"corpses", `Corpses you Spawn`,
	)
)

// sheetText reads stat descriptions as the passive tree renders them.
type sheetText struct {
	files map[string]*statdesc.Scope
}

func newSheetText(s *dataset.Store) (*sheetText, error) {
	text, err := s.Text("en")
	if err != nil {
		return nil, fmt.Errorf("poe1: %w", err)
	}
	t := &sheetText{files: make(map[string]*statdesc.Scope, len(text.Descriptions))}
	for i := range text.Descriptions {
		t.files[text.Descriptions[i].Name] = &text.Descriptions[i]
	}
	return t, nil
}

// lines returns the lines of every variant of stat's description in scope, or nil when it
// has no text.
func (t *sheetText) lines(stat string, scope resolve.Scope) []string {
	file := DescriptionFiles()[scope]
	var out []string
	for sc := t.files[file]; sc != nil; sc = t.files[sc.Parent] {
		i := slices.IndexFunc(sc.Descriptions, func(d statdesc.Description) bool { return slices.Contains(d.Stats, stat) })
		if i < 0 {
			continue
		}
		for _, va := range sc.Descriptions[i].Variants {
			out = append(out, strings.Split(strings.ReplaceAll(va.Text, `\n`, "\n"), "\n")...)
		}
		break
	}
	return out
}

// judgement is whether a stat feeds the sheet, and when a line of its text matches
// sheetLines but the stat is excluded, why and which line.
type judgement struct {
	feeds              bool
	excluded, excludes string
}

// judge decides by id where an id rule matches, else by the first line of text that matches
// sheetLines and no exclusion.
func (t *sheetText) judge(stat string, scope resolve.Scope) judgement {
	lines := t.lines(stat, scope)
	// excludedBy reports the first line that reads as a sheet value, if any, as excluded.
	excludedBy := func(why string) judgement {
		for _, line := range lines {
			if _, ok := firstMatch(sheetLines, line); ok {
				return judgement{excluded: line, excludes: why}
			}
		}
		return judgement{}
	}
	if why := notSheet[stat]; why != "" {
		return excludedBy("listed: " + why)
	}
	if _, ok := firstMatch(feedsByID, stat); ok {
		return judgement{feeds: true}
	}
	if r, ok := firstMatch(notTheirsByID, stat); ok {
		return excludedBy("not the player, by id: " + r.name)
	}
	if r, ok := firstMatch(notSheetByID, stat); ok {
		return excludedBy("not a sheet value, by id: " + r.name)
	}
	var j judgement
	for _, line := range lines {
		if _, ok := firstMatch(sheetLines, line); !ok {
			continue
		}
		if r, ok := firstMatch(notTheirLines, line); ok {
			j.excluded, j.excludes = line, "not the player: "+r.name
			continue
		}
		if r, ok := firstMatch(notSheetLines, line); ok {
			j.excluded, j.excludes = line, "not a sheet value: "+r.name
			continue
		}
		return judgement{feeds: true}
	}
	return j
}

// FeedsSheet returns a predicate for semantics.Cover that reports whether a stat in a scope
// changes a character sheet value of the player (attributes, maximum life, mana and energy
// shield, armour, evasion rating, resistances and their maximums, life and mana
// regeneration) under any condition. It decides by the stat id where an id pattern is
// reliable, and otherwise reads the stat's English description in s as the passive tree
// renders it in that scope.
func FeedsSheet(s *dataset.Store) (func(stat string, scope resolve.Scope) bool, error) {
	t, err := newSheetText(s)
	if err != nil {
		return nil, err
	}
	return func(stat string, scope resolve.Scope) bool { return t.judge(stat, scope).feeds }, nil
}
