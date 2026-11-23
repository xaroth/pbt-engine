package pob_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/pob"
)

// counts are PoB's count inputs and the config key each imports into.
func counts() map[string]string {
	return map[string]string{
		"configResonanceCount": "lowest-resonance", "multiplierWitheredStackCount": "enemy-withered-stacks",
		"minionsOverrideFrenzyCharges": "minion-frenzy-charge-count",
	}
}

func TestFromXMLCount(t *testing.T) {
	t.Parallel()
	for input, key := range counts() {
		for _, c := range []struct {
			name, number string
			want         json.RawMessage
			warnings     []build.Warning
		}{
			{name: "a count", number: "15", want: json.RawMessage("15")},
			{
				name: "not a count", number: "-5",
				warnings: []build.Warning{{
					Code: pob.ConfigIgnored, Subject: input,
					Message: `config input ` + input + ` value "-5" is not a value the config takes; not imported`,
				}},
			},
		} {
			t.Run(input+" "+c.name, func(t *testing.T) {
				t.Parallel()
				in := `<Input name="` + input + `" number="` + c.number + `"/>`
				b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, `<ConfigSet id="1">`+in+`</ConfigSet>`)
				want := values(t, "kill-all", "")
				if c.want != nil {
					want[key] = c.want
				}
				if diff := cmp.Diff(want, b.Config); diff != "" {
					t.Errorf("config (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(c.warnings, w, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("warnings (-want +got):\n%s", diff)
				}
			})
		}
	}
}

func TestToXMLCountRoundTrip(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	b := chieftain(t)
	for _, key := range counts() {
		b.Config[key] = json.RawMessage("12")
	}
	file, warnings, err := pob.ToXML(b, reg)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("export: %v, warnings %v", err, warnings)
	}
	got, warnings, err := pob.FromXML(file, reg)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("import: %v, warnings %v", err, warnings)
	}
	if diff := cmp.Diff(b.Config, got.Config, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}
