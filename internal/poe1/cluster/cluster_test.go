package cluster_test

import (
	"errors"
	"math"
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/cluster"
	"github.com/xaroth/pbt-engine/statdesc"
)

const (
	itemScope = "Metadata/StatDescriptions/stat_descriptions.txt"
	treeScope = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"

	smallBase  = "Metadata/Items/Jewels/JewelPassiveTreeExpansionSmall"
	mediumBase = "Metadata/Items/Jewels/JewelPassiveTreeExpansionMedium"
	largeBase  = "Metadata/Items/Jewels/JewelPassiveTreeExpansionLarge"
)

// Nodes of the test tree. A large socket (Index 2) holds three medium sockets; the middle one
// (Index 1) holds a small socket. Proxies, sockets and their angles follow the game's 3.29 tree
// around one large socket.
const (
	start       = 1
	large       = 10
	largeProxy  = 11
	medium0     = 12
	medium1     = 13
	medium2     = 14
	medium1Prox = 15
	small       = 16
	smallProxy  = 17
	medium0Prox = 18
	medium2Prox = 19
	// Small passive kinds and notables.
	coldLarge  = 20
	fireLarge  = 21
	coldMedium = 22
	alpha      = 30
	beta       = 31
	gamma      = 32
)

// Stat handles of the test dataset, in statIDs order.
const (
	hNodes = iota
	hSockets
	hSocketsAdded
	hEmpty
	hNothing
	hEffect
	hAlsoLife
	hBeta
	hAlpha
	hGamma
	hLife
	hCold
	hFire
)

func statIDs() []string {
	return []string{
		"local_jewel_expansion_passive_node_count", "local_jewel_expansion_jewels_count",
		"local_jewel_expansion_jewels_count_override", "local_unique_jewel_grants_x_empty_passives",
		"local_affliction_jewel_display_small_nodes_grant_nothing", "local_affliction_jewel_small_nodes_have_effect_+%",
		"local_affliction_jewel_small_nodes_grant_maximum_life",
		"local_affliction_notable_beta", "local_affliction_notable_alpha", "local_affliction_notable_gamma",
		"base_maximum_life", "cold_damage_+%", "fire_damage_+%",
	}
}

func any32() []statdesc.Limit { return []statdesc.Limit{{Min: math.MinInt32, Max: math.MaxInt32}} }

func desc(stat, text string) statdesc.Description {
	return statdesc.Description{Stats: []string{stat}, Variants: []statdesc.Variant{{Limits: any32(), Text: text}}}
}

// descriptions describe the beta notable before alpha, so beta comes first on the ring.
func descriptions() []statdesc.Scope {
	return []statdesc.Scope{
		{Name: itemScope, Descriptions: []statdesc.Description{
			desc("local_jewel_expansion_passive_node_count", "Adds {0} Passive Skills"),
			{Stats: []string{"local_jewel_expansion_jewels_count"}, Variants: []statdesc.Variant{
				{Limits: []statdesc.Limit{{Min: 1, Max: 1}}, Text: "1 Added Passive Skill is a Jewel Socket"},
				{Limits: []statdesc.Limit{{Min: 2, Max: math.MaxInt32}}, Text: "{0} Added Passive Skills are Jewel Sockets"},
			}},
			desc("local_jewel_expansion_jewels_count_override", "Adds {0} Jewel Socket Passive Skills"),
			desc("local_unique_jewel_grants_x_empty_passives", "Adds {0} Small Passive Skills which grant nothing"),
			desc("local_affliction_jewel_display_small_nodes_grant_nothing", "Added Small Passive Skills grant Nothing"),
			desc("local_affliction_jewel_small_nodes_have_effect_+%", "Added Small Passive Skills have {0}% increased Effect"),
			desc("local_affliction_jewel_small_nodes_grant_maximum_life", "Added Small Passive Skills also grant: {0:+d} to Maximum Life"),
			desc("local_affliction_notable_beta", "1 Added Passive Skill is Beta"),
			desc("local_affliction_notable_alpha", "1 Added Passive Skill is Alpha"),
			desc("local_affliction_notable_gamma", "1 Added Passive Skill is Gamma"),
			desc("base_maximum_life", "{0:+d} to maximum Life"),
		}},
		{Name: treeScope, Parent: itemScope, Descriptions: []statdesc.Description{
			desc("cold_damage_+%", "{0}% increased Cold Damage"),
			desc("fire_damage_+%", "{0}% increased Fire Damage"),
		}},
	}
}

func sv(stat uint32, v int32) []dataset.StatValue { return []dataset.StatValue{{Stat: stat, Value: v}} }

func testTree() dataset.Tree {
	return dataset.Tree{
		Variant: "standard",
		Classes: []dataset.Class{{Character: "Metadata/Characters/Str/Str", StartNode: start}},
		Nodes: []dataset.Node{
			{ID: start, Kind: dataset.ClassStart, Neighbours: []uint32{large}},
			{ID: large, Kind: dataset.JewelSocket, Neighbours: []uint32{start}},
			{ID: largeProxy, Kind: dataset.Proxy},
			{ID: medium0, Kind: dataset.JewelSocket},
			{ID: medium1, Kind: dataset.JewelSocket},
			{ID: medium2, Kind: dataset.JewelSocket},
			{ID: medium1Prox, Kind: dataset.Proxy},
			{ID: small, Kind: dataset.JewelSocket},
			{ID: smallProxy, Kind: dataset.Proxy},
			{ID: medium0Prox, Kind: dataset.Proxy},
			{ID: medium2Prox, Kind: dataset.Proxy},
			{ID: coldLarge, Stats: sv(hCold, 12)},
			{ID: fireLarge, Stats: sv(hFire, 12)},
			{ID: coldMedium, Stats: sv(hCold, 10)},
			{ID: alpha, Kind: dataset.Notable, Stats: sv(hLife, 40)},
			{ID: beta, Kind: dataset.Notable, Stats: sv(hFire, 30)},
			{ID: gamma, Kind: dataset.Notable, Stats: sv(hCold, 30)},
		},
		JewelSlots: []uint32{large, medium0, medium1, medium2, small},
		Clusters: dataset.Clusters{
			Sockets: []dataset.ClusterSocket{
				{Node: large, Size: dataset.LargeCluster, Index: 2, Proxy: largeProxy, StartIndices: []int32{4, 7, 7}},
				{Node: medium0, Size: dataset.MediumCluster, Index: 0, Proxy: medium0Prox, Parent: large, StartIndices: []int32{2, 5}},
				{Node: medium1, Size: dataset.MediumCluster, Index: 1, Proxy: medium1Prox, Parent: large, StartIndices: []int32{4, 7}},
				{Node: medium2, Size: dataset.MediumCluster, Index: 2, Proxy: medium2Prox, Parent: large, StartIndices: []int32{3, 5}},
				{Node: small, Size: dataset.SmallCluster, Index: 0, Proxy: smallProxy, Parent: medium1, StartIndices: []int32{3}},
			},
			Jewels: []dataset.ClusterJewel{
				{
					Size: dataset.SmallCluster, Base: smallBase, MinNodes: 2, MaxNodes: 3,
					SmallIndices: []int32{0, 4, 2}, NotableIndices: []int32{4}, SocketIndices: []int32{4}, TotalIndices: 6,
				},
				{
					Size: dataset.MediumCluster, Base: mediumBase, MinNodes: 4, MaxNodes: 6,
					SmallIndices: []int32{0, 6, 8, 4, 10, 2}, NotableIndices: []int32{6, 10, 2, 0}, SocketIndices: []int32{6}, TotalIndices: 12,
				},
				{
					Size: dataset.LargeCluster, Base: largeBase, MinNodes: 8, MaxNodes: 12,
					SmallIndices: []int32{0, 4, 6, 8, 10, 2, 7, 5, 9, 3, 11, 1}, NotableIndices: []int32{6, 4, 8, 10, 2},
					SocketIndices: []int32{4, 8, 6}, TotalIndices: 12,
				},
			},
			Skills: []dataset.ClusterSkill{
				{Size: dataset.LargeCluster, Passive: fireLarge, Tag: "affliction_fire_damage"},
				{Size: dataset.LargeCluster, Passive: coldLarge, Tag: "affliction_cold_damage"},
				{Size: dataset.MediumCluster, Passive: coldMedium, Tag: "affliction_cold_damage"},
			},
			Notables: []dataset.ClusterNotable{{Passive: alpha, Stat: hAlpha}, {Passive: beta, Stat: hBeta}, {Passive: gamma, Stat: hGamma}},
		},
	}
}

// testLayout places the sockets and proxies as the 3.29 tree places them around a large socket;
// the passives a jewel adds are not drawn.
func testLayout() dataset.Layout {
	const icon = "Art/2DArt/SkillIcons/passives/Blood2.png"
	n := func(id, group uint32, orbit uint8, index uint16) dataset.LayoutNode {
		return dataset.LayoutNode{ID: id, Group: group, Orbit: orbit, OrbitIndex: index, Icon: icon}
	}
	return dataset.Layout{
		Variant: "standard",
		Orbits:  []dataset.Orbit{{Radius: 0, Slots: 1}, {Radius: 82, Slots: 6}, {Radius: 162, Slots: 16}, {Radius: 335, Slots: 16}},
		Groups: []dataset.LayoutGroup{
			{ID: 1, X: -2000, Y: 0},
			{ID: 2, X: 0, Y: 0, Proxy: true},
			{ID: 3, X: 1000, Y: 0, Proxy: true},
			{ID: 4, X: 2000, Y: 0, Proxy: true},
			{ID: 5, X: 0, Y: 1000, Proxy: true},
			{ID: 6, X: 0, Y: -1000, Proxy: true},
		},
		Nodes: []dataset.LayoutNode{
			n(start, 1, 0, 0), n(large, 1, 1, 2),
			n(largeProxy, 2, 3, 9), n(medium0, 2, 3, 15), n(medium1, 2, 3, 1), n(medium2, 2, 3, 4),
			n(medium1Prox, 3, 2, 9), n(small, 3, 2, 1), n(smallProxy, 4, 1, 3),
			n(medium0Prox, 5, 2, 7), n(medium2Prox, 6, 2, 12),
			{ID: coldLarge, Icon: icon}, {ID: fireLarge, Icon: icon}, {ID: coldMedium, Icon: icon},
			{ID: alpha, Icon: icon}, {ID: beta, Icon: icon}, {ID: gamma, Icon: icon},
		},
	}
}

func testGenerator(t *testing.T) *cluster.Generator {
	t.Helper()
	d := &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Trees:      []dataset.Tree{testTree()},
		Layouts:    []dataset.Layout{testLayout()},
		Texts:      []dataset.Text{{Language: "en", Descriptions: descriptions()}},
		Characters: []dataset.Character{{ID: "Metadata/Characters/Str/Str"}},
		Items: &dataset.Items{
			Classes: []dataset.ItemClass{{ID: "Jewel"}},
			Bases: []dataset.ItemBase{
				{ID: largeBase, Class: "Jewel"},
				{ID: mediumBase, Class: "Jewel"},
				{ID: smallBase, Class: "Jewel"},
			},
		},
	}
	for _, id := range statIDs() {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	g, err := cluster.NewGenerator(s, "standard")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}
	return g
}

// node is a generated node; positions are checked by TestPositions.
func node(id uint32, index int32, kind dataset.NodeKind, passive uint32, stats []dataset.StatValue, neighbours ...uint32) cluster.Node {
	return cluster.Node{ID: id, Kind: kind, Stats: stats, Neighbours: neighbours, Passive: passive, OrbitIndex: index}
}

func socketNode(id uint32, index int32, neighbours ...uint32) cluster.Node {
	return node(id, index, dataset.JewelSocket, id, nil, neighbours...)
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	g := testGenerator(t)
	tree := testTree()
	skill := func(i int) *dataset.ClusterSkill { return &tree.Clusters.Skills[i] }
	// Large jewel in large socket Index 2: 0x10000 | size 2 << 4 | 2 << 6.
	const l = 65696
	// Its small passives: 12% cold and 5 life with 25% increased effect, truncated.
	largeSmall := []dataset.StatValue{{Stat: hLife, Value: 6}, {Stat: hCold, Value: 15}}
	// Medium jewel in medium socket Index 1 of large socket Index 2.
	const m = 65536 | 1<<4 | 2<<6 | 1<<9
	// Medium jewel in the large socket: the large socket's index, a medium's size.
	const ml = 65536 | 1<<4 | 2<<6
	testCases := []struct {
		name   string
		socket uint32
		jewel  cluster.Jewel
		want   *cluster.Subgraph
	}{
		{
			// Sockets at 4 and 8, notables on the first free of 6, 4, 8, 10, small passives on the
			// first free of 0, 4, 6, 8, 10, 2, 7, 5. Beta is described first, so it takes 6.
			name: "large with 8 passives and 2 notables", socket: large,
			jewel: cluster.Jewel{Base: largeBase, Lines: []string{
				"Adds 8 Passive Skills", "2 Added Passive Skills are Jewel Sockets",
				"Added Small Passive Skills grant: 12% increased Cold Damage",
				"Added Small Passive Skills also grant: +5 to Maximum Life",
				"Added Small Passive Skills also grant: +5 to Strength",
				"Added Small Passive Skills have 25% increased Effect",
				"1 Added Passive Skill is Alpha", "1 Added Passive Skill is Beta",
			}},
			want: &cluster.Subgraph{
				Socket: large, Size: dataset.LargeCluster, Skill: skill(1), Entrance: l,
				Nodes: []cluster.Node{
					socketNode(medium0, 4, l+2, l+5),
					socketNode(medium2, 8, l+7, l+10),
					node(l, 0, dataset.Normal, coldLarge, largeSmall, large, l+2, l+10),
					node(l+2, 2, dataset.Normal, coldLarge, largeSmall, medium0, l),
					node(l+5, 5, dataset.Normal, coldLarge, largeSmall, medium0, l+6),
					node(l+6, 6, dataset.Notable, beta, sv(hFire, 30), l+5, l+7),
					node(l+7, 7, dataset.Normal, coldLarge, largeSmall, medium2, l+6),
					node(l+10, 10, dataset.Notable, alpha, sv(hLife, 40), medium2, l),
				},
				Unread: []string{"Added Small Passive Skills also grant: +5 to Strength"},
			},
		},
		{
			// The socket at 6 is the small socket drawn there; notables on 10 and 2, small
			// passives on 0 and 8.
			name: "medium in a large's medium socket", socket: medium1,
			jewel: cluster.Jewel{Base: mediumBase, Lines: []string{
				"Adds 5 Passive Skills", "1 Added Passive Skill is a Jewel Socket",
				"Added Small Passive Skills grant: 10% increased Cold Damage",
				"1 Added Passive Skill is Gamma", "1 Added Passive Skill is Alpha",
			}},
			want: &cluster.Subgraph{
				Socket: medium1, Size: dataset.MediumCluster, Skill: skill(2), Entrance: m,
				Nodes: []cluster.Node{
					socketNode(small, 6, m+2, m+8),
					node(m, 0, dataset.Normal, coldMedium, sv(hCold, 10), medium1, m+2, m+10),
					node(m+2, 2, dataset.Notable, alpha, sv(hLife, 40), small, m),
					node(m+8, 8, dataset.Normal, coldMedium, sv(hCold, 10), small, m+10),
					node(m+10, 10, dataset.Notable, gamma, sv(hCold, 30), m, m+8),
				},
			},
		},
		{
			name: "Voices", socket: large,
			jewel: cluster.Jewel{Base: largeBase, Lines: []string{
				"Adds 3 Jewel Socket Passive Skills", "Adds 5 Small Passive Skills which grant nothing",
			}},
			want: &cluster.Subgraph{
				Socket: large, Size: dataset.LargeCluster, Entrance: l,
				Nodes: []cluster.Node{
					socketNode(medium0, 4, l+2, l+5),
					socketNode(medium1, 6, l+5, l+7),
					socketNode(medium2, 8, l+7, l+10),
					node(l, 0, dataset.Normal, 0, nil, large, l+2, l+10),
					node(l+2, 2, dataset.Normal, 0, nil, medium0, l),
					node(l+5, 5, dataset.Normal, 0, nil, medium0, medium1),
					node(l+7, 7, dataset.Normal, 0, nil, medium1, medium2),
					node(l+10, 10, dataset.Normal, 0, nil, medium2, l),
				},
			},
		},
		{
			// Four passives in a medium jewel move the notables from 10 and 2 to 9 and 3.
			name: "Megalomaniac in a large socket", socket: large,
			jewel: cluster.Jewel{Base: mediumBase, Lines: []string{
				"Adds 4 Passive Skills", "Added Small Passive Skills grant Nothing",
				"1 Added Passive Skill is Gamma", "1 Added Passive Skill is Alpha", "1 Added Passive Skill is Beta",
			}},
			want: &cluster.Subgraph{
				Socket: large, Size: dataset.MediumCluster, Entrance: ml,
				Nodes: []cluster.Node{
					node(ml, 0, dataset.Normal, 0, nil, large, ml+3, ml+9),
					node(ml+3, 3, dataset.Notable, beta, sv(hFire, 30), ml, ml+6),
					node(ml+6, 6, dataset.Notable, alpha, sv(hLife, 40), ml+3, ml+9),
					node(ml+9, 9, dataset.Notable, gamma, sv(hCold, 30), ml, ml+6),
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := g.Generate(testCase.socket, testCase.jewel)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if diff := gocmp.Diff(testCase.want, got, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(cluster.Node{}, "X", "Y")); diff != "" {
				t.Errorf("Generate (-want +got):\n%s", diff)
			}
		})
	}
}

// TestPositions checks that a jewel's ring is turned to its socket: the large socket's start
// index 7 puts orbit index 0 at 210 degrees, and the sockets it adds where the tree draws them.
func TestPositions(t *testing.T) {
	t.Parallel()
	g := testGenerator(t)
	voices := cluster.Jewel{Base: largeBase, Lines: []string{"Adds 3 Jewel Socket Passive Skills", "Adds 5 Small Passive Skills which grant nothing"}}
	got, err := g.Generate(large, voices)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	l := testLayout()
	want := map[uint32][2]float64{65696: {-167.5, 335 * math.Sqrt(3) / 2}}
	for _, id := range []uint32{medium0, medium1, medium2} {
		x, y, _ := l.Position(id)
		want[id] = [2]float64{x, y}
	}
	for _, n := range got.Nodes {
		if w, ok := want[n.ID]; ok && math.Hypot(n.X-w[0], n.Y-w[1]) > 1e-6 {
			t.Errorf("node %d at (%v, %v), want (%v, %v)", n.ID, n.X, n.Y, w[0], w[1])
		}
	}
}

func TestGenerateErrors(t *testing.T) {
	t.Parallel()
	g := testGenerator(t)
	lines := []string{"Adds 8 Passive Skills", "Added Small Passive Skills grant: 12% increased Cold Damage"}
	testCases := []struct {
		name   string
		socket uint32
		jewel  cluster.Jewel
		want   error
	}{
		{name: "not a cluster socket", socket: start, jewel: cluster.Jewel{Base: largeBase, Lines: lines}, want: cluster.ErrNotSocket},
		{name: "not a cluster jewel", socket: large, jewel: cluster.Jewel{Base: "Metadata/Items/Jewels/JewelStr", Lines: lines}, want: cluster.ErrNotCluster},
		{name: "larger than its socket", socket: medium1, jewel: cluster.Jewel{Base: largeBase, Lines: lines}, want: cluster.ErrTooLarge},
		{name: "no passive count", socket: large, jewel: cluster.Jewel{Base: largeBase, Lines: lines[1:]}, want: cluster.ErrUnread},
		{
			name: "small passive kind of another size", socket: large,
			jewel: cluster.Jewel{Base: largeBase, Lines: []string{"Adds 8 Passive Skills", "Added Small Passive Skills grant: 10% increased Cold Damage"}},
			want:  cluster.ErrUnread,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := g.Generate(testCase.socket, testCase.jewel); !errors.Is(err, testCase.want) {
				t.Errorf("Generate: got %v, want %v", err, testCase.want)
			}
		})
	}
}
