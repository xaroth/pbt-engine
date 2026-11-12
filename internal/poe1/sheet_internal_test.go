package poe1

import "testing"

func TestMustIDPanicsOnUnregistered(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("mustID of an unregistered stat: no panic")
		}
	}()
	statDef(statFireResistance, nil)
}

// Each attribute bonus has a rule source id of its own.
func TestAttributeBonusIDs(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, b := range attributeBonuses() {
		if b.id == "" || seen[b.id] {
			t.Errorf("%s: id %q empty or repeated", b.constant, b.id)
		}
		seen[b.id] = true
	}
}
