package resolve_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

const (
	fireballGem      = "Metadata/Items/Gems/SkillGemFireball"
	freezingPulseGem = "Metadata/Items/Gems/SkillGemFreezingPulse"
	empowerGem       = "Metadata/Items/Gems/SupportGemAdditionalLevel"
	spellTotemGem    = "Metadata/Items/Gems/SupportGemSpellTotem"
	multiTotemGem    = "Metadata/Items/Gems/SupportGemMultiTotem"
	closeCombatGem   = "Metadata/Items/Gems/SupportGemCloseCombat"
	meleePhysicalGem = "Metadata/Items/Gems/SupportGemMeleePhysicalDamage"
	addedFireGem     = "Metadata/Items/Gems/SupportGemAddedFireDamage"
	spellEchoGem     = "Metadata/Items/Gems/SupportGemMulticast"
)

func active(id string, quality int) build.Gem {
	return build.Gem{ID: id, Level: 20, Quality: quality}
}

func support(id string, level int) build.Gem {
	return build.Gem{ID: id, Level: level, Support: true}
}

// mainSkill resolves the main skill of a level 90 marauder with group g, and items, on the
// current real dataset.
func mainSkill(t *testing.T, g build.SkillGroup, items build.Items, bonuses []resolve.GemBonus) (*resolve.Skill, []build.Warning) {
	t.Helper()
	b := testBuild(90)
	b.Version = realdata.Current
	b.Skills = build.Skills{Groups: []build.SkillGroup{g}}
	b.Items = items
	c, _, err := build.Check(b, realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.NewSkillResolver(c, bonuses)
	if err != nil {
		t.Fatal(err)
	}
	sk, ws, err := r.Group(c.Build().Skills.MainGroup)
	if err != nil {
		t.Fatal(err)
	}
	return sk, ws
}

// TestMainSkillFireball checks Fireball at level 20 with 20% quality against its dataset rows:
// the stat set's implicit and constant stats, the level 20 row, and 20 times the quality stat's
// 0.15 area radius per 1%.
func TestMainSkillFireball(t *testing.T) {
	t.Parallel()
	got, ws := mainSkill(t, build.SkillGroup{Gems: []build.Gem{active(fireballGem, 20)}}, nil, nil)
	want := &resolve.Skill{
		Source:        resolve.GemSource{ID: fireballGem},
		GrantedEffect: "Fireball", ActiveSkill: "fireball",
		Level: 20, Quality: 20,
		Types: []string{
			"Spell", "Projectile", "Damage", "Area", "Trappable", "Totemable", "Mineable", "Multicastable", "Fire",
			"Triggerable", "ProjectilesFromUser", "CanRapidFire", "AreaSpell",
		},
		CastTime: 750,
		Costs:    []dataset.Cost{{Type: "Mana", Amount: 20}},
		StatSets: []resolve.SkillStatSet{{
			ID: "Fireball", SpellCritChance: 500, DamageEffectiveness: 38000,
			Stats: []resolve.StatValue{
				{Stat: "base_is_projectile", Value: 1},
				{Stat: "quality_display_active_skill_ignite_damage_is_gem", Value: 1},
				{Stat: "base_chance_to_ignite_%", Value: 25},
				{Stat: "spell_minimum_base_fire_damage", Value: 1883},
				{Stat: "spell_maximum_base_fire_damage", Value: 2825},
				{Stat: "active_skill_base_area_of_effect_radius", Value: 18},
				{Stat: "active_skill_base_area_of_effect_radius", Value: 3},
			},
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Fireball 20/20 (-want +got):\n%s", diff)
	}
	if len(ws) > 0 {
		t.Errorf("warnings %v, want none", ws)
	}
}

func TestMainSkillSupports(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		gems     []build.Gem
		supports []string
		warned   []int
		reason   string
	}{
		{"applies by allowed types", []build.Gem{active(fireballGem, 0), support(addedFireGem, 20)}, []string{"SupportAddedFireDamage"}, nil, ""},
		{"lacks allowed types", []build.Gem{active(fireballGem, 0), support(meleePhysicalGem, 20)}, nil, []int{1}, "lacks the types"},
		{"weapon class not shared", []build.Gem{active(fireballGem, 0), support(closeCombatGem, 20)}, nil, []int{1}, "not used with"},
		{
			"types another support adds",
			[]build.Gem{active(freezingPulseGem, 0), support(multiTotemGem, 20), support(spellTotemGem, 20)},
			[]string{"SupportMultipleTotems", "SupportSpellTotem"}, nil, "",
		},
		{
			"excluded by types a later support adds",
			[]build.Gem{active(freezingPulseGem, 0), support(spellEchoGem, 20), support(spellTotemGem, 20)},
			[]string{"SupportSpellTotem"}, []int{1}, "excludes",
		},
		{
			"excluded by types an earlier support adds",
			[]build.Gem{active(freezingPulseGem, 0), support(spellTotemGem, 20), support(spellEchoGem, 20)},
			[]string{"SupportSpellTotem"}, []int{2}, "excludes",
		},
		{"without the support that adds types", []build.Gem{active(freezingPulseGem, 0), support(multiTotemGem, 20)}, nil, []int{1}, "lacks the types"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sk, ws := mainSkill(t, build.SkillGroup{Gems: testCase.gems}, nil, nil)
			var got []string
			for _, sp := range sk.Supports {
				got = append(got, sp.GrantedEffect)
			}
			if diff := cmp.Diff(testCase.supports, got); diff != "" {
				t.Errorf("supports (-want +got):\n%s", diff)
			}
			var subjects, want []string
			for _, w := range ws {
				if w.Code != resolve.SupportNotApplied {
					t.Errorf("warning %v, want only %s", w, resolve.SupportNotApplied)
				}
				subjects = append(subjects, w.Subject)
				if !strings.Contains(w.Message, testCase.reason) {
					t.Errorf("warning %v, want its reason to say %q", w, testCase.reason)
				}
			}
			for _, i := range testCase.warned {
				want = append(want, resolve.GemSource{Gem: i, ID: testCase.gems[i].ID}.String())
			}
			if diff := cmp.Diff(want, subjects); diff != "" {
				t.Errorf("warned gems (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMainSkillGemLevels checks levels from a socketed item stat and from a support: Empower at
// level 4 adds 3 levels to the gem it supports, and the item's levels for socketed supports
// take Empower beyond its highest row.
func TestMainSkillGemLevels(t *testing.T) {
	t.Parallel()
	bonuses := []resolve.GemBonus{
		{Stat: "local_socketed_support_gem_level_+", Reach: resolve.Socketed, Gems: resolve.GemFilter{Kind: resolve.SupportGem}},
		{Stat: "local_socketed_gem_quality_+", Quality: true, Reach: resolve.Socketed},
		{Stat: "supported_active_skill_gem_level_+", Reach: resolve.Supported, Gems: resolve.GemFilter{Kind: resolve.SkillGem}},
	}
	armour := build.Item{Rarity: build.Rare, Base: "Metadata/Items/Armours/BodyArmours/BodyStr1", Mods: []build.ModLine{{
		Kind: build.Explicit, Stats: []build.StatValue{{Stat: "local_socketed_support_gem_level_+", Value: 7}, {Stat: "local_socketed_gem_quality_+", Value: 5}},
	}}}
	testCases := []struct {
		name                    string
		slot                    build.Slot
		level, quality, empower int
		beyond                  bool
	}{
		{"not socketed in the item", build.SlotHelmet, 23, 0, 4, false},
		{"socketed in the item", build.SlotBodyArmour, 29, 5, 10, true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			g := build.SkillGroup{Slot: testCase.slot, Gems: []build.Gem{active(fireballGem, 0), support(empowerGem, 4)}}
			sk, ws := mainSkill(t, g, build.Items{build.SlotBodyArmour: armour}, bonuses)
			if sk.Level != testCase.level || sk.Quality != testCase.quality {
				t.Errorf("Fireball level %d quality %d, want %d and %d", sk.Level, sk.Quality, testCase.level, testCase.quality)
			}
			if len(sk.Supports) != 1 || sk.Supports[0].Level != testCase.empower {
				t.Fatalf("supports %+v, want Empower at level %d", sk.Supports, testCase.empower)
			}
			beyond := len(ws) == 1 && ws[0].Code == resolve.GemLevelBeyondData
			if beyond != testCase.beyond || len(ws) > 1 {
				t.Errorf("warnings %v, want level beyond data %t", ws, testCase.beyond)
			}
		})
	}
}

func TestGroupNoActiveSkill(t *testing.T) {
	t.Parallel()
	b := testBuild(90)
	b.Version = realdata.Current
	disabled := active(fireballGem, 0)
	disabled.Disabled = true
	b.Skills = build.Skills{Groups: []build.SkillGroup{{Gems: []build.Gem{disabled}}, {Gems: []build.Gem{support(empowerGem, 4)}}}}
	c, _, err := build.Check(b, realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.NewSkillResolver(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1, 2} {
		if _, _, err := r.Group(i); !errors.Is(err, resolve.ErrNoActiveSkill) {
			t.Errorf("group %d: got %v, want ErrNoActiveSkill", i+1, err)
		}
	}
}

// All resolves every enabled active gem outside the weapon swap, with each skill it has: Vaal
// Haste's own and Haste. A group's supports apply to each of its skills, and an active gem's
// support effect, Autoexertion's, supports the warcry beside it. A skill Group resolved is not
// resolved again.
func TestAll(t *testing.T) {
	t.Parallel()
	b := testBuild(90)
	b.Version = realdata.Current
	disabled := active(fireballGem, 0)
	disabled.Disabled = true
	b.Skills = build.Skills{Groups: []build.SkillGroup{
		{Gems: []build.Gem{active(fireballGem, 0), disabled, support(addedFireGem, 20), active(freezingPulseGem, 0)}},
		{Slot: build.SlotWeaponSwap, Gems: []build.Gem{active(fireballGem, 0)}},
		{Gems: []build.Gem{active("Metadata/Items/Gems/SkillGemVaalHaste", 0)}},
		{Gems: []build.Gem{active("Metadata/Items/Gems/SkillGemSeismicCry", 0), active("Metadata/Items/Gems/SkillGemCallToArms", 0)}},
	}}
	c, _, err := build.Check(b, realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.NewSkillResolver(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	main, _, err := r.Group(0)
	if err != nil {
		t.Fatal(err)
	}
	all := r.All()
	if all[0].Skill != main {
		t.Error("All resolved group 1's skill again; want the skill Group resolved")
	}
	type skill struct {
		group, gem  int
		effect      string
		supports    []string
		reservation int32
	}
	got := make([]skill, 0, len(all))
	for _, s := range all {
		sp := make([]string, 0, len(s.Skill.Supports))
		for _, x := range s.Skill.Supports {
			sp = append(sp, x.GrantedEffect)
		}
		got = append(got, skill{s.Skill.Source.Group, s.Skill.Source.Gem, s.Skill.GrantedEffect, sp, s.Skill.Reservation.ManaPercent})
	}
	want := []skill{
		{0, 0, "Fireball", []string{"SupportAddedFireDamage"}, 0},
		{0, 3, "FreezingPulse", []string{"SupportAddedFireDamage"}, 0},
		{2, 0, "VaalHaste", []string{}, 0},
		{2, 0, "Haste", []string{}, 5000},
		{3, 0, "SeismicCry", []string{"SupportAutoexertion"}, 2000},
		{3, 1, "Autoexertion", []string{}, 0},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(skill{})); diff != "" {
		t.Errorf("skills (-want +got):\n%s", diff)
	}
}
