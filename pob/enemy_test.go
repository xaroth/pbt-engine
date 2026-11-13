package pob_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/pob"
)

// enemyConfig sets every enemy input PoB's Enemy Stats section shares with the config, inside
// a config set, beside a placeholder and an enemy input the config does not hold.
const enemyConfig = `<ConfigSet id="1">
	<Input name="enemyIsBoss" string="Uber"/>
	<Input name="enemyLevel" number="80"/>
	<Input name="enemyLightningResist" number="-10"/>
	<Input name="enemyColdResist" number="20"/>
	<Input name="enemyFireResist" number="0"/>
	<Input name="enemyChaosResist" number="75"/>
	<Input name="enemyEvasion" number="0"/>
	<Input name="enemyArmour" number="12000"/>
	<Input name="enemyCritChance" number="10"/>
	<Placeholder name="enemyArmour" number="61989"/>
</ConfigSet>`

func TestFromXMLEnemy(t *testing.T) {
	t.Parallel()
	b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, enemyConfig)
	want := values(t, poe1.BanditsKillAll, "")
	for name, v := range map[string]string{
		"enemy": `"uber"`, "enemy-level": "80", "enemy-lightning-resistance": "-10", "enemy-cold-resistance": "20",
		"enemy-fire-resistance": "0", "enemy-chaos-resistance": "75", "enemy-evasion": "0", "enemy-armour": "12000",
	} {
		want[name] = json.RawMessage(v)
	}
	if diff := cmp.Diff(want, b.Config); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
	wantWarnings := []build.Warning{{Code: pob.ConfigIgnored, Subject: "enemyCritChance", Message: "config input not modelled (10)"}}
	if diff := cmp.Diff(wantWarnings, w); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

func TestFromXMLEnemyKinds(t *testing.T) {
	t.Parallel()
	for pobValue, want := range map[string]poe1.EnemyKind{
		"None": poe1.EnemyNone, "Boss": poe1.EnemyBoss, "Pinnacle": poe1.EnemyPinnacle, "Uber": poe1.EnemyUber,
		"uber atziri": poe1.EnemyBoss, "Shaper": poe1.EnemyPinnacle, "SIRUS": poe1.EnemyPinnacle,
	} {
		b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, `<Input name="enemyIsBoss" string="`+pobValue+`"/>`)
		if got, _, err := config.Get(b.Config, poe1.Enemy); err != nil || got != want || len(w) != 0 {
			t.Errorf("%s: got %q, %v, warnings %v; want %q", pobValue, got, err, w, want)
		}
	}
}

func TestFromXMLEnemyBadValues(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		`<Input name="enemyIsBoss" string="Maven"/>`,
		`<Input name="enemyLevel" number="0"/>`,
		`<Input name="enemyArmour" number="-5"/>`,
		`<Input name="enemyFireResist" number="12.5"/>`,
		`<Input name="enemyChaosResist" string="high"/>`,
	} {
		b, w := fromXML(t, `className="Marauder"`, `treeVersion="3_29"`, in)
		if diff := cmp.Diff(values(t, poe1.BanditsKillAll, ""), b.Config); diff != "" || len(w) != 1 || w[0].Code != pob.ConfigIgnored {
			t.Errorf("%s: config (-want +got):\n%s\nwarnings %v", in, diff, w)
		}
	}
}

func TestToXMLEnemyRoundTrip(t *testing.T) {
	t.Parallel()
	reg := exportRegistry(t)
	b := chieftain(t)
	for k, v := range map[string]string{"enemy": `"boss"`, "enemy-level": "70", "enemy-fire-resistance": "-30", "enemy-armour": "0"} {
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
