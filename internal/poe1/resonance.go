package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
)

// statLowestResonance is the Resonance of the element the character has least of, the count
// Trinity's modifiers scale with or hold from.
const statLowestResonance = "lowest_resonance"

// LowestResonance is the character's Resonance on its lowest element, in Trinity's terms. The
// build's hits do not derive it: the config says it, at most maximumResonance. Its default, 0,
// is Path of Building's.
const LowestResonance config.Key[Resonance] = "lowest-resonance"

// Resonance is a count of Resonance, from 0.
type Resonance int

// Valid reports whether r is not negative.
func (r Resonance) Valid() error {
	if r >= 0 {
		return nil
	}
	return fmt.Errorf("%w: resonance %d, want 0 or more", ErrConfigValue, r)
}

// maximumResonance caps each element's Resonance, from the text of Trinity: "There is a
// maximum of 50 Resonance for each Element". Path of Building clamps its config to it too.
const maximumResonance = 50

func resonanceSpecs() []config.Spec {
	return []config.Spec{config.SpecOf(LowestResonance, 0)}
}

// addResonance adds the lowest Resonance, 0 until the config sets it.
func (sh *Sheet) addResonance() error {
	src := sh.newSource(Source{RuleID: "resonance", Rule: "Resonance (configured)"})
	var err error
	if sh.resonance, err = sh.session.AddModifier(core.Modifier{Stat: sh.stats[statLowestResonance], Op: core.OpBase, Source: src}); err != nil {
		return fmt.Errorf("sheet: resonance: %w", err)
	}
	return nil
}

// setResonance sets the lowest Resonance from cfg.
func (sh *Sheet) setResonance(cfg config.Resolved) {
	resonance, _ := config.Lookup(cfg, LowestResonance)
	// The modifier is this session's, so this cannot fail.
	_ = sh.session.SetModifierValue(sh.resonance, float64(min(resonance, maximumResonance)))
}

func resonanceMechanic() mechanic {
	return mechanic{name: "resonance", stats: []string{statLowestResonance}}
}
