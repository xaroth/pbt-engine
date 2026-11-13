package items_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/items"
)

func gameSample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "game", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestParseGameSamples reads items copied from the game with Ctrl+Alt+C: those no slot takes and
// flasks are refused, every other item reads without a warning but those listed.
func TestParseGameSamples(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	refused := map[string]error{
		"15-map-tier-16.txt": items.ErrNotEquipable,
		"16-endless-eternal-life-flask-of-alleviation.txt":  items.ErrNotSupported,
		"17-investigator-s-quartz-flask-of-the-gazelle.txt": items.ErrNotSupported,
		"18-scroll-of-wisdom.txt":                           items.ErrNotEquipable,
		"19-portal-scroll.txt":                              items.ErrNotEquipable,
	}
	// An anoint names a passive node, which no stat description reads.
	unread := map[string]string{"05-rotmother-s-mutiny.txt": "Allocates Light of Divinity"}
	names, err := filepath.Glob(filepath.Join("testdata", "game", "*.txt"))
	if err != nil || len(names) != 23 {
		t.Fatalf("%d samples, %v", len(names), err)
	}
	for _, path := range names {
		name := filepath.Base(path)
		it, warnings, err := r.Parse(gameSample(t, name))
		if want := refused[name]; want != nil || err != nil {
			if !errors.Is(err, want) {
				t.Errorf("%s: got %v, want %v", name, err, want)
			}
			continue
		}
		got := make([]string, 0, len(warnings))
		for _, w := range warnings {
			got = append(got, string(w.Code)+" "+w.Subject)
		}
		want := []string{}
		if line, ok := unread[name]; ok {
			want = []string{string(items.LineUnread) + " " + line}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s warnings (-want +got):\n%s", name, diff)
		}
		if it.Name == "" || len(it.Mods) == 0 {
			t.Errorf("%s: read without a name or mods: %+v", name, it)
		}
	}
}

// Armageddon Circle as Path of Building writes it.
const armageddonPoB = `Rarity: RARE
Armageddon Circle
Vermillion Ring
Item Level: 85
Implicits: 1
5% increased maximum Life
+10 to Evasion Rating
+25 to maximum Energy Shield
{crafted}+53 to maximum Life
+49 to Dexterity
+46% to Cold Resistance
`

// Armageddon Circle as the game copies it with Ctrl+C, without mod headers.
const armageddonPlain = `Item Class: Rings
Rarity: Rare
Armageddon Circle
Vermillion Ring
--------
Intangibility: 49%
--------
Requirements:
Level: 80
--------
Item Level: 85
--------
5% increased maximum Life (implicit)
--------
+10 to Evasion Rating
+25 to maximum Energy Shield
+53 to maximum Life (crafted)
+49 to Dexterity
+46% to Cold Resistance
`

// Headhunter as Path of Building writes it.
const headhunterPoB = `Rarity: UNIQUE
Headhunter
Leather Belt
Item Level: 85
Implicits: 1
+37 to maximum Life
28% increased Damage with Hits against Rare monsters
When you Kill a Rare monster, you gain its Modifiers for 60 seconds
+57 to maximum Life
+47 to Strength
+55 to Dexterity
Corrupted
`

// TestParseGameAsPoB reads a rare and a unique in the game's formats and in Path of Building's
// to the same item.
func TestParseGameAsPoB(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	for _, tc := range []struct{ name, game, pob string }{
		{"rare, Ctrl+Alt+C", gameSample(t, "02-armageddon-circle.txt"), armageddonPoB},
		{"rare, Ctrl+C", armageddonPlain, armageddonPoB},
		{"unique, Ctrl+Alt+C", gameSample(t, "07-headhunter.txt"), headhunterPoB},
	} {
		want, _, err := r.Parse(tc.pob)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := r.Parse(tc.game)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s (-pob +game):\n%s", tc.name, diff)
		}
	}
}

// TestParseGameUnidentified reads an unidentified rare, which shows its base only.
func TestParseGameUnidentified(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	text := strings.Join([]string{
		"Item Class: Rings", "Rarity: Rare", "Vermillion Ring", "--------", "Item Level: 85", "--------",
		"5% increased maximum Life (implicit)", "--------", "Unidentified",
	}, "\n")
	it, _, err := r.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	want := build.Item{
		Rarity: build.Rare, Base: "Metadata/Items/Rings/RingAtlas3", ItemLevel: 85, Flags: []string{"Unidentified"},
		Mods: []build.ModLine{{Kind: build.Implicit, Text: "5% increased maximum Life", Stats: []build.StatValue{{Stat: "maximum_life_+%", Value: 5}}}},
	}
	if diff := cmp.Diff(want, it); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}
