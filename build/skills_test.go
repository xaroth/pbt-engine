package build_test

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
)

func fireball() build.GemID { return build.GemID{ID: "Metadata/Items/Gems/SkillGemFireball"} }
func inspiration() build.GemID {
	return build.GemID{ID: "Metadata/Items/Gems/SkillGemSupportInspiration"}
}
func clarity() build.GemID { return build.GemID{ID: "Metadata/Items/Gems/SkillGemClarity"} }

func fireballAlt() build.GemID {
	return build.GemID{ID: "Metadata/Items/Gems/SkillGemFireball", Effect: "FireballAltX"}
}

// withSkills is juggernaut with two groups: Fireball with Inspiration twice in the helmet, the
// main skill, and Clarity in no item.
func withSkills() *build.Build {
	b := juggernaut()
	b.Skills = build.Skills{Groups: []build.SkillGroup{
		{Slot: build.SlotHelmet, Gems: []build.Gem{
			{GemID: fireball(), Level: 20, Quality: 20},
			{GemID: inspiration(), Level: 20, Support: true},
			{GemID: inspiration(), Level: 18, Support: true, Disabled: true},
		}},
		{Gems: []build.Gem{{GemID: clarity(), Level: 10}}},
	}}
	return b
}

// TestSkillEdits checks for each skill edit that it applies, that Diff gives exactly it, that
// the Diff back reverts it, and that it round-trips through JSON.
func TestSkillEdits(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		edit   build.Edit
		change func(s *build.Skills)
	}{
		{"add a group", build.AddSkillGroup(build.SkillGroup{Slot: build.SlotBoots, Gems: []build.Gem{{GemID: fireballAlt(), Level: 1}}}), func(s *build.Skills) {
			s.Groups = append(s.Groups, build.SkillGroup{Slot: build.SlotBoots, Gems: []build.Gem{{GemID: fireballAlt(), Level: 1}}})
		}},
		{"remove a group", build.RemoveSkillGroup(1), func(s *build.Skills) { s.Groups = s.Groups[:1] }},
		{"set a slot", build.SetSkillSlot(1, build.SlotGloves), func(s *build.Skills) { s.Groups[1].Slot = build.SlotGloves }},
		{"clear a slot", build.SetSkillSlot(0, ""), func(s *build.Skills) { s.Groups[0].Slot = "" }},
		{"set the main skill", build.SetMainSkill(1), func(s *build.Skills) { s.MainGroup = 1 }},
		{"add a gem", build.AddGem(1, build.Gem{GemID: inspiration(), Level: 5, Support: true}), func(s *build.Skills) {
			s.Groups[1].Gems = append(s.Groups[1].Gems, build.Gem{GemID: inspiration(), Level: 5, Support: true})
		}},
		{"remove a gem", build.RemoveGem(0, 2), func(s *build.Skills) { s.Groups[0].Gems = s.Groups[0].Gems[:2] }},
		{"set the active gem", build.SetActiveGem(0, 1), func(s *build.Skills) { s.Groups[0].Active = 1 }},
		{"set a gem level", build.SetGemLevel(0, 0, 21), func(s *build.Skills) { s.Groups[0].Gems[0].Level = 21 }},
		{"set a gem quality", build.SetGemQuality(0, 0, 23), func(s *build.Skills) { s.Groups[0].Gems[0].Quality = 23 }},
		{"disable a gem", build.SetGemEnabled(0, 1, false), func(s *build.Skills) { s.Groups[0].Gems[1].Disabled = true }},
		{"enable a gem", build.SetGemEnabled(0, 2, true), func(s *build.Skills) { s.Groups[0].Gems[2].Disabled = false }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			want := withSkills()
			testCase.change(&want.Skills)
			checkSkillEdit(t, testCase.edit, want)
		})
	}
}

// checkSkillEdit checks that e turns withSkills into want, that Diff gives exactly e, that the
// Diff back reverts it, and that e round-trips through JSON.
func checkSkillEdit(t *testing.T, e build.Edit, want *build.Build) {
	t.Helper()
	got := withSkills()
	if err := got.Apply(e); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("apply (-want +got):\n%s", diff)
	}
	edits, err := build.Diff(withSkills(), want)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]build.Edit{e}, edits); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
	back, err := build.Diff(want, withSkills())
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Apply(back...); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(withSkills(), got); diff != "" {
		t.Errorf("revert (-want +got):\n%s", diff)
	}
	checkEditJSON(t, e)
}

func checkEditJSON(t *testing.T, e build.Edit) {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded build.Edit
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(e, decoded); diff != "" {
		t.Errorf("JSON %s (-want +got):\n%s", data, diff)
	}
}

func TestSkillEditJSON(t *testing.T) {
	t.Parallel()
	edits := []build.Edit{
		build.SetGemLevel(0, 2, 21), build.SetGemEnabled(1, 0, false), build.SetMainSkill(0),
		build.AddGem(1, build.Gem{GemID: fireballAlt(), Level: 1, Support: false}),
	}
	data, err := json.Marshal(edits)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"op":"set-gem-level","level":21,"skill":{"group":0,"gem":2}},{"op":"set-gem-enabled","skill":{"group":1,"gem":0}},` +
		`{"op":"set-main-skill","skill":{"group":0,"gem":0}},` +
		`{"op":"add-gem","skill":{"group":1,"gem":0},"gem":{"id":"Metadata/Items/Gems/SkillGemFireball","effect":"FireballAltX","level":1}}]`
	if string(data) != want {
		t.Errorf("got  %s\nwant %s", data, want)
	}
}

// Removing a group or gem keeps the main skill and the active gem on theirs, or moves them to
// the first when it is the one removed.
func TestSkillRemovalsShift(t *testing.T) {
	t.Parallel()
	b := withSkills()
	b.Skills.MainGroup, b.Skills.Groups[0].Active = 1, 2
	if err := b.Apply(build.RemoveGem(0, 0), build.RemoveSkillGroup(0)); err != nil {
		t.Fatal(err)
	}
	if b.Skills.MainGroup != 0 || len(b.Skills.Groups) != 1 || b.Skills.Groups[0].Gems[0].GemID != clarity() {
		t.Errorf("after removing group 0 the main skill is %d of %+v, want Clarity's group at 0", b.Skills.MainGroup, b.Skills.Groups)
	}
	b = withSkills()
	b.Skills.Groups[0].Active = 2
	if err := b.Apply(build.RemoveGem(0, 1)); err != nil || b.Skills.Groups[0].Active != 1 {
		t.Errorf("active %d after removing gem 1 (%v), want 1", b.Skills.Groups[0].Active, err)
	}
	if err := b.Apply(build.RemoveGem(0, 1)); err != nil || b.Skills.Groups[0].Active != 0 {
		t.Errorf("active %d after removing the active gem (%v), want 0", b.Skills.Groups[0].Active, err)
	}
}

func TestSkillEditTargets(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		edit build.Edit
	}{
		{"group past the end", build.SetSkillSlot(2, build.SlotBoots)},
		{"negative group", build.RemoveSkillGroup(-1)},
		{"gem past the end", build.SetGemLevel(1, 1, 3)},
		{"gem in a missing group", build.RemoveGem(5, 0)},
		{"add a gem to a missing group", build.AddGem(2, build.Gem{GemID: clarity()})},
		{"no address", build.Edit{Op: build.OpSetGemQuality}},
		{"add a group without one", build.Edit{Op: build.OpAddSkillGroup}},
		{"add a gem without one", build.Edit{Op: build.OpAddGem, Skill: &build.SkillAt{}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := withSkills()
			if err := b.Apply(testCase.edit); !errors.Is(err, build.ErrEditTarget) {
				t.Errorf("got %v, want ErrEditTarget", err)
			}
			if diff := cmp.Diff(withSkills(), b); diff != "" {
				t.Errorf("failed edit changed the build (-want +got):\n%s", diff)
			}
		})
	}
	// The main skill and the active gem are values; Check judges them.
	b := withSkills()
	if err := b.Apply(build.SetMainSkill(7), build.SetActiveGem(1, 4)); err != nil || b.Skills.MainGroup != 7 || b.Skills.Groups[1].Active != 4 {
		t.Errorf("out of range main skill and active gem: %v, %+v", err, b.Skills)
	}
}

// The handles write the API's skill lines as edits on the build.
func TestSkillHandles(t *testing.T) {
	t.Parallel()
	next := withSkills()
	group := next.Skills.Main()
	group.Gem(fireball()).SetLevel(21).SetQuality(23)
	group.AddSupport(clarity(), 20, 20).SetEnabled(false)
	next.Skills.AddGroup(build.SkillGroup{}).SetSlot(build.SlotBoots).SetMain().AddGem(fireballAlt(), 1, 0).SetActive()
	next.Skills.Group(0).Gem(inspiration()).Remove()
	if group.Err() != nil {
		t.Fatal(group.Err())
	}
	want := withSkills()
	want.Skills.Groups = append(want.Skills.Groups, build.SkillGroup{Slot: build.SlotBoots, Gems: []build.Gem{{GemID: fireballAlt(), Level: 1}}})
	want.Skills.MainGroup = 2
	want.Skills.Groups[0].Gems = []build.Gem{
		{GemID: fireball(), Level: 21, Quality: 23},
		{GemID: inspiration(), Level: 18, Support: true, Disabled: true},
		{GemID: clarity(), Level: 20, Quality: 20, Support: true, Disabled: true},
	}
	if diff := cmp.Diff(want, next); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}

	empty := juggernaut()
	ref := empty.Skills.Main().Gem(fireball()).SetLevel(21)
	if !errors.Is(ref.Err(), build.ErrEditTarget) {
		t.Errorf("gem of a build without groups: %v, want ErrEditTarget", ref.Err())
	}
	missing := withSkills().Skills.Group(1).Gem(fireball())
	if !errors.Is(missing.SetQuality(1).Err(), build.ErrEditTarget) {
		t.Errorf("gem not in the group: %v, want ErrEditTarget", missing.Err())
	}
	if diff := cmp.Diff(juggernaut(), empty, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("failed handle edits changed the build (-want +got):\n%s", diff)
	}
}

func TestSkillsCloneAndRoundTrip(t *testing.T) {
	t.Parallel()
	b := withSkills()
	c := b.Clone()
	c.Skills.Groups[0].Gems[0].Level = 1
	c.Skills.Groups[1].Slot = build.SlotBoots
	if diff := cmp.Diff(withSkills(), b); diff != "" {
		t.Errorf("editing the clone changed the original (-want +got):\n%s", diff)
	}
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	b.Nodes = []uint32{1, 3, 10}
	if diff := cmp.Diff(b, got); diff != "" {
		t.Errorf("Parse(Encode) (-want +got):\n%s\n%s", diff, data)
	}
}

func randSkills(r *rand.Rand) build.Skills {
	gems := []build.GemID{fireball(), fireballAlt(), inspiration(), clarity()}
	var s build.Skills
	for range r.IntN(4) {
		g := build.SkillGroup{Slot: []build.Slot{"", build.SlotHelmet}[r.IntN(2)]}
		for range r.IntN(4) {
			g.Gems = append(g.Gems, build.Gem{
				GemID: gems[r.IntN(len(gems))], Level: 1 + r.IntN(3), Quality: r.IntN(2), Support: r.IntN(2) == 0, Disabled: r.IntN(3) == 0,
			})
		}
		if len(g.Gems) > 0 {
			g.Active = r.IntN(len(g.Gems))
		}
		s.Groups = append(s.Groups, g)
	}
	if len(s.Groups) > 0 {
		s.MainGroup = r.IntN(len(s.Groups))
	}
	return s
}
