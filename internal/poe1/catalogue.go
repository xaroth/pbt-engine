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
	// hand is which hand's value an output of the skill group reads.
	hand handRead
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
	// needsAttackHit is an attack's hit.
	needsAttackHit
	// needsOneHandHit is a hit that is not an attack's with both hands.
	needsOneHandHit
	// needsBothHandsHit is an attack's hit with both hands.
	needsBothHandsHit
	// needsDot is a skill that deals damage over time (dotSkill).
	needsDot
)

// IsSkill reports whether o is read in the main skill's context.
func (o Output) IsSkill() bool { return o.Group == GroupSkill }

// Gives reports whether the sheet gives o for a build whose main skill is sk, nil for none, with
// hands: an output of the sheet always, one of the skill group for a main skill; its hit only
// for a hit, its accuracy only for an attack's, its hit per damage type only for a hit with one
// hand, its hit per hand only for an attack's with both, its rate of hits and hit damage per
// second only for a hit the character uses itself, and its damage over time only for a skill
// that deals it.
func Gives(o Output, sk *resolve.Skill, hands Hands) bool {
	switch {
	case !o.IsSkill():
		return true
	case sk == nil:
		return false
	}
	return o.needs.met(sk, hands)
}

// met reports whether sk with hands is what n needs.
func (n skillNeed) met(sk *resolve.Skill, hands Hands) bool {
	switch n {
	case needsHit:
		return Hit(sk, hands)
	case needsRate:
		return selfUsedHit(sk, hands)
	case needsAttackHit:
		return attackHit(sk, hands)
	case needsOneHandHit:
		return Hit(sk, hands) && !bothHandsHit(sk, hands)
	case needsBothHandsHit:
		return bothHandsHit(sk, hands)
	case needsDot:
		return dotSkill(sk)
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
	}), group(GroupSkill, hitOutputs()), group(GroupSkill, dotOutputs()))
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

// hitOutputs are the main skill's hit outputs: what the enemy takes from an average hit, and
// from each hand's of an attack with both, an attack's accuracy, the chance to hit and that a
// use strikes critically, hits per second and hit damage per second, and per damage type the
// smallest and largest hit that is not critical, of a hit with one hand or of each hand of an
// attack with both.
func hitOutputs() []Output {
	return append([]Output{
		{Key: AverageHit, Label: "Average Hit", Better: output.Higher, Places: hitPlaces, stat: statAverageHit, needs: needsHit},
		{
			Key: MainHandAverageHit, Label: "Main Hand Average Hit", Better: output.Higher, Places: hitPlaces,
			stat: statAverageHit, needs: needsBothHandsHit, hand: readMainHand,
		},
		{
			Key: OffHandAverageHit, Label: "Off Hand Average Hit", Better: output.Higher, Places: hitPlaces,
			stat: statAverageHit, needs: needsBothHandsHit, hand: readOffHand,
		},
		{Key: AccuracyRating, Label: "Accuracy Rating", Better: output.Higher, stat: statAccuracyRating, needs: needsAttackHit},
		{Key: HitChance, Label: "Chance to Hit", Better: output.Higher, Unit: UnitPercent, stat: statHitChance, needs: needsHit},
		{
			Key: CriticalStrikeChance, Label: "Critical Strike Chance", Better: output.Higher, Unit: UnitPercent, Places: ratePlaces,
			stat: statEffectiveCriticalStrikeChance, needs: needsHit,
		},
		{
			Key: HitsPerSecond, Label: "Hits per Second", Better: output.Higher, Unit: UnitPerSecond, Places: ratePlaces,
			stat: statHitsPerSecond, needs: needsRate,
		},
		{Key: HitDPS, Label: "Hit DPS", Better: output.Higher, Places: hitPlaces, stat: statHitDPS, needs: needsRate},
	}, hitTypeOutputs()...)
}

// hitTypeOutputs are the smallest and largest hit per damage type that is not critical, of a
// hit with one hand and of each hand of an attack with both.
func hitTypeOutputs() []Output {
	type hitType struct {
		t        DamageType
		min, max output.Key
	}
	hands := []struct {
		label string
		needs skillNeed
		hand  handRead
		types []hitType
	}{
		{"", needsOneHandHit, readSkill, []hitType{
			{Physical, PhysicalMinimum, PhysicalMaximum}, {Fire, FireMinimum, FireMaximum}, {Cold, ColdMinimum, ColdMaximum},
			{Lightning, LightningMinimum, LightningMaximum}, {Chaos, ChaosMinimum, ChaosMaximum},
		}},
		{"Main Hand ", needsBothHandsHit, readMainHand, []hitType{
			{Physical, MainHandPhysicalMinimum, MainHandPhysicalMaximum}, {Fire, MainHandFireMinimum, MainHandFireMaximum},
			{Cold, MainHandColdMinimum, MainHandColdMaximum}, {Lightning, MainHandLightningMinimum, MainHandLightningMaximum},
			{Chaos, MainHandChaosMinimum, MainHandChaosMaximum},
		}},
		{"Off Hand ", needsBothHandsHit, readOffHand, []hitType{
			{Physical, OffHandPhysicalMinimum, OffHandPhysicalMaximum}, {Fire, OffHandFireMinimum, OffHandFireMaximum},
			{Cold, OffHandColdMinimum, OffHandColdMaximum}, {Lightning, OffHandLightningMinimum, OffHandLightningMaximum},
			{Chaos, OffHandChaosMinimum, OffHandChaosMaximum},
		}},
	}
	var out []Output
	for _, h := range hands {
		for _, d := range h.types {
			name := strings.ToUpper(d.t.String()[:1]) + d.t.String()[1:]
			out = append(out,
				Output{
					Key: d.min, Label: h.label + "Minimum " + name + " Hit", Better: output.Higher, Places: hitPlaces,
					stat: statEffectiveMinimumHitDamage, damage: d.t, needs: h.needs, hand: h.hand,
				},
				Output{
					Key: d.max, Label: h.label + "Maximum " + name + " Hit", Better: output.Higher, Places: hitPlaces,
					stat: statEffectiveMaximumHitDamage, damage: d.t, needs: h.needs, hand: h.hand,
				},
			)
		}
	}
	return out
}

// group returns outputs with their group set to g.
func group(g string, outputs []Output) []Output {
	for i := range outputs {
		outputs[i].Group = g
	}
	return outputs
}

// Output returns the value of catalogue output k, an output of the skill group in the main
// skill's context: the sheet's first skill.
func (sh *Sheet) Output(k output.Key) (float64, error) { return sh.SkillOutput(0, k) }

// SkillOutput returns the value of catalogue output k, an output of the skill group in the
// context of skill i of SetSkills.
func (sh *Sheet) SkillOutput(i int, k output.Key) (float64, error) {
	o, ok := sh.outputs[k]
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownOutput, k)
	}
	n, err := sh.outputNode(o, i)
	if err != nil {
		return 0, err
	}
	v, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: %s: %w", k, err)
	}
	return v, nil
}

// outputNode returns the node o reads: a skill output's in the context of skill i, for its
// damage type, a hit output's for hits and a damage over time output's for damage over time;
// ErrSkill when the sheet has no skill i.
func (sh *Sheet) outputNode(o Output, i int) (core.Node, error) {
	if !o.IsSkill() {
		return core.Node{Stat: sh.stats[o.stat]}, nil
	}
	var damage []DamageType
	if o.damage != 0 {
		damage = []DamageType{o.damage}
	}
	var form damageForm
	switch o.needs {
	case needsSkill:
		form = formNone
	case needsDot:
		form = formDot
	case needsHit, needsRate, needsAttackHit, needsOneHandHit, needsBothHandsHit:
		form = formHit
	}
	return sh.skillHandNode(i, o.stat, damage, form, o.hand)
}

// outputsByKey returns the catalogue by key.
func outputsByKey() map[output.Key]Output {
	all := Outputs()
	out := make(map[output.Key]Output, len(all))
	for _, o := range all {
		out[o.Key] = o
	}
	return out
}
