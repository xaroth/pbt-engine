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
