package items

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/itemdefence"
)

// DefenceDiffers is a defence game text prints that its base, quality and local modifiers give
// at no roll of the base; the printed value is kept.
const DefenceDiffers build.Code = "item-defence-differs"

// printedDefences counts the defences it prints that base b has.
func printedDefences(b *dataset.ItemBase, it *build.Item) int {
	if b.Armour == nil {
		return 0
	}
	n := 0
	for _, d := range itemdefence.All() {
		if d.Printed(it) != 0 && d.OnBase(b.Armour) {
			n++
		}
	}
	return n
}

// rollDefences gives each defence it prints without a roll the roll that gives the printed value,
// as Path of Building derives it. On game text the printed value is the check: where the defence
// at that roll differs from it, the defence keeps no roll, so the printed value is taken, with a
// warning.
func rollDefences(it *build.Item, base *dataset.ItemBase, game bool) []build.Warning {
	if base.Armour == nil {
		return nil
	}
	l := itemdefence.LocalOf(it)
	var warnings []build.Warning
	for _, d := range itemdefence.All() {
		printed := d.Printed(it)
		if _, ok := it.DefenceRolls[d.Key]; ok || printed == 0 {
			continue
		}
		roll := d.Roll(base.Armour, l, printed)
		if v := d.Value(base.Armour, l, roll); game && v != float64(printed) {
			warnings = append(warnings, build.Warning{
				Code: DefenceDiffers, Subject: d.Name,
				Message: fmt.Sprintf("the text gives %d, its base, quality and local modifiers %g; the text's taken", printed, v),
			})
			continue
		}
		if it.DefenceRolls == nil {
			it.DefenceRolls = map[build.Defence]float64{}
		}
		it.DefenceRolls[d.Key] = roll
	}
	return warnings
}
