package pob_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/pob"
)

func TestFromXMLConditions(t *testing.T) {
	t.Parallel()
	b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, `<ConfigSet id="1">
	<Input name="conditionEnemyShocked" boolean="true"/>
	<Input name="conditionShockEffect" number="25"/>
	<Input name="conditionEnemyColdExposure" boolean="true"/>
	<Input name="conditionFullLife" boolean="true"/>
	<Input name="buffFortification" boolean="false"/>
	<Input name="conditionEnemyChilled" boolean="maybe"/>
	<Input name="conditionBeenHitRecently" boolean="true"/>
</ConfigSet>`)
	want := values(t, poe1.BanditsKillAll, "")
	for name, v := range map[string]string{
		"enemy-shocked": "true", "enemy-shock-effect": "25", "enemy-cold-exposure": "true", "full-life": "true", "fortified": "false",
	} {
		want[name] = json.RawMessage(v)
	}
	if diff := cmp.Diff(want, b.Config); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{
		{
			Code: pob.ConfigIgnored, Subject: "conditionEnemyChilled",
			Message: `config input conditionEnemyChilled value "maybe" is not a value the config takes; not imported`,
		},
		{Code: pob.ConfigIgnored, Subject: "conditionBeenHitRecently", Message: "config input conditionBeenHitRecently not modelled (value true)"},
	}
	if diff := cmp.Diff(wantWarnings, w); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

func TestToXMLConditionsRoundTrip(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	b := chieftain(t)
	for k, v := range map[string]string{
		"enemy-unnerved": "true", "enemy-shocked": "true", "enemy-shock-effect": "30", "enemy-fire-exposure": "true", "leeching": "true",
	} {
		b.Config[k] = json.RawMessage(v)
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
