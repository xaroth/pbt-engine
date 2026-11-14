package poe1_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/output"
)

func TestEnemyOf(t *testing.T) {
	t.Parallel()
	levels := []dataset.MonsterLevel{{Level: 60, Armour: 600, Evasion: 6000}, {Level: 85, Armour: 850, Evasion: 8500}}
	pinnacle := poe1.EnemyStats{Kind: poe1.EnemyPinnacle, Level: 84, FireResistance: 50, ColdResistance: 50, LightningResistance: 50,
		ChaosResistance: 30, Armour: 61989, Evasion: 10143}
	testCases := []struct {
		name      string
		build     config.Values
		character int
		want      poe1.EnemyStats
	}{
		{name: "default is the pinnacle boss", character: 60, want: pinnacle},
		{name: "pinnacle", build: config.Values{"enemy": raw(`"pinnacle"`)}, character: 1, want: pinnacle},
		{name: "uber", build: config.Values{"enemy": raw(`"uber"`)}, character: 60, want: poe1.EnemyStats{Kind: poe1.EnemyUber,
			Level: 85, FireResistance: 50, ColdResistance: 50, LightningResistance: 50, ChaosResistance: 30, Armour: 55739, Evasion: 9778}},
		{name: "boss at the character's level", build: config.Values{"enemy": raw(`"boss"`)}, character: 60, want: poe1.EnemyStats{
			Kind: poe1.EnemyBoss, Level: 60, FireResistance: 40, ColdResistance: 40, LightningResistance: 40, ChaosResistance: 25, Armour: 600, Evasion: 6000}},
		{name: "monster at most level 85", build: config.Values{"enemy": raw(`"none"`)}, character: 100,
			want: poe1.EnemyStats{Kind: poe1.EnemyNone, Level: 85, Armour: 850, Evasion: 8500}},
		{name: "set values win over the kind's", character: 90,
			build: config.Values{"enemy": raw(`"none"`), "enemy-level": raw(`60`), "enemy-fire-resistance": raw(`-20`),
				"enemy-chaos-resistance": raw(`75`), "enemy-evasion": raw(`0`)},
			want: poe1.EnemyStats{Kind: poe1.EnemyNone, Level: 60, FireResistance: -20, ChaosResistance: 75, Armour: 600, Evasion: 0}},
		{name: "pinnacle armour and evasion stay at another level", character: 90,
			build: config.Values{"enemy-level": raw(`60`), "enemy-armour": raw(`1000`)},
			want: poe1.EnemyStats{Kind: poe1.EnemyPinnacle, Level: 60, FireResistance: 50, ColdResistance: 50, LightningResistance: 50,
				ChaosResistance: 30, Armour: 1000, Evasion: 10143}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: testCase.build})
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			got, err := poe1.EnemyOf(cfg, testCase.character, levels)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("enemy (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnemyOfLevelNotInDataset(t *testing.T) {
	t.Parallel()
	cfg, _ := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: config.Values{"enemy": raw(`"boss"`)}})
	if _, err := poe1.EnemyOf(cfg, 70, nil); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("got %v, want ErrNotInDataset", err)
	}
}

func TestEnemyKeysRejectBadValues(t *testing.T) {
	t.Parallel()
	for name, v := range map[string]string{"enemy": `"shaper"`, "enemy-level": `0`, "enemy-armour": `-1`, "enemy-evasion": `-1`} {
		_, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: config.Values{name: raw(v)}})
		if len(problems) != 1 || !errors.Is(problems[0].Err, poe1.ErrConfigValue) {
			t.Errorf("%s %s: problems %v, want one ErrConfigValue", name, v, problems)
		}
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// An uber pinnacle boss takes 70% less damage of every type, after its resistances.
func TestUberTakesLessDamage(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	setSkills(t, sh, elementalSpell())
	sh.SetEnemy(poe1.EnemyStats{Kind: poe1.EnemyUber, FireResistance: 50})
	for k, want := range map[output.Key]float64{poe1.FireMinimum: 15, poe1.ChaosMinimum: 30, poe1.AverageHit: 15 + 30 + 30 + 30} {
		if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	sh.SetEnemy(poe1.EnemyStats{Kind: poe1.EnemyPinnacle, FireResistance: 50})
	if got := outputOf(t, sh, poe1.FireMinimum); got != 50 {
		t.Errorf("fire minimum against a pinnacle boss: got %v, want 50", got)
	}
}
