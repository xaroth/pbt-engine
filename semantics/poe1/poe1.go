// Package poe1 holds the stat semantics of Path of Exile 1, authored from game data: table.json
// in the semantics format, the engine stats and rules of the character sheet it may name, and
// the predicate that says which stats feed the sheet.
//
// Each entry cites its sources. The op follows the description's wording. The game's
// tutorial (ClientStrings TutorialPanelStatsAndModsCalculationsContent3 and Content4, in
// 3.28.0.16 and 3.29.3.3) says "increased" modifiers add together before they apply and "more"
// and "less" each multiply separately. Reading "+N to" as a flat amount that the increases
// then scale is ours; Caption1 to Caption3 of the same tutorial (life from an item, a passive
// and levels, then increased by passives) agree with it. Per-minute stats have the factor of
// their description's per_minute_to_per_second handler, 1/60. Stats a sheet value depends on
// in a way the table cannot express yet are unknown, with a typed reason; those that need
// rules code name the rule.
package poe1

import (
	_ "embed"

	"github.com/xaroth/pbt-engine/semantics"
)

//go:embed table.json
var table []byte

// Load parses the edition's table against the sheet's Vocabulary.
func Load() (*semantics.Table, error) {
	return semantics.Load(table, Vocabulary{})
}
