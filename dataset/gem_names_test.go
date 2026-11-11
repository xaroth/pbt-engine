package dataset_test

import (
	"errors"
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
			g, e, err := s.GemByName(testCase.name)
			if err != nil {
				t.Fatal(err)
			}
			if g.ID != testCase.gem || e.ID != testCase.effect {
				t.Errorf("got %s %s, want %s %s", g.ID, e.ID, testCase.gem, testCase.effect)
			}
		})
	}
	for _, name := range []string{"Fireball Support", "Monster Slam", "fireball"} {
		if _, _, err := s.GemByName(name); !errors.Is(err, dataset.ErrNotInDataset) {
			t.Errorf("%q: got %v, want ErrNotInDataset", name, err)
		}
	}
	plain, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := plain.GemByName("Fireball"); !errors.As(err, new(*dataset.MissingGroupError)) {
		t.Errorf("without skills: got %v, want a MissingGroupError", err)
	}
}
