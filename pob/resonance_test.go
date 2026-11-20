package pob_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/pob"
)

func TestFromXMLResonanceCount(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, input string
		want        json.RawMessage
		warnings    []build.Warning
	}{
		{name: "a count", input: `<Input name="configResonanceCount" number="25"/>`, want: json.RawMessage("25")},
		{
			name: "not a count", input: `<Input name="configResonanceCount" number="-5"/>`,
			warnings: []build.Warning{{
				Code: pob.ConfigIgnored, Subject: "configResonanceCount",
				Message: `config input configResonanceCount value "-5" is not a value the config takes; not imported`,
			}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, `<ConfigSet id="1">`+c.input+`</ConfigSet>`)
			want := values(t, "kill-all", "")
			if c.want != nil {
				want["lowest-resonance"] = c.want
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

func TestToXMLResonanceCountRoundTrip(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	b := chieftain(t)
	b.Config["lowest-resonance"] = json.RawMessage("30")
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
