package poe1_test

import (
	"errors"
	"testing"

	rules "github.com/xaroth/pbt-engine/rules/poe1"
)

func TestOutputs(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(endgame()...))
	seen := map[string]bool{}
	for _, o := range rules.Outputs() {
		if seen[o.Name] {
			t.Errorf("output %s listed twice", o.Name)
		}
		seen[o.Name] = true
		if _, err := sh.Output(o.Name); err != nil {
			t.Errorf("%s: %v", o.Name, err)
		}
	}
	for name, want := range map[string]float64{"player.life": 1134, "player.fire_resistance": -60, "player.maximum_fire_resistance": 75} {
		if got, err := sh.Output(name); err != nil || got != want {
			t.Errorf("%s: got %v, %v, want %v", name, got, err, want)
		}
	}
	if _, err := sh.Output("player.lifes"); !errors.Is(err, rules.ErrUnknownOutput) {
		t.Errorf("unknown output: got %v, want %v", err, rules.ErrUnknownOutput)
	}
}
