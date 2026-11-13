package poe1

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/output"
)

// ErrConfigValue is a config value outside a key's set.
var ErrConfigValue = errors.New("not a value of the key")

// CampaignStage is how far the character is through the story campaign. Defeating Kitava at
// the end of act 5 and again at the end of act 10 each lowers the character's resistances.
type CampaignStage string

// Campaign stages.
const (
	// CampaignComplete is after defeating Kitava in act 10. It is the default, since the
	// endgame (maps) opens only then.
	CampaignComplete CampaignStage = "complete"
	// CampaignActs6To10 is after defeating Kitava in act 5, before act 10's Kitava.
	CampaignActs6To10 CampaignStage = "acts-6-10"
	// CampaignActs1To5 is before defeating Kitava in act 5.
	CampaignActs1To5 CampaignStage = "acts-1-5"
)

// campaignStages are the campaign stages, latest first.
func campaignStages() []CampaignStage {
	return []CampaignStage{CampaignComplete, CampaignActs6To10, CampaignActs1To5}
}

// Valid reports whether s is a campaign stage.
func (s CampaignStage) Valid() error {
	if slices.Contains(campaignStages(), s) {
		return nil
	}
	return fmt.Errorf("%w: campaign %q, want %s, %s or %s", ErrConfigValue, s, CampaignComplete, CampaignActs6To10, CampaignActs1To5)
}

// BanditChoice is the outcome of the act 2 quest Deal with the Bandits. The zero value is a
// character that has not completed the quest, so it has no reward from it.
type BanditChoice string

// Quest outcomes.
const (
	BanditsNotCompleted BanditChoice = ""
	BanditsKillAll      BanditChoice = "kill-all"
	BanditsHelpAlira    BanditChoice = "help-alira"
	BanditsHelpKraityn  BanditChoice = "help-kraityn"
	BanditsHelpOak      BanditChoice = "help-oak"
)

// banditChoices are the bandit quest outcomes, not completed first.
func banditChoices() []BanditChoice {
	return []BanditChoice{BanditsNotCompleted, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak}
}

// Valid reports whether b is a bandit quest outcome.
func (b BanditChoice) Valid() error {
	if slices.Contains(banditChoices(), b) {
		return nil
	}
	return fmt.Errorf("%w: bandits %q, want %s, %s, %s, %s or empty (quest not completed)",
		ErrConfigValue, b, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak)
}

// Config keys. Names describe the game, not PoB's options.
const (
	Campaign config.Key[CampaignStage] = "campaign"
	Bandits  config.Key[BanditChoice]  = "bandits"
)

// ConfigSpecs returns every poe1 config key with its default and values.
func ConfigSpecs() []config.Spec {
	return append([]config.Spec{
		config.SpecOf(Campaign, CampaignComplete, campaignStages()...),
		config.SpecOf(Bandits, BanditsNotCompleted, banditChoices()...),
	}, enemySpecs()...)
}

// Output keys, one per catalogue entry.
const (
	Strength                    output.Key = "player.strength"
	Dexterity                   output.Key = "player.dexterity"
	Intelligence                output.Key = "player.intelligence"
	Life                        output.Key = "player.life"
	ReservedLife                output.Key = "player.reserved_life"
	UnreservedLife              output.Key = "player.unreserved_life"
	Mana                        output.Key = "player.mana"
	ReservedMana                output.Key = "player.reserved_mana"
	UnreservedMana              output.Key = "player.unreserved_mana"
	EnergyShield                output.Key = "player.energy_shield"
	Armour                      output.Key = "player.armour"
	EvasionRating               output.Key = "player.evasion_rating"
	FireResistance              output.Key = "player.fire_resistance"
	ColdResistance              output.Key = "player.cold_resistance"
	LightningResistance         output.Key = "player.lightning_resistance"
	ChaosResistance             output.Key = "player.chaos_resistance"
	MaximumFireResistance       output.Key = "player.maximum_fire_resistance"
	MaximumColdResistance       output.Key = "player.maximum_cold_resistance"
	MaximumLightningResistance  output.Key = "player.maximum_lightning_resistance"
	MaximumChaosResistance      output.Key = "player.maximum_chaos_resistance"
	UncappedFireResistance      output.Key = "player.uncapped_fire_resistance"
	UncappedColdResistance      output.Key = "player.uncapped_cold_resistance"
	UncappedLightningResistance output.Key = "player.uncapped_lightning_resistance"
	UncappedChaosResistance     output.Key = "player.uncapped_chaos_resistance"
	LifeRegeneration            output.Key = "player.life_regeneration"
	ManaRegeneration            output.Key = "player.mana_regeneration"
	EnergyShieldRegeneration    output.Key = "player.energy_shield_regeneration"
	SpellBlockChance            output.Key = "player.spell_block_chance"
	MovementSpeed               output.Key = "player.movement_speed"
	PhysicalDamageModifier      output.Key = "skill.physical_damage_modifier"
	FireDamageModifier          output.Key = "skill.fire_damage_modifier"
	ColdDamageModifier          output.Key = "skill.cold_damage_modifier"
	LightningDamageModifier     output.Key = "skill.lightning_damage_modifier"
	ChaosDamageModifier         output.Key = "skill.chaos_damage_modifier"
	ManaCost                    output.Key = "skill.mana_cost"
	LifeCost                    output.Key = "skill.life_cost"
)
