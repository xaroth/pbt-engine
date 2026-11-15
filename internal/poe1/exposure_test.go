package poe1_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// Exposure is a condition on the enemy, derived as the other conditions are: on when a hit of
// the build inflicts it on every hit, also when another skill than the main one does; an
// explicit off wins; set on with no source it applies the base 10% with a warning. Exposure a
// hit inflicts by chance is on only when its key is, then without a warning. The strongest
// Exposure of an element applies, at least its minimum. The main skill hits for 100 of the
// element against the default enemy's 50% resistance, so its minimum is 50 plus the Exposure.
func TestDeriveExposure(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	sks := func(s ...*resolve.Skill) []*resolve.Skill { return s }
	type want struct {
		on      bool
		src     config.Source
		warns   bool
		minimum float64
	}
	type element struct {
		name string
		key  config.Key[bool]
		read output.Key
	}
	elements := []element{
		{"fire", poe1.EnemyFireExposure, poe1.FireMinimum},
		{"cold", poe1.EnemyColdExposure, poe1.ColdMinimum},
		{"lightning", poe1.EnemyLightningExposure, poe1.LightningMinimum},
	}
	type tc struct {
		name   string
		set    string
		node   []resolve.StatValue
		skills []*resolve.Skill
		want   want
	}
	for _, e := range elements {
		magnitude := e.name + "_exposure_on_hit_magnitude"
		chance := "base_inflict_" + e.name + "_exposure_on_hit_%_chance"
		main := spellOf(0, e.name)
		inflicts := spellOf(1, "physical", stat(magnitude, -15))
		for _, c := range []tc{
			{name: "derived from the second skill", skills: sks(main, inflicts), want: want{true, config.Derived, false, 65}},
			{name: "derived from the main skill", skills: sks(spellOf(0, e.name, stat(magnitude, -15))), want: want{true, config.Derived, false, 65}},
			{name: "forced off", set: "false", skills: sks(main, inflicts), want: want{false, config.FromBuild, false, 50}},
			{name: "forced on without a source", set: "true", skills: sks(main), want: want{true, config.FromBuild, true, 60}},
			{name: "no source", skills: sks(main), want: want{false, config.Default, false, 50}},
			{name: "by chance, not set", skills: sks(main, spellOf(1, "physical", stat(chance, 25))), want: want{false, config.Default, false, 50}},
			{name: "by chance, set on", set: "true", skills: sks(main, spellOf(1, "physical", stat(chance, 25))), want: want{true, config.FromBuild, false, 60}},
			{name: "on every hit by a chance of 100%", skills: sks(main, spellOf(1, "physical", stat(chance, 100))), want: want{true, config.Derived, false, 60}},
			{
				name: "the strongest of two skills", skills: sks(spellOf(0, e.name, stat(magnitude, -12)), inflicts),
				want: want{true, config.Derived, false, 65},
			},
			{
				name: "the strongest of a chance set on and a magnitude", set: "true",
				skills: sks(main, spellOf(1, "physical", stat(chance, 25), stat(magnitude, -15))), want: want{true, config.FromBuild, false, 65},
			},
			{
				name: "at least the minimum", node: []resolve.StatValue{stat("exposure_you_inflict_has_minimum_resistance_%", -20)},
				skills: sks(main, inflicts), want: want{true, config.Derived, false, 70},
			},
			{
				name: "the minimum after the extra", skills: sks(main, inflicts),
				node: []resolve.StatValue{
					stat("exposure_you_inflict_applies_extra_%_to_affected_resistance", -10), stat("exposure_you_inflict_has_minimum_resistance_%", -20),
				},
				want: want{true, config.Derived, false, 75},
			},
			{
				name: "no minimum without Exposure", node: []resolve.StatValue{stat("exposure_you_inflict_has_minimum_resistance_%", -20)},
				skills: sks(main), want: want{false, config.Default, false, 50},
			},
		} {
			t.Run(e.name+", "+c.name, func(t *testing.T) {
				t.Parallel()
				values := config.Values{}
				if c.set != "" {
					values[e.key.Name()] = json.RawMessage(c.set)
				}
				cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: values})
				if len(problems) != 0 {
					t.Fatal(problems)
				}
				sh, err := poe1.Open(resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.node...)), table(t), cfg)
				if err != nil {
					t.Fatal(err)
				}
				setSkills(t, sh, c.skills...)
				derived, warnings, err := sh.DeriveConditions(cfg)
				if err != nil {
					t.Fatal(err)
				}
				on, src := config.Lookup(derived, e.key)
				warns := slices.ContainsFunc(warnings, func(w build.Warning) bool {
					return w.Code == poe1.NoConditionSource && w.Subject == "config "+e.key.Name()
				})
				got := want{on, src, warns, outputOf(t, sh, e.read)}
				if got.on != c.want.on || got.src != c.want.src || got.warns != c.want.warns || math.Abs(got.minimum-c.want.minimum) > 1e-9 {
					t.Errorf("got %+v, want %+v; warnings %v", got, c.want, warnings)
				}
			})
		}
	}
}
