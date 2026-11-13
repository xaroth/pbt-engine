package dataset_test

import (
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

// withGrantedSkills returns withItems with withSkills's skills group, a class's unarmed damage,
// a mod that grants two skills and one of its own in Ruthless, and a node that grants one.
func withGrantedSkills() *dataset.Dataset {
	d := withItems()
	sk := withSkills()
	d.Stats, d.Skills = sk.Stats, sk.Skills
	d.Characters[0].UnarmedDamage = dataset.ValueRange{Min: 2, Max: 8}
	d.Characters[0].UnarmedAttackTime = 833
	d.Mods[0].GrantedSkills = []dataset.GrantedSkill{{Effect: "Fireball", Level: 20}, {Effect: "FireballAltX", Level: 1}}
	d.Mods[0].RuthlessGrantedSkills = []dataset.GrantedSkill{{Effect: "FireballAltX", Level: 1}}
	d.Trees[0].Nodes[1].GrantedSkill = &dataset.GrantedSkill{Effect: "Fireball", Level: 1}
	return d
}

func TestGrantedSkillsRoundTrip(t *testing.T) {
	t.Parallel()
	d := withGrantedSkills()
	dir := t.TempDir()
	m, err := dataset.Write(dir, d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := readAll(os.DirFS(dir))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	d.Manifest = m
	if diff := gocmp.Diff(d, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestGrantedSkillsWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "mod grants an unknown effect", want: `mod IncreasedLife1: granted effect "Frostbolt" not in skills`, change: func(d *dataset.Dataset) {
			d.Mods[0].GrantedSkills[1].Effect = "Frostbolt"
		}},
		{name: "mod grants an unknown effect in Ruthless", want: `mod IncreasedLife1 in Ruthless: granted effect "Frostbolt" not in skills`,
			change: func(d *dataset.Dataset) {
				d.Mods[0].RuthlessGrantedSkills[0].Effect = "Frostbolt"
			}},
		{name: "node grants an unknown level", want: `tree standard node 2: granted effect "Fireball" has no level 2`, change: func(d *dataset.Dataset) {
			d.Trees[0].Nodes[1].GrantedSkill.Level = 2
		}},
		{name: "granted skill without skills group", want: "granted skill without skills", change: func(d *dataset.Dataset) {
			d.Skills = nil
		}},
		{name: "unarmed damage reversed", want: "unarmed damage reversed", change: func(d *dataset.Dataset) {
			d.Characters[0].UnarmedDamage.Min = 9
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withGrantedSkills()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

func TestModGrants(t *testing.T) {
	t.Parallel()
	standard := []dataset.GrantedSkill{{Effect: "Grace", Level: 10}}
	ruthless := []dataset.GrantedSkill{{Effect: "GraceHardMode", Level: 10}}
	testCases := []struct {
		name     string
		mod      dataset.Mod
		ruthless bool
		want     []dataset.GrantedSkill
	}{
		{name: "standard", mod: dataset.Mod{GrantedSkills: standard, RuthlessGrantedSkills: ruthless}, want: standard},
		{name: "ruthless with its own", mod: dataset.Mod{GrantedSkills: standard, RuthlessGrantedSkills: ruthless}, ruthless: true, want: ruthless},
		{name: "ruthless without its own", mod: dataset.Mod{GrantedSkills: standard}, ruthless: true, want: standard},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if diff := gocmp.Diff(testCase.want, testCase.mod.Grants(testCase.ruthless)); diff != "" {
				t.Errorf("Grants mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
