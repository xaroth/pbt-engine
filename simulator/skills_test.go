package simulator_test

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// Fireball with Added Fire Damage at 20% quality, whose quality gives 10% increased Fire
// Damage to the skill it supports: the main skill's fire part takes it, its cold part does not,
// and the sheet without the skill reads the default attack, which it does not support.
func TestMainSkillOutputs(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	bare := marauderBuild(90)
	b := bare.Clone()
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{
		{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20},
		{ID: "Metadata/Items/Gems/SupportGemAddedFireDamage", Level: 20, Quality: 20, Support: true},
	}})
	st := simulate(t, s, b)
	sk, ok := st.MainSkill()
	if !ok || sk.ID != "Metadata/Items/Gems/SkillGemFireball" || !slices.Contains(sk.Types, "Spell") {
		t.Fatalf("main skill %+v, %v", sk, ok)
	}
	if got := get(t, st, poe1.FireDamageModifier); math.Abs(got-110) > 1e-9 {
		t.Errorf("fire damage modifier %v, want 110", got)
	}
	if got := get(t, st, poe1.ColdDamageModifier); got != 100 {
		t.Errorf("cold damage modifier %v, want 100", got)
	}
	st0 := simulate(t, s, bare)
	if sk, ok := st0.MainSkill(); !ok || sk.GrantedEffect != "Melee" || sk.Group != -1 {
		t.Errorf("main skill without skills %+v, %v; want the default attack", sk, ok)
	}
	if got := get(t, st0, poe1.FireDamageModifier); got != 100 {
		t.Errorf("default attack's fire damage modifier %v, want 100", got)
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Stats", data)
	ex, err := st.Explain(poe1.FireDamageModifier, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Skill == nil || ex.Skill.Group != 0 || ex.Skill.Gem != 0 {
		t.Errorf("explanation skill %+v, want group 0 gem 0", ex.Skill)
	}
	if !slices.ContainsFunc(ex.Lines, func(l simulator.Line) bool { return l.Value == 10 && l.Source.Bundle != nil }) {
		t.Errorf("lines %+v, want the support's 10", ex.Lines)
	}
	data, err = json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Explanation", data)
}

// Skills that reserve take their share of the pools: a percentage of the maximum rounded up, a
// flat amount, both after the supports' multipliers (level rows: Enlighten 10 at 64%,
// Arrogance 20 at 201%, Added Fire Damage 20 at 120%). Arrogance moves the reservation to life,
// Eternal Blessing removes it, and Autoexertion's support makes Seismic Cry reserve its 20%.
// The main skill's mana cost is its level row's 20 with the support's multiplier, rounded down.
func TestReservation(t *testing.T) {
	t.Parallel()
	const gems = "Metadata/Items/Gems/"
	s := realSim(t)
	gem := func(id string, level int) build.Gem { return build.Gem{ID: gems + id, Level: level} }
	sup := func(id string, level int) build.Gem { return build.Gem{ID: gems + id, Level: level, Support: true} }
	pct := func(pool, permyriad float64) float64 { return math.Ceil(pool * permyriad / 10000) }
	testCases := []struct {
		name   string
		groups [][]build.Gem
		// life and mana reserved from the pools, given the maximum.
		life, mana func(life, mana float64) float64
	}{
		{
			name:   "percentage",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20)}},
			mana:   func(_, m float64) float64 { return pct(m, 5000) },
		},
		{
			name:   "flat and a multiplier",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20), sup("SupportGemAdditionalXP", 10)}, {gem("SkillGemPrecision", 20)}},
			mana:   func(_, m float64) float64 { return 186 + pct(m, 5000*0.64) },
		},
		{
			name:   "life instead of mana",
			groups: [][]build.Gem{{gem("SkillGemDetermination", 20), sup("SupportGemBloodMagic", 20)}},
			life:   func(l, _ float64) float64 { return pct(l, 5000*2.01) },
		},
		{
			name:   "no reservation",
			groups: [][]build.Gem{{gem("SkillGemHatred", 20), sup("SupportGemEternalBlessing", 20)}},
		},
		{
			name:   "a support's reservation",
			groups: [][]build.Gem{{gem("SkillGemSeismicCry", 20), gem("SkillGemCallToArms", 20)}},
			mana:   func(_, m float64) float64 { return pct(m, 2000) },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := marauderBuild(90)
			b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{gem("SkillGemFireball", 20), sup("SupportGemAddedFireDamage", 20)}})
			for _, g := range testCase.groups {
				b.Skills.AddGroup(build.SkillGroup{Gems: g})
			}
			st := simulate(t, s, b)
			life, mana := get(t, st, poe1.Life), get(t, st, poe1.Mana)
			var wantLife, wantMana float64
			if testCase.life != nil {
				wantLife = testCase.life(life, mana)
			}
			if testCase.mana != nil {
				wantMana = testCase.mana(life, mana)
			}
			for _, c := range []struct {
				key  output.Key
				want float64
			}{
				{poe1.ReservedLife, wantLife}, {poe1.UnreservedLife, life - wantLife},
				{poe1.ReservedMana, wantMana}, {poe1.UnreservedMana, mana - wantMana},
				{poe1.ManaCost, 24},
			} {
				if got := get(t, st, c.key); got != c.want {
					t.Errorf("%s %v, want %v", c.key, got, c.want)
				}
			}
			for _, k := range []output.Key{poe1.UnreservedMana, poe1.ManaCost} {
				if _, err := st.Explain(k, simulator.Proportional); err != nil {
					t.Errorf("explain %s: %v", k, err)
				}
			}
		})
	}
}

// Fireball 20 alone on a level 90 Marauder without passives: its level row's 1883 to 2825 fire
// damage, against the default enemy's 50% fire resistance, with the class's 5% critical strike
// chance and 150% multiplier, cast every 0.75 seconds. An enemy without fire resistance takes it whole. The skill is
// simulated, so it does not warn; an attack is not, so it gives no hit. Spell Totem 20 deals 50%
// less damage: 942 to 1413 before the enemy's resistance, rounded as damage is.
func TestSpellHitOutputs(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := marauderBuild(90)
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}})
	st := simulate(t, s, b)
	average := (1883 + 2825) / 2 * 0.5 * (0.95 + 0.05*1.5)
	for k, want := range map[output.Key]float64{
		poe1.FireMinimum: 941.5, poe1.FireMaximum: 1412.5, poe1.CriticalStrikeChance: 5,
		poe1.AverageHit: average, poe1.HitsPerSecond: 1 / 0.75, poe1.HitDPS: average / 0.75,
	} {
		if got := get(t, st, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	if slices.ContainsFunc(st.Warnings(), func(w simulator.Warning) bool { return w.Code == simulator.SkillsNotSimulated }) {
		t.Errorf("warnings %v, want none for a simulated spell", st.Warnings())
	}
	if got := get(t, simulate(t, s, b, simulator.Set(poe1.EnemyFireResistance, 0)), poe1.FireMinimum); got != 1883 {
		t.Errorf("fire minimum against 0%% fire resistance: got %v, want 1883", got)
	}
	totem := marauderBuild(90)
	totem.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{
		{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20},
		{ID: "Metadata/Items/Gems/SupportGemSpellTotem", Level: 20, Support: true},
	}})
	ts := simulate(t, s, totem)
	for k, want := range map[output.Key]float64{
		poe1.FireMinimum: 471, poe1.FireMaximum: 706.5, poe1.AverageHit: (471 + 706.5) / 2 * (0.95 + 0.05*1.5),
	} {
		if got := get(t, ts, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("with Spell Totem: %s: got %v, want %v", k, got, want)
		}
	}
	attack := marauderBuild(90)
	attack.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemCleave", Level: 20}}})
	if _, ok := simulate(t, s, attack).Get(poe1.AverageHit); ok {
		t.Error("an attack has an average hit")
	}
}

// With EverySkill a result gives each skill's outputs. Fireball is the main skill: its values are
// the outputs', which are those of a run without EverySkill. Freezing Pulse, in another group,
// has the values it has as the main skill; Haste, an aura, has a cost and no hit. A Vaal Flicker
// Strike choosing its Vaal skill's second stat set does not warn about its regular skill, which
// has one set.
func TestEverySkill(t *testing.T) {
	t.Parallel()
	const gems = "Metadata/Items/Gems/"
	s := realSim(t)
	b := marauderBuild(90)
	flicker := build.Gem{ID: gems + "SkillGemVaalFlickerStrike", Level: 20, StatSet: 1}
	for _, g := range [][]build.Gem{
		{{ID: gems + "SkillGemFireball", Level: 20}, {ID: gems + "SupportGemAddedFireDamage", Level: 20, Support: true}},
		{{ID: gems + "SkillGemFreezingPulse", Level: 18}},
		{{ID: gems + "SkillGemHaste", Level: 20}},
		{flicker},
	} {
		b.Skills.AddGroup(build.SkillGroup{Gems: g})
	}
	sheet := simulate(t, s, b)
	st := simulate(t, s, b, simulator.EverySkill())
	if len(sheet.Skills()) != 0 {
		t.Errorf("skills %v without EverySkill", sheet.Skills())
	}
	for _, k := range sheet.Keys() {
		if get(t, st, k) != get(t, sheet, k) {
			t.Errorf("%s %v with EverySkill, %v without", k, get(t, st, k), get(t, sheet, k))
		}
	}
	refs := make([]simulator.SkillRef, 0, len(st.Skills()))
	for _, ss := range st.Skills() {
		refs = append(refs, ss.Ref)
	}
	fireball, pulse, haste := simulator.SkillRef{}, simulator.SkillRef{Group: 1}, simulator.SkillRef{Group: 2}
	want := []simulator.SkillRef{fireball, pulse, haste, {Group: 3}, {Group: 3, Skill: 1}}
	if !slices.Equal(refs, want) {
		t.Fatalf("skills %v, want %v", refs, want)
	}
	sameAs := func(ref simulator.SkillRef, main *simulator.Stats) {
		t.Helper()
		ss := st.Skill(ref)
		var skillKeys []output.Key
		for _, o := range s.Outputs() {
			if o.IsSkill() && slices.Contains(main.Keys(), o.Key) {
				skillKeys = append(skillKeys, o.Key)
			}
		}
		if !slices.Equal(ss.Keys(), skillKeys) {
			t.Errorf("%v: outputs %v, want the main skill's %v", ref, ss.Keys(), skillKeys)
		}
		for _, k := range skillKeys {
			if v, _ := ss.Get(k); v != get(t, main, k) {
				t.Errorf("%v: %s %v, want %v", ref, k, v, get(t, main, k))
			}
		}
	}
	sameAs(fireball, sheet)
	asMain := b.Clone()
	asMain.Skills.MainGroup = 1
	sameAs(pulse, simulate(t, s, asMain))
	if _, ok := st.Skill(haste).Get(poe1.AverageHit); ok {
		t.Error("Haste has an average hit")
	}
	if _, ok := st.Skill(haste).Get(poe1.ManaCost); !ok {
		t.Error("Haste has no mana cost")
	}
	if slices.ContainsFunc(st.Warnings(), func(w simulator.Warning) bool { return w.Code == resolve.StatSetUnknown }) {
		t.Errorf("warnings %v, want none about Vaal Flicker Strike's stat set", st.Warnings())
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Stats", data)
}

// Compare and Sweep read a named skill's outputs: Freezing Pulse, outside the main group, gains
// average hit with its level, and a sweep ranked by it puts the higher level first and gives its
// changes on the fast path too.
func TestSkillChanges(t *testing.T) {
	t.Parallel()
	const gems = "Metadata/Items/Gems/"
	s := realSim(t)
	b := chieftainWithHelmet(t, s)
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: gems + "SkillGemFireball", Level: 20}}})
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: gems + "SkillGemFreezingPulse", Level: 10}}})
	pulse := simulator.SkillRef{Group: 1}
	up := b.Clone()
	if err := up.Apply(build.SetGemLevel(1, 0, 20)); err != nil {
		t.Fatal(err)
	}
	d, err := s.Compare(b, up, simulator.EverySkill())
	if err != nil {
		t.Fatal(err)
	}
	if c := d.GetSkill(pulse, poe1.AverageHit); !c.Has || c.Abs <= 0 {
		t.Errorf("Freezing Pulse's average hit from level 10 to 20: %+v, want a gain", c)
	}
	if c := d.Get(poe1.AverageHit); !c.Has || c.Abs != 0 {
		t.Errorf("main skill's average hit: %+v, want no change", c)
	}
	node := []build.Edit{build.Deallocate(b.Nodes[len(b.Nodes)-1])}
	if fast, err := simulator.FastPath(s, b, node); err != nil || !fast {
		t.Fatalf("deallocating a node: fast path %v, %v", fast, err)
	}
	results, err := s.Sweep(b, [][]build.Edit{{build.SetGemLevel(1, 0, 11)}, {build.SetGemLevel(1, 0, 20)}, {build.SetGemLevel(0, 0, 1)}, node},
		simulator.Outputs(poe1.Life), simulator.RankBySkill(pulse, poe1.AverageHit))
	if err != nil {
		t.Fatal(err)
	}
	if order := []int{results[0].Index, results[1].Index}; !slices.Equal(order, []int{1, 0}) {
		t.Errorf("ranked %v, want level 20 then 11", order)
	}
	for _, r := range results {
		if _, ok := r.SkillChanges[pulse][poe1.AverageHit]; !ok || r.Err != nil {
			t.Errorf("variant %d: %v, %v; want Freezing Pulse's average hit", r.Index, r.SkillChanges, r.Err)
		}
	}
	for def, v := range map[string]any{"Diff": d, "Result": results[0]} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, def, data)
	}
}

// A sweep ranked by a skill's output fails, naming what is wrong, for a key that is not a skill
// output and for a ref that names no skill of the base.
func TestRankBySkillErrors(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := chieftainWithHelmet(t, s)
	variants := [][]build.Edit{{build.Deallocate(b.Nodes[len(b.Nodes)-1])}}
	main := simulator.SkillRef{}
	for _, c := range []struct {
		name string
		opt  simulator.Option
		want error
		text string
	}{
		{"unknown key", simulator.RankBySkill(main, "noSuchOutput"), simulator.ErrUnknownOutput, "noSuchOutput"},
		{"not a skill output", simulator.RankBySkill(main, poe1.Life), simulator.ErrNotSkillOutput, string(poe1.Life)},
		{"no skill at ref", simulator.RankBySkill(simulator.SkillRef{Group: 99}, poe1.AverageHit), simulator.ErrNoSkill, "group 99"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.Sweep(b, variants, c.opt)
			if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.text) {
				t.Errorf("err %v, want %v naming %q", err, c.want, c.text)
			}
		})
	}
}

// A skill the dataset does not resolve, one without levels, warns with EverySkill, and the run
// goes on without its outputs.
func TestSkillUnresolved(t *testing.T) {
	t.Parallel()
	d := testDataset("3.29.3.3", 20)
	d.Skills = &dataset.Skills{
		Gems: []dataset.Gem{{
			ID: "Metadata/Items/Gems/SkillGemNoLevels", Colour: dataset.Blue,
			Effects: []dataset.GemEffect{{ID: "NoLevels", GrantedEffect: "NoLevels", Style: dataset.DefaultStyle}},
		}},
		GrantedEffects: []dataset.GrantedEffect{{ID: "NoLevels", ActiveSkill: "no_levels"}},
		ActiveSkills:   []dataset.ActiveSkill{{ID: "no_levels"}},
	}
	s, err := simulator.New(storeOf(t, d))
	if err != nil {
		t.Fatal(err)
	}
	b := marauderBuild(90)
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemNoLevels", Level: 1}}})
	st := simulate(t, s, b, simulator.EverySkill())
	if !slices.Contains(codes(st), simulator.SkillUnresolved) || len(st.Skills()) != 0 {
		t.Errorf("warnings %v, skills %v; want %s and no skills", st.Warnings(), st.Skills(), simulator.SkillUnresolved)
	}
	if slices.Contains(codes(simulate(t, s, b)), simulator.SkillUnresolved) {
		t.Errorf("warns %s without EverySkill", simulator.SkillUnresolved)
	}
}

// A ref is a position: after removing a group before it, another skill is there, so Compare and
// Sweep give no change for it, while the skill before the removed group keeps its change.
func TestSkillMovedByEdit(t *testing.T) {
	t.Parallel()
	const gems = "Metadata/Items/Gems/"
	s := realSim(t)
	b := marauderBuild(90)
	for _, id := range []string{"SkillGemFireball", "SkillGemFreezingPulse", "SkillGemArc"} {
		b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: gems + id, Level: 20}}})
	}
	edit := []build.Edit{build.RemoveSkillGroup(1)}
	removed := b.Clone()
	if err := removed.Apply(edit...); err != nil {
		t.Fatal(err)
	}
	fireball, moved := simulator.SkillRef{}, simulator.SkillRef{Group: 1}
	d, err := s.Compare(b, removed, simulator.EverySkill())
	if err != nil {
		t.Fatal(err)
	}
	if c := d.GetSkill(moved, poe1.AverageHit); c.Has {
		t.Errorf("group 2, Freezing Pulse before and Arc after: %+v, want no change", c)
	}
	if c := d.GetSkill(fireball, poe1.AverageHit); !c.Has {
		t.Error("Fireball has no change")
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		SkillChanges []simulator.SkillRef `json:"skillChanges"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.SkillChanges, []simulator.SkillRef{fireball}) {
		t.Errorf("wire skill changes %v, want Fireball's alone", w.SkillChanges)
	}
	results, err := s.Sweep(b, [][]build.Edit{edit}, simulator.EverySkill())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := results[0].SkillChanges[moved]; ok || results[0].SkillChanges[fireball] == nil {
		t.Errorf("sweep skill changes %v, want Fireball's alone", results[0].SkillChanges)
	}
}

// A build without skills attacks with the default attack, and without a weapon with the class's
// unarmed attacks: a Marauder's 2 to 8 physical damage at 833 ms per attack (Characters), 1.2
// per second. Strength's 6% (32 Strength) rounds back to 2 to 8; the default enemy's armour
// takes the most it can, 75%. A bow skill cannot be used unarmed, so it has no hit.
func TestUnarmedAttack(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	st := simulate(t, s, marauderBuild(1))
	for k, want := range map[output.Key]float64{
		poe1.PhysicalMinimum: 0.5, poe1.PhysicalMaximum: 2, poe1.HitsPerSecond: 1.2, poe1.CriticalStrikeChance: 0,
	} {
		if got := get(t, st, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	b := marauderBuild(1)
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemSplitArrow", Level: 1}}})
	if _, ok := simulate(t, s, b).Get(poe1.AverageHit); ok {
		t.Error("a bow skill unarmed has an average hit")
	}
}
