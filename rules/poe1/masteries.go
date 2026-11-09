package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/semantics"
)

// lifeMasteriesNeeded is how many Life Masteries the life mastery count effect needs: "10%
// more Maximum Life if you have at least 6 Life Masteries allocated" (stat_descriptions.txt,
// life_mastery_count_maximum_life_+%_final). The 6 is in the text, not in the stat.
const lifeMasteriesNeeded = 6

// addLifeMasteryCount adds the rule's modifier for a stat of value v, on the entry's source:
// v% more maximum life while number_of_life_masteries is at least lifeMasteriesNeeded.
func (sh *Sheet) addLifeMasteryCount(e *semantics.Entry, v float64, src core.SourceID) error {
	mod := core.Modifier{
		Stat: sh.stats[statMaximumLife], Op: core.OpMore, Value: e.RuleFactor.Apply(v), Source: src,
		Conds: []core.Cond{{Ref: core.SelfRef(sh.stats[statNumberOfLifeMasteries]), Cmp: core.CmpGE, Threshold: lifeMasteriesNeeded}},
	}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: %s: %w", e.Stat, err)
	}
	return nil
}
