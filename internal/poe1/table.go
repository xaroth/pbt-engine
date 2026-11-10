package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/semantics"

	_ "embed"
)

// table is the stat semantics of Path of Exile 1 in the semantics format. Each entry cites
// its sources. The op follows the description's wording. The game's
// tutorial (ClientStrings TutorialPanelStatsAndModsCalculationsContent3 and Content4, in
// 3.28.0.16 and 3.29.3.3) says "increased" modifiers add together before they apply and "more"
// and "less" each multiply separately. Reading "+N to" as a flat amount that the increases
// then scale is ours; Caption1 to Caption3 of the same tutorial (life from an item, a passive
// and levels, then increased by passives) agree with it. Per-minute stats have the factor of
// their description's per_minute_to_per_second handler, 1/60. Stats a sheet value depends on
// in a way the table cannot express yet are unknown, with a typed reason; those that need
// rules code name the rule.
//
//go:embed table.json
var table []byte

// Load parses the table against the engine stats and rules of this package.
func Load() (*semantics.Table, error) {
	t, err := semantics.Load(table, Vocabulary{})
	if err != nil {
		return nil, fmt.Errorf("poe1: %w", err)
	}
	return t, nil
}
