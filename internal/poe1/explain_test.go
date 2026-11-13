package poe1_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

func explainOf(t *testing.T, sh *poe1.Sheet, key output.Key, v poe1.View) *poe1.Explanation {
	t.Helper()
	ex, err := sh.Explain(key, v)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

// fireSheet has 200 fire resistance and 10% increased life on node 1 and fire modifiers
// applying to cold at 50% on node 2.
func fireSheet() []resolve.Bundle {
	return append(endgame(
		resolve.StatValue{Stat: "base_fire_damage_resistance_%", Value: 200}, resolve.StatValue{Stat: "maximum_life_+%", Value: 10}),
		other(resolve.StatValue{Stat: "modifiers_to_fire_resistance_also_apply_to_cold_lightning_resistance_at_%_value", Value: 50}))
}

func TestExplainProportional(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(fireSheet()...))
	setSkills(t, sh, skill(0, "Spell"))
	for _, o := range poe1.Outputs() {
		ex := explainOf(t, sh, o.Key, poe1.Proportional)
		sum := 0.0
		for _, l := range ex.Lines {
			sum += l.Value
		}
		if !near(sum, ex.Amount) {
			t.Errorf("%s: lines sum to %v, amount %v", o.Key, sum, ex.Amount)
		}
	}
	fire := explainOf(t, sh, "player.fire_resistance", poe1.Proportional)
	if fire.Amount != 140 || fire.Value != 75 || len(fire.CapBy) != 1 || fire.Lines[0].Value != 200 || fire.Lines[0].Parts[0].Via != nil {
		t.Errorf("fire: %+v", fire)
	}
	if got := render(t, fire); !strings.HasPrefix(got, "Fire Resistance: 75% (+65% over)") {
		t.Errorf("fire header:\n%s", got)
	}
	cold := explainOf(t, sh, "player.cold_resistance", poe1.Proportional)
	top := cold.Lines[0]
	if top.Value != 100 || top.Source.String() != "node 2" || len(top.Parts) != 1 || top.Parts[0].From == nil || top.Parts[0].From.String() != "node 1" {
		t.Errorf("cold top line: %+v", top)
	}
}

// A marginal line is what the sheet shows without that bundle.
func TestExplainMarginal(t *testing.T) {
	t.Parallel()
	bundles := fireSheet()
	sh := open(t, resolved(bundles...))
	for _, out := range []output.Key{poe1.Life, poe1.FireResistance, poe1.ColdResistance} {
		ex := explainOf(t, sh, out, poe1.Marginal)
		for _, l := range ex.Lines {
			if l.Source.Bundle == nil {
				continue
			}
			var rest []resolve.Bundle
			for _, b := range bundles {
				if b.Source != l.Source.Bundle.Source {
					rest = append(rest, b)
				}
			}
			without, err := open(t, resolved(rest...)).Output(out)
			if err != nil {
				t.Fatal(err)
			}
			if got := ex.Value - without; math.Abs(got-l.Value) > 1e-9 {
				t.Errorf("%s without %s: line %v, recompute %v", out, l.Source, l.Value, got)
			}
		}
	}
	if ex := explainOf(t, sh, "player.fire_resistance", poe1.Marginal); ex.Amount != 140 || ex.Lines[0].Total != 200 {
		t.Errorf("fire marginal totals: %+v", ex)
	}
}

func TestExplainTree(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(fireSheet()...))
	ex := explainOf(t, sh, "player.cold_resistance", poe1.Tree)
	root := ex.Tree
	if root.Stat != "cold_resistance_capped" || root.Formula == "" || len(root.Reads) != 2 || root.Reads[0].Stat != "cold_resistance" {
		t.Fatalf("root: %+v", root)
	}
	var shared *poe1.TreeTerm
	for i, tt := range root.Reads[0].Terms {
		if tt.Stat == "fire_resistance" {
			shared = &root.Reads[0].Terms[i]
		}
	}
	if shared == nil || shared.Rate != 0.5 || shared.Value != 100 {
		t.Errorf("shared term: %+v", shared)
	}
	if got := render(t, ex); !strings.Contains(got, "base 100  node 1 on fire_resistance at 50%") {
		t.Errorf("tree:\n%s", got)
	}
	if _, err := sh.Explain("player.nothing", poe1.Tree); !errors.Is(err, poe1.ErrUnknownOutput) {
		t.Errorf("unknown output: got %v", err)
	}
	if _, err := sh.Explain("player.life", 9); !errors.Is(err, poe1.ErrUnknownView) {
		t.Errorf("unknown view: got %v", err)
	}
}
