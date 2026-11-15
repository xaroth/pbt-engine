// Package itemdefence computes an armour item's own defences from its base, the roll of each in
// the base's range, its quality and its local modifiers, as Path of Building does (Item.lua,
// BuildModListForSlotNum).
package itemdefence

import (
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

const percentScale = 100

// Defence is one of an armour item's defences and the local stats that change it.
type Defence struct {
	Key build.Defence
	// Name is how a message names it, such as "energy shield".
	Name string
	// Stat is the character's stat the item's value adds to; "" for ward, not a sheet value.
	Stat    string
	base    func(*dataset.ArmourValues) dataset.ValueRange
	printed func(*build.Item) int
	// flat are the local flat stats of this defence alone, hybrid those it shares with another.
	flat, hybrid, inc []string
}

const (
	localDefencesInc      = "local_armour_and_evasion_and_energy_shield_+%"
	localEvasionAndESFlat = "local_evasion_rating_and_energy_shield"
	// qualityIgnored is "Quality does not increase Defences".
	qualityIgnored = "local_quality_does_not_increase_defences"
)

// All returns the defences in a fixed order: armour, evasion, energy shield, ward.
func All() []Defence {
	return []Defence{
		{
			Key: build.DefenceArmour, Name: "armour", Stat: "base_physical_damage_reduction_rating",
			base:    func(a *dataset.ArmourValues) dataset.ValueRange { return a.Armour },
			printed: func(it *build.Item) int { return it.Armour },
			flat:    []string{"local_base_physical_damage_reduction_rating"},
			inc: []string{
				"local_physical_damage_reduction_rating_+%", "local_armour_and_evasion_+%", "local_armour_and_energy_shield_+%",
				localDefencesInc,
			},
		},
		{
			Key: build.DefenceEvasion, Name: "evasion", Stat: "base_evasion_rating",
			base:    func(a *dataset.ArmourValues) dataset.ValueRange { return a.Evasion },
			printed: func(it *build.Item) int { return it.Evasion },
			flat:    []string{"local_base_evasion_rating"},
			hybrid:  []string{localEvasionAndESFlat},
			inc: []string{
				"local_evasion_rating_+%", "local_armour_and_evasion_+%", "local_evasion_and_energy_shield_+%",
				localDefencesInc,
			},
		},
		{
			Key: build.DefenceEnergyShield, Name: "energy shield", Stat: "base_maximum_energy_shield",
			base:    func(a *dataset.ArmourValues) dataset.ValueRange { return a.EnergyShield },
			printed: func(it *build.Item) int { return it.EnergyShield },
			flat:    []string{"local_energy_shield"},
			hybrid:  []string{localEvasionAndESFlat},
			inc: []string{
				"local_energy_shield_+%", "local_armour_and_energy_shield_+%", "local_evasion_and_energy_shield_+%",
				localDefencesInc,
			},
		},
		{
			Key: build.DefenceWard, Name: "ward",
			base:    func(a *dataset.ArmourValues) dataset.ValueRange { return a.Ward },
			printed: func(it *build.Item) int { return it.Ward },
			flat:    []string{"local_ward"},
			inc:     []string{"local_ward_+%", localDefencesInc},
		},
	}
}

// IsLocal reports whether stat changes an item's own defences rather than the character's.
func IsLocal(stat string) bool {
	if stat == qualityIgnored {
		return true
	}
	for _, d := range All() {
		if slices.Contains(d.flat, stat) || slices.Contains(d.hybrid, stat) || slices.Contains(d.inc, stat) {
			return true
		}
	}
	return false
}

// Local is what an item's defences read of it besides the base and the rolls.
type Local struct {
	sums    map[string]float64
	quality float64
}

// LocalOf returns the sums of its stats and its quality, 0 when quality does not increase its
// defences.
func LocalOf(it *build.Item) Local {
	l := Local{sums: map[string]float64{}, quality: float64(it.Quality)}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			l.sums[sv.Stat] += float64(sv.Value)
		}
	}
	if l.sums[qualityIgnored] > 0 {
		l.quality = 0
	}
	return l
}

// Printed is the defence as its text prints it, 0 when it prints none.
func (d Defence) Printed(it *build.Item) int { return d.printed(it) }

// Varies reports whether the base's value of the defence rolls in a range.
func (d Defence) Varies(a *dataset.ArmourValues) bool {
	r := d.base(a)
	return r.Max != r.Min
}

// Value returns the defence of an item on a base with values a and local l, its base value at
// roll in the base's range: the base value plus the local flat stats, times the local increases
// and, apart, the quality, rounded half up.
func (d Defence) Value(a *dataset.ArmourValues, l Local, roll float64) float64 {
	r := d.base(a)
	flat := l.sum(d.flat) + float64(r.Min) + l.sum(d.hybrid) + float64(r.Max-r.Min)*roll
	return math.Floor(flat*l.scale(d) + 0.5) //nolint:mnd // round half up, as PoB
}

// Roll returns the roll that gives printed as the defence of an item on a base with values a and
// local l, kept within the range and rounded to 4 decimals as Path of Building does;
// 1 for a base whose value does not roll. Unlike Path of Building it counts the local flat
// stats shared with another defence, as Value does.
func (d Defence) Roll(a *dataset.ArmourValues, l Local, printed int) float64 {
	r := d.base(a)
	if r.Max == r.Min {
		return 1
	}
	roll := (float64(printed)/l.scale(d) - (l.sum(d.flat) + float64(r.Min) + l.sum(d.hybrid))) / float64(r.Max-r.Min)
	const decimals = 1e4
	return math.Floor(min(max(roll, 0), 1)*decimals+0.5) / decimals //nolint:mnd // round half up, as PoB
}

func (l Local) sum(stats []string) float64 {
	v := 0.0
	for _, s := range stats {
		v += l.sums[s]
	}
	return v
}

func (l Local) scale(d Defence) float64 {
	return (1 + l.sum(d.inc)/percentScale) * (1 + l.quality/percentScale)
}
