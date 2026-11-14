package build_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
)

// largeSocket is a large cluster socket of the 3.29 tree, Index 3 in the game's data; added is
// the id base of what a large jewel adds in it: bit 16, size 2 in bits 4-5, index 3 in bits 6-8.
// mediumSocket is the medium socket at Index 0 on it, and regularSocket a socket that takes no
// cluster jewel.
const (
	largeSocket   = 2491
	added         = 1<<16 | 2<<4 | 3<<6
	mediumSocket  = 10532
	regularSocket = 26725
)

const largeJewel = `Rarity: RARE
Doom Spiral
Large Cluster Jewel
Item Level: 84
Implicits: 3
{enchant}Adds 8 Passive Skills
{enchant}2 Added Passive Skills are Jewel Sockets
{enchant}Added Small Passive Skills grant: 12% increased Fire Damage
Added Small Passive Skills also grant: +10 to Maximum Life
1 Added Passive Skill is Master of Fire
1 Added Passive Skill is Fire Attunement
`

const mediumJewel = `Rarity: RARE
Doom Spiral
Medium Cluster Jewel
Item Level: 84
Implicits: 2
{enchant}Adds 5 Passive Skills
{enchant}Added Small Passive Skills grant: 12% increased Burning Damage
`

func readJewel(t *testing.T, r *items.Reader, text string) build.Item {
	t.Helper()
	it, _, err := r.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return it
}

// clusterBuild is a level 90 Marauder with the path to the large socket allocated and the large
// jewel in it.
func clusterBuild(t *testing.T, s *dataset.Store, jewel build.Item) *build.Build {
	t.Helper()
	b := &build.Build{Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard", Class: marauder, Level: 90}
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	toSocket, ok := c.AllocatePath(largeSocket)
	if !ok {
		t.Fatal("no path to the socket")
	}
	if err := b.Apply(toSocket, build.SocketJewel(largeSocket, jewel)); err != nil {
		t.Fatal(err)
	}
	return b
}

// check checks b and returns it checked and its warnings, only those of codes if any.
func check(t *testing.T, b *build.Build, s *dataset.Store, codes ...build.Code) (*build.Checked, []build.Warning) {
	t.Helper()
	c, ws, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) == 0 {
		return c, ws
	}
	return c, slices.DeleteFunc(ws, func(w build.Warning) bool { return !slices.Contains(codes, w.Code) })
}

// TestCheckClusterNodes: a large cluster jewel in an allocated socket and 4 of the passives it
// adds check clean, the path to them runs from the socket through the jewel's first passive and
// a socket it adds, and their stats count; without the jewel each of them is dropped with a
// warning; a medium jewel in a socket from the large one gives passives too; a large jewel in
// that socket, or in a socket that takes no cluster jewel, is left out.
func TestCheckClusterNodes(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	large := readJewel(t, r, largeJewel)
	b := clusterBuild(t, s, large)
	c, ws := check(t, b, s)
	if len(ws) > 0 {
		t.Fatalf("warnings with the jewel and no passives of it: %v", ws)
	}
	// Orbit index 5 is two passives and the socket at index 4 from the entrance at index 0.
	path, ok := c.AllocatePath(added + 5)
	want := []uint32{added, added + 2, mediumSocket, added + 5}
	if !ok || !slices.Equal(path.Nodes, want) {
		t.Fatalf("AllocatePath(%d) = %v, %t; want %v", added+5, path.Nodes, ok, want)
	}
	if err := b.Apply(path); err != nil {
		t.Fatal(err)
	}
	c, ws = check(t, b, s)
	if len(ws) > 0 {
		t.Errorf("warnings with 4 passives of the jewel: %v", ws)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(c.Nodes(), func(n *dataset.Node) bool { return n.ID == added })
	if i < 0 {
		t.Fatalf("passive %d not allocated", added)
	}
	got := map[string]int32{}
	for _, sv := range c.Nodes()[i].Stats {
		got[stats[sv.Stat].ID] = sv.Value
	}
	if wantStats := map[string]int32{"fire_damage_+%": 12, "base_maximum_life": 10}; !maps.Equal(got, wantStats) {
		t.Errorf("stats of passive %d: %v, want %v", added, got, wantStats)
	}

	t.Run("without the jewel", func(t *testing.T) {
		t.Parallel()
		without := b.Clone()
		if err := without.Apply(build.UnsocketJewel(largeSocket)); err != nil {
			t.Fatal(err)
		}
		_, ws := check(t, without, s, build.NodeNoClusterJewel)
		got := make([]string, 0, len(ws))
		for _, w := range ws {
			got = append(got, w.Subject)
		}
		want := []string{"node 10532", "node 65760", "node 65762", "node 65765"}
		if !slices.Equal(got, want) {
			t.Errorf("node-no-cluster-jewel warnings on %v, want %v", got, want)
		}
	})
	t.Run("medium jewel in the added socket", func(t *testing.T) {
		t.Parallel()
		nested := b.Clone()
		if err := nested.Apply(build.SocketJewel(mediumSocket, readJewel(t, r, mediumJewel))); err != nil {
			t.Fatal(err)
		}
		c, ws := check(t, nested, s)
		if len(ws) > 0 {
			t.Errorf("warnings: %v", ws)
		}
		// A medium jewel in medium socket Index 0 of large socket Index 3.
		const entrance = 1<<16 | 1<<4 | 3<<6
		if path, ok := c.AllocatePath(entrance); !ok || !slices.Equal(path.Nodes, []uint32{entrance}) {
			t.Errorf("AllocatePath(%d) = %v, %t; want the entrance alone", entrance, path.Nodes, ok)
		}
	})
	t.Run("jewels that do not fit", func(t *testing.T) {
		t.Parallel()
		misfit := b.Clone()
		if err := misfit.Apply(build.SocketJewel(mediumSocket, large), build.SocketJewel(regularSocket, large)); err != nil {
			t.Fatal(err)
		}
		_, ws := check(t, misfit, s, build.JewelTooLarge, build.JewelKindMismatch)
		want := []build.Warning{
			{Code: build.JewelTooLarge, Subject: "jewel 10532", Message: "a large cluster jewel does not fit a medium socket; left out"},
			{Code: build.JewelKindMismatch, Subject: "jewel 26725", Message: "a cluster jewel does not fit a socket that takes none; left out"},
		}
		if diff := cmp.Diff(want, ws); diff != "" {
			t.Errorf("warnings (-want +got):\n%s", diff)
		}
	})
}
