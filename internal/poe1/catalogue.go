package poe1

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// Errors of the catalogue and explanations.
var (
	// ErrUnknownOutput is returned for an output name not in the catalogue.
	ErrUnknownOutput = errors.New("unknown output")
	// ErrUnknownView is returned for a View that is not Tree, Proportional or Marginal.
	ErrUnknownView = errors.New("unknown view")
)

// Units of outputs.
const (
	UnitNone      = ""
	UnitPercent   = "%"
	UnitPerSecond = "/s"
)

// Groups of outputs: the panel a consumer shows an output in.
const (
	// GroupSheet is the character sheet: attributes, pools, armour and evasion, resistances,
	// regeneration and movement speed.
	GroupSheet = "sheet"
	// GroupDefence is how the character avoids or mitigates hits: block.
	GroupDefence = "defence"
	// GroupSkill is the main skill's values, read in its context. A build without a main
	// skill has none of them.
	GroupSkill = "skill"
)

// Output is a named value of the sheet a consumer can ask for and render without knowing the
// engine stats: its engine stat, a label, a unit, the decimal places it shows, its group, which
// way is better and its display order.
type Output struct {
	Key    output.Key `json:"key"`
	Label  string     `json:"label"`
	Unit   string     `json:"unit"`
	Places int        `json:"places"`
	Group  string     `json:"group"`
	// Better is which way the value improves the build.
	Better output.Better `json:"better"`
	// Order is the output's position in the catalogue; lower shows first.
	Order int `json:"order"`
	stat  string
	// damage, for an output of the skill group, is the damage type of the part of the main
	// skill's damage it reads.
	damage DamageType
	// needs is what the main skill must be for the sheet to give the output.
	needs skillNeed
}

// skillNeed is what a main skill must be for the sheet to give an output of it.
type skillNeed uint8

const (
	// needsSkill is any main skill.
	needsSkill skillNeed = iota
	// needsHit is a hit (Hit).
	needsHit
	// needsRate is a hit the character uses itself.
	needsRate
	// needsSpellHit is a spell hit (SpellHit): an attack's chance to hit is not given yet.
	needsSpellHit
	// needsSpellRate is a spell hit the character casts itself.
	needsSpellRate
)

// IsSkill reports whether o is read in the main skill's context.
func (o Output) IsSkill() bool { return o.Group == GroupSkill }

// Gives reports whether the sheet gives o for a build whose main skill is sk, nil for none, with
// hands: an output of the sheet always, one of the skill group for a main skill; its hit only
// for a hit, its rate of hits only for a hit the character uses itself, and its chance to hit
// and hit damage per second only for a spell.
func Gives(o Output, sk *resolve.Skill, hands Hands) bool {
	switch {
	case !o.IsSkill():
		return true
	case sk == nil:
		return false
	}
	switch o.needs {
	case needsHit:
		return Hit(sk, hands)
	case needsRate:
		return selfUsedHit(sk, hands)
	case needsSpellHit:
		return SpellHit(sk)
	case needsSpellRate:
		return SpellHit(sk) && selfUsedHit(sk, hands)
	case needsSkill:
	}
	return true
}

// Outputs returns the catalogue of outputs, in display order.
func Outputs() []Output {
	out := slices.Concat(group(GroupSheet, []Output{
		{Key: Strength, Label: "Strength", Better: output.Higher, stat: statStrength},
		{Key: Dexterity, Label: "Dexterity", Better: output.Higher, stat: statDexterity},
		{Key: Intelligence, Label: "Intelligence", Better: output.Higher, stat: statIntelligence},
		{Key: Life, Label: "Life", Better: output.Higher, stat: statMaximumLife},
		{Key: ReservedLife, Label: "Reserved Life", Better: output.Lower, stat: statReservedLife},
		{Key: UnreservedLife, Label: "Unreserved Life", Better: output.Higher, stat: statUnreservedLife},
		{Key: Mana, Label: "Mana", Better: output.Higher, stat: statMaximumMana},
		{Key: ReservedMana, Label: "Reserved Mana", Better: output.Lower, stat: statReservedMana},
		{Key: UnreservedMana, Label: "Unreserved Mana", Better: output.Higher, stat: statUnreservedMana},
		{Key: EnergyShield, Label: "Energy Shield", Better: output.Higher, stat: statMaximumEnergyShield},
		{Key: Armour, Label: "Armour", Better: output.Higher, stat: statArmour},
		{Key: EvasionRating, Label: "Evasion Rating", Better: output.Higher, stat: statEvasionRating},
		{Key: FireResistance, Label: "Fire Resistance", Better: output.Higher, Unit: UnitPercent, stat: statFireResistanceCapped},
		{Key: ColdResistance, Label: "Cold Resistance", Better: output.Higher, Unit: UnitPercent, stat: statColdResistanceCapped},
		{Key: LightningResistance, Label: "Lightning Resistance", Better: output.Higher, Unit: UnitPercent, stat: statLightningResistanceCapped},
		{Key: ChaosResistance, Label: "Chaos Resistance", Better: output.Higher, Unit: UnitPercent, stat: statChaosResistanceCapped},
		{Key: MaximumFireResistance, Label: "Maximum Fire Resistance", Better: output.Higher, Unit: UnitPercent, stat: statEffectiveMaximumFireResistance},
		{Key: MaximumColdResistance, Label: "Maximum Cold Resistance", Better: output.Higher, Unit: UnitPercent, stat: statEffectiveMaximumColdResistance},
		{
			Key: MaximumLightningResistance, Label: "Maximum Lightning Resistance", Better: output.Higher, Unit: UnitPercent,
			stat: statEffectiveMaximumLightningResistance,
		},
		{Key: MaximumChaosResistance, Label: "Maximum Chaos Resistance", Better: output.Higher, Unit: UnitPercent, stat: statMaximumChaosResistance},
		{Key: UncappedFireResistance, Label: "Uncapped Fire Resistance", Better: output.Higher, Unit: UnitPercent, stat: statFireResistance},
		{Key: UncappedColdResistance, Label: "Uncapped Cold Resistance", Better: output.Higher, Unit: UnitPercent, stat: statColdResistance},
		{Key: UncappedLightningResistance, Label: "Uncapped Lightning Resistance", Better: output.Higher, Unit: UnitPercent, stat: statLightningResistance},
		{Key: UncappedChaosResistance, Label: "Uncapped Chaos Resistance", Better: output.Higher, Unit: UnitPercent, stat: statChaosResistance},
		{Key: LifeRegeneration, Label: "Life Regeneration", Better: output.Higher, Unit: UnitPerSecond, Places: regenerationPlaces, stat: statLifeRegeneration},
		{Key: ManaRegeneration, Label: "Mana Regeneration", Better: output.Higher, Unit: UnitPerSecond, Places: regenerationPlaces, stat: statManaRegeneration},
		{
			Key: EnergyShieldRegeneration, Label: "Energy Shield Regeneration", Better: output.Higher, Unit: UnitPerSecond, Places: regenerationPlaces,
			stat: statEnergyShieldRegeneration,
		},
		{Key: MovementSpeed, Label: "Movement Speed", Better: output.Higher, Unit: UnitPercent, stat: statMovementSpeed},
	}), group(GroupDefence, []Output{
		{Key: SpellBlockChance, Label: "Chance to Block Spell Damage", Better: output.Higher, Unit: UnitPercent, stat: statSpellBlockChanceCapped},
	}), group(GroupSkill, []Output{
		// The increased and more multiplier of one damage type's part of the main skill's damage.
		{Key: PhysicalDamageModifier, Label: "Physical Damage Modifier", Better: output.Higher, Unit: UnitPercent, stat: statDamage, damage: Physical},
		{Key: FireDamageModifier, Label: "Fire Damage Modifier", Better: output.Higher, Unit: UnitPercent, stat: statDamage, damage: Fire},
		{Key: ColdDamageModifier, Label: "Cold Damage Modifier", Better: output.Higher, Unit: UnitPercent, stat: statDamage, damage: Cold},
		{Key: LightningDamageModifier, Label: "Lightning Damage Modifier", Better: output.Higher, Unit: UnitPercent, stat: statDamage, damage: Lightning},
		{Key: ChaosDamageModifier, Label: "Chaos Damage Modifier", Better: output.Higher, Unit: UnitPercent, stat: statDamage, damage: Chaos},
		{Key: ManaCost, Label: "Mana Cost", Better: output.Lower, Places: costPlaces, stat: statManaCost},
		{Key: LifeCost, Label: "Life Cost", Better: output.Lower, Places: costPlaces, stat: statLifeCost},
	}), group(GroupSkill, hitOutputs()))
	for i := range out {
		out[i].Order = i
	}
	return out
}

// Decimal places of hit outputs: damage, and chances and rates.
const (
	hitPlaces  = 1
	ratePlaces = 2
)

// hitOutputs are the main skill's hit outputs: what the enemy takes from an average hit, the
// chances to hit and to strike critically, hits per second and hit damage per second, and per
// damage type the smallest and largest hit that is not critical.
func hitOutputs() []Output {
	types := []struct {
		t        DamageType
		min, max output.Key
	}{
		{Physical, PhysicalMinimum, PhysicalMaximum}, {Fire, FireMinimum, FireMaximum}, {Cold, ColdMinimum, ColdMaximum},
		{Lightning, LightningMinimum, LightningMaximum}, {Chaos, ChaosMinimum, ChaosMaximum},
	}
	perType := make([]Output, 0, len(types)*len([]string{"minimum", "maximum"}))
	for _, d := range types {
		name := strings.ToUpper(d.t.String()[:1]) + d.t.String()[1:]
		perType = append(perType,
			Output{
				Key: d.min, Label: "Minimum " + name + " Hit", Better: output.Higher, Places: hitPlaces,
				stat: statEffectiveMinimumHitDamage, damage: d.t, needs: needsHit,
			},
			Output{
				Key: d.max, Label: "Maximum " + name + " Hit", Better: output.Higher, Places: hitPlaces,
				stat: statEffectiveMaximumHitDamage, damage: d.t, needs: needsHit,
			},
		)
	}
	return append([]Output{
		{Key: AverageHit, Label: "Average Hit", Better: output.Higher, Places: hitPlaces, stat: statAverageHit, needs: needsHit},
		{Key: HitChance, Label: "Chance to Hit", Better: output.Higher, Unit: UnitPercent, stat: statHitChance, needs: needsSpellHit},
		{
			Key: CriticalStrikeChance, Label: "Critical Strike Chance", Better: output.Higher, Unit: UnitPercent, Places: ratePlaces,
			stat: statCriticalStrikeChance, needs: needsHit,
		},
		{
			Key: HitsPerSecond, Label: "Hits per Second", Better: output.Higher, Unit: UnitPerSecond, Places: ratePlaces,
			stat: statHitsPerSecond, needs: needsRate,
		},
		{Key: HitDPS, Label: "Hit DPS", Better: output.Higher, Places: hitPlaces, stat: statHitDPS, needs: needsSpellRate},
	}, perType...)
}

// group returns outputs with their group set to g.
func group(g string, outputs []Output) []Output {
	for i := range outputs {
		outputs[i].Group = g
	}
	return outputs
}

// Output returns the value of catalogue output k.
func (sh *Sheet) Output(k output.Key) (float64, error) {
	o, ok := outputByKey(k)
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownOutput, k)
	}
	n, err := sh.outputNode(o)
	if err != nil {
		return 0, err
	}
	v, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: %s: %w", k, err)
	}
	return v, nil
}

// outputNode returns the node o reads: a skill output's in the main skill's context (the
// sheet's first skill), for its damage type, and a hit output's for hits; ErrSkill when the
// sheet has no skill.
func (sh *Sheet) outputNode(o Output) (core.Node, error) {
	if !o.IsSkill() {
		return core.Node{Stat: sh.stats[o.stat]}, nil
	}
	var damage []DamageType
	if o.damage != 0 {
		damage = []DamageType{o.damage}
	}
	form := formNone
	if o.needs != needsSkill {
		form = formHit
	}
	return sh.skillNode(0, o.stat, damage, form)
}

func outputByKey(k output.Key) (Output, bool) {
	for _, o := range Outputs() {
		if o.Key == k {
			return o, true
		}
	}
	return Output{}, false
}
