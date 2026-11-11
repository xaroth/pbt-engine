package dataset_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestGemByName(t *testing.T) {
	t.Parallel()
	d := withSkills()
	d.Texts[0].GemNames = append(d.Texts[0].GemNames,
		dataset.Name{ID: "Metadata/Items/Gems/SkillGemVaalFireball", Name: "Vaal Fireball"},
		dataset.Name{ID: "Metadata/Items/Gems/SupportGemSpellTotem", Name: "Spell Totem Support"})
	d.Texts[0].SkillNames = append(d.Texts[0].SkillNames, dataset.Name{ID: "monster_slam", Name: "Monster Slam"})
	s, err := dataset.Open(written(t, d))
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		name, gem, effect string
	}{
		{"Fireball", "Metadata/Items/Gems/SkillGemFireball", "Fireball"},
		{"Fireball of Something", "Metadata/Items/Gems/SkillGemFireball", "FireballAltX"},
		{"Vaal Fireball", "Metadata/Items/Gems/SkillGemVaalFireball", "VaalFireball"},
		{"Spell Totem Support", "Metadata/Items/Gems/SupportGemSpellTotem", "SupportSpellTotem"},
		{"Spell Totem", "Metadata/Items/Gems/SupportGemSpellTotem", "SupportSpellTotem"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			m, err := s.GemByName(testCase.name)
			if err != nil {
				t.Fatal(err)
			}
			if m.Gem.ID != testCase.gem || m.Effect.ID != testCase.effect || len(m.Others) != 0 {
				t.Errorf("got %s %s %v, want %s %s", m.Gem.ID, m.Effect.ID, m.Others, testCase.gem, testCase.effect)
			}
		})
	}
	for _, name := range []string{"Fireball Support", "Monster Slam", "fireball"} {
		if _, err := s.GemByName(name); !errors.Is(err, dataset.ErrNotInDataset) {
			t.Errorf("%q: got %v, want ErrNotInDataset", name, err)
		}
	}
	plain, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.GemByName("Fireball"); !errors.As(err, new(*dataset.MissingGroupError)) {
		t.Errorf("without skills: got %v, want a MissingGroupError", err)
	}
}

// duplicateGem is a gem of withDuplicates: its id after Metadata/Items/Gems/, English name,
// gem effects (each granting the granted effect of the same id), and whether they take quality.
type duplicateGem struct {
	id, name string
	effects  []string
	quality  bool
}

// withDuplicates is sample with gems that share a name, as in 3.29.3.3's files, in the game's
// order except where noted, and one pair the choice cannot decide.
func withDuplicates() *dataset.Dataset {
	gems := []duplicateGem{
		// A superseded effect, first in the game's order.
		{"SkillGemConvocation", "Convocation", []string{"ConvocationOld"}, true},
		{"SkillGemConvocationNew", "Convocation", []string{"Convocation"}, true},
		// Two items of one gem effect.
		{"SkillGemNewArcticArmour", "Arctic Armour", []string{"ArcticArmour"}, true},
		{"SkillGemArcticArmour", "Arctic Armour", []string{"ArcticArmour"}, true},
		// The item with the transfigured variants, a copy of its first, and the Royale gem.
		{"SkillGemLightningTendrils", "Lightning Tendrils", []string{"LightningTendrils", "LightningTendrilsAltX", "LightningTendrilsAltY"}, true},
		{"SkillGemLightningTendrilsChannelled", "Lightning Tendrils", []string{"LightningTendrils"}, true},
		{"SkillGemLightningTendrilsRoyale", "Lightning Tendrils", []string{"LightningTendrilsRoyale"}, false},
		// A Royale support, put before the regular one so the game's order does not decide.
		{"SupportGemAddedColdDamageRoyale", "Added Cold Damage Support", []string{"SupportAddedColdDamageRoyale"}, false},
		{"SupportGemAddedColdDamage", "Added Cold Damage Support", []string{"SupportAddedColdDamage"}, true},
		// Not from the game: two current gems of one name with different effects.
		{"SkillGemTwinA", "Twin", []string{"TwinA"}, true},
		{"SkillGemTwinB", "Twin", []string{"TwinB"}, true},
	}
	d := sample()
	d.Skills = &dataset.Skills{Types: []string{"Spell"}}
	seen := map[string]bool{}
	for _, g := range gems {
		dg := dataset.Gem{ID: "Metadata/Items/Gems/" + g.id, Colour: dataset.Blue, Support: strings.HasPrefix(g.id, "Support")}
		for _, e := range g.effects {
			dg.Effects = append(dg.Effects, dataset.GemEffect{ID: e, GrantedEffect: e, Style: dataset.DefaultStyle})
			if seen[e] {
				continue
			}
			seen[e] = true
			ge := dataset.GrantedEffect{ID: e, Support: dg.Support, Levels: []dataset.EffectLevel{{Level: 1}}}
			if g.quality {
				ge.QualityStats = []dataset.StatValue{{Stat: 0, Value: 1000}}
			}
			d.Skills.GrantedEffects = append(d.Skills.GrantedEffects, ge)
		}
		d.Skills.Gems = append(d.Skills.Gems, dg)
		d.Texts[0].GemNames = append(d.Texts[0].GemNames, dataset.Name{ID: dg.ID, Name: g.name})
	}
	return d
}

func TestGemByNameDuplicates(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, withDuplicates()))
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		name, gem, effect string
		others            []string
	}{
		{"Convocation", "SkillGemConvocationNew", "Convocation", nil},
		{"Arctic Armour", "SkillGemNewArcticArmour", "ArcticArmour", nil},
		{"Lightning Tendrils", "SkillGemLightningTendrils", "LightningTendrils", nil},
		{"Added Cold Damage", "SupportGemAddedColdDamage", "SupportAddedColdDamage", nil},
		{"Twin", "SkillGemTwinA", "TwinA", []string{"Metadata/Items/Gems/SkillGemTwinB"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			m, err := s.GemByName(testCase.name)
			if err != nil {
				t.Fatal(err)
			}
			if m.Gem.ID != "Metadata/Items/Gems/"+testCase.gem || m.Effect.ID != testCase.effect || !slices.Equal(m.Others, testCase.others) {
				t.Errorf("got %s %s, others %v; want %s %s, others %v", m.Gem.ID, m.Effect.ID, m.Others, testCase.gem, testCase.effect, testCase.others)
			}
		})
	}
	sk, err := s.Skills()
	if err != nil {
		t.Fatal(err)
	}
	if m, err := sk.GemByGrantedEffect("LightningTendrils"); err != nil || m.Gem.ID != "Metadata/Items/Gems/SkillGemLightningTendrils" || len(m.Others) != 0 {
		t.Errorf("by granted effect: %+v, %v", m, err)
	}
	if _, err := sk.GemByGrantedEffect("Nothing"); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("unknown granted effect: %v", err)
	}
}

// TestGemByNameRealDuplicates resolves every gem name that more than one gem of the real
// dataset has, when $PBT_DATASETS holds 3.29.3.3 with a skills group: none is ambiguous or a
// Royale gem, and the replaced gems lose to the current ones.
func TestGemByNameRealDuplicates(t *testing.T) {
	t.Parallel()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		t.Skip("PBT_DATASETS not set")
	}
	reg, err := dataset.OpenRoot(os.DirFS(root))
	if reg == nil {
		t.Fatal(err)
	}
	res, err := reg.Resolve("poe1", "3.29.3.3")
	if err != nil || res.Store.Manifest().Version != "3.29.3.3" {
		t.Skip("PBT_DATASETS has no 3.29.3.3")
	}
	s := res.Store
	if _, err := s.Skills(); errors.As(err, new(*dataset.MissingGroupError)) {
		t.Skip("3.29.3.3 has no skills group")
	}
	text, err := s.Text("en")
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, n := range text.GemNames {
		count[n.Name]++
	}
	dupes := 0
	for name, c := range count {
		if c < 2 {
			continue
		}
		dupes++
		m, err := s.GemByName(name)
		if err != nil || len(m.Others) > 0 || strings.HasSuffix(m.Gem.ID, "Royale") {
			t.Errorf("%s: %+v, %v", name, m, err)
		}
	}
	t.Logf("%d names of more than one gem", dupes)
	for name, want := range map[string]string{
		"Convocation": "SkillGemConvocationNew", "Arctic Armour": "SkillGemNewArcticArmour",
		"Phase Run": "SkillGemNewPhaseRun", "Lightning Tendrils": "SkillGemLightningTendrils",
	} {
		if m, err := s.GemByName(name); err != nil || m.Gem.ID != "Metadata/Items/Gems/"+want {
			t.Errorf("%s: %+v, %v, want %s", name, m, err, want)
		}
	}
}
