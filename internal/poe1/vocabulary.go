package poe1

import "slices"

// Vocabulary is the engine stats, rules and contexts of the character sheet, for
// semantics.Load.
type Vocabulary struct{}

// engineStats are the engine stats of every mechanic, named after the game's words.
func engineStats() []string {
	var out []string
	for _, m := range mechanics() {
		out = append(out, m.stats...)
	}
	return out
}

// IsEngineStat reports whether name is an engine stat of the character sheet.
func (Vocabulary) IsEngineStat(name string) bool { return slices.Contains(engineStats(), name) }

// RuleParams returns the parameters of rule name of the character sheet, and false for an
// unknown rule.
func (Vocabulary) RuleParams(name string) ([]string, bool) {
	for _, m := range mechanics() {
		for _, r := range m.rules {
			if r.name == name {
				return r.params, true
			}
		}
	}
	return nil, false
}

// IsContext reports whether name, as "dimension.value", is a context value a table modifier
// may require: a skill type (type.Spell), a damage type (damage.fire) or a form of damage
// (form.hit).
func (Vocabulary) IsContext(name string) bool { return isTableContext(name) }
