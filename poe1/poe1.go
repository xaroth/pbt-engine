// Package poe1 names what a host passes to and reads from a Path of Exile 1 simulation: the
// output keys (stats.Get(poe1.Life)), the config keys and their values
// (simulator.Set(poe1.Bandits, poe1.BanditsHelpOak)). Every name is an alias of the engine's
// own, so the two cannot disagree; the rules behind them stay internal.
package poe1

import (
	rules "github.com/xaroth/pbt-engine/internal/poe1"
)

// Output keys, as simulator.Sim.Outputs lists them.
const (
	Strength                    = rules.Strength
	Dexterity                   = rules.Dexterity
	Intelligence                = rules.Intelligence
	Life                        = rules.Life
	ReservedLife                = rules.ReservedLife
	UnreservedLife              = rules.UnreservedLife
	Mana                        = rules.Mana
	ReservedMana                = rules.ReservedMana
	UnreservedMana              = rules.UnreservedMana
	EnergyShield                = rules.EnergyShield
	Armour                      = rules.Armour
	EvasionRating               = rules.EvasionRating
	FireResistance              = rules.FireResistance
	ColdResistance              = rules.ColdResistance
	LightningResistance         = rules.LightningResistance
	ChaosResistance             = rules.ChaosResistance
	MaximumFireResistance       = rules.MaximumFireResistance
	MaximumColdResistance       = rules.MaximumColdResistance
	MaximumLightningResistance  = rules.MaximumLightningResistance
	MaximumChaosResistance      = rules.MaximumChaosResistance
	UncappedFireResistance      = rules.UncappedFireResistance
	UncappedColdResistance      = rules.UncappedColdResistance
	UncappedLightningResistance = rules.UncappedLightningResistance
	UncappedChaosResistance     = rules.UncappedChaosResistance
	LifeRegeneration            = rules.LifeRegeneration
	ManaRegeneration            = rules.ManaRegeneration
	EnergyShieldRegeneration    = rules.EnergyShieldRegeneration
	SpellBlockChance            = rules.SpellBlockChance
	MovementSpeed               = rules.MovementSpeed
	PhysicalDamageModifier      = rules.PhysicalDamageModifier
	FireDamageModifier          = rules.FireDamageModifier
	ColdDamageModifier          = rules.ColdDamageModifier
	LightningDamageModifier     = rules.LightningDamageModifier
	ChaosDamageModifier         = rules.ChaosDamageModifier
	ManaCost                    = rules.ManaCost
	LifeCost                    = rules.LifeCost
)

// Config keys, as simulator.Sim.ConfigKeys lists them.
const (
	Campaign                 = rules.Campaign
	Bandits                  = rules.Bandits
	Enemy                    = rules.Enemy
	EnemyLevel               = rules.EnemyLevel
	EnemyFireResistance      = rules.EnemyFireResistance
	EnemyColdResistance      = rules.EnemyColdResistance
	EnemyLightningResistance = rules.EnemyLightningResistance
	EnemyChaosResistance     = rules.EnemyChaosResistance
	EnemyArmour              = rules.EnemyArmour
	EnemyEvasion             = rules.EnemyEvasion
)

// CampaignStage is how far the character is through the story campaign: the value of
// Campaign. Defeating Kitava at the end of act 5 and again at the end of act 10 each lowers
// the character's resistances.
type CampaignStage = rules.CampaignStage

// Campaign stages. CampaignComplete is the default.
const (
	CampaignComplete  = rules.CampaignComplete
	CampaignActs6To10 = rules.CampaignActs6To10
	CampaignActs1To5  = rules.CampaignActs1To5
)

// BanditChoice is the outcome of the act 2 quest Deal with the Bandits: the value of Bandits.
type BanditChoice = rules.BanditChoice

// Quest outcomes. BanditsNotCompleted is the default, a character with no reward from the
// quest.
const (
	BanditsNotCompleted = rules.BanditsNotCompleted
	BanditsKillAll      = rules.BanditsKillAll
	BanditsHelpAlira    = rules.BanditsHelpAlira
	BanditsHelpKraityn  = rules.BanditsHelpKraityn
	BanditsHelpOak      = rules.BanditsHelpOak
)

// EnemyKind is what the enemy is, a normal monster or a boss of some tier: the value of Enemy.
// Enemy keys without a value take the kind's values.
type EnemyKind = rules.EnemyKind

// Enemy kinds. EnemyPinnacle is the default.
const (
	EnemyNone     = rules.EnemyNone
	EnemyBoss     = rules.EnemyBoss
	EnemyPinnacle = rules.EnemyPinnacle
	EnemyUber     = rules.EnemyUber
)

// MonsterLevel is a monster's level, from 1: the value of EnemyLevel.
type MonsterLevel = rules.MonsterLevel

// Rating is an armour or evasion rating, from 0: the value of EnemyArmour and EnemyEvasion.
type Rating = rules.Rating
