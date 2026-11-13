package dataset_test

import (
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

const largeCluster = "Metadata/Items/Jewels/JewelPassiveTreeExpansionLarge"

// withJewels returns withItems with the standard tree's jewel radii and cluster data: a large
// socket (2) holding a medium one (3), each with its proxy (4, 5), and the passives a jewel adds
// (14084, 44810, 50000).
func withJewels() *dataset.Dataset {
	d := withItems()
	d.Items.Classes = append(d.Items.Classes, dataset.ItemClass{ID: "Jewel"})
	d.Items.Bases = append(d.Items.Bases, dataset.ItemBase{ID: largeCluster, Class: "Jewel", ModDomain: 21})
	t := &d.Trees[0]
	t.Nodes = append(t.Nodes,
		dataset.Node{ID: 3, Kind: dataset.JewelSocket},
		dataset.Node{ID: 4, Kind: dataset.Proxy},
		dataset.Node{ID: 5, Kind: dataset.Proxy},
		dataset.Node{ID: 14084, Stats: []dataset.StatValue{{Stat: 1, Value: 12}}},
		dataset.Node{ID: 44810, Kind: dataset.Mastery},
		dataset.Node{ID: 50000, Kind: dataset.Notable, Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 2, Value: 25}}},
	)
	t.JewelSlots = []uint32{2, 3}
	t.JewelRadii = []dataset.JewelRadius{
		{ID: "Small", Radius: 800, RingInner: 800, RingOuter: 1100},
		{ID: "Large", Radius: 1500, RingInner: 1400, RingOuter: 1700},
	}
	t.Clusters = dataset.Clusters{
		Sockets: []dataset.ClusterSocket{
			{Node: 2, Size: dataset.LargeCluster, Proxy: 4, StartIndices: []int32{3, 5, 5}},
			{Node: 3, Size: dataset.MediumCluster, Index: 1, Proxy: 5, Parent: 2, StartIndices: []int32{1, 1}},
		},
		Jewels: []dataset.ClusterJewel{{
			Size: dataset.LargeCluster, Base: largeCluster, MinNodes: 8, MaxNodes: 12,
			SmallIndices: []int32{0, 4, 6}, NotableIndices: []int32{6, 4}, SocketIndices: []int32{4, 8, 6}, TotalIndices: 12,
		}},
		Skills:   []dataset.ClusterSkill{{Size: dataset.LargeCluster, Passive: 14084, Mastery: 44810, Tag: "affliction_axe_and_sword_damage"}},
		Notables: []dataset.ClusterNotable{{Passive: 50000, Stat: 2}},
	}
	return d
}

func TestJewelsRoundTrip(t *testing.T) {
	t.Parallel()
	d := withJewels()
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

func TestJewelsWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(t *dataset.Tree, d *dataset.Dataset)
	}{
		{name: "ring reversed", want: `jewel radius "Large": empty id, radius not positive or ring reversed`, change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.JewelRadii[1].RingInner = 1800
		}},
		{name: "socket not a jewel slot", want: "cluster socket 3: not a jewel slot", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.JewelSlots = []uint32{2}
		}},
		{name: "proxy not in tree", want: "cluster socket 2: proxy 9 not a proxy node", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Sockets[0].Proxy = 9
		}},
		{name: "proxy not a proxy", want: "cluster socket 2: proxy 3 not a proxy node", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Sockets[0].Proxy = 3
		}},
		{name: "parent not a socket", want: "cluster socket 3: parent 1 not another cluster socket", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Sockets[1].Parent = 1
		}},
		{name: "sockets unsorted", want: "cluster sockets not sorted or repeated at 2", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			s := t.Clusters.Sockets
			s[0], s[1] = s[1], s[0]
		}},
		{name: "jewel index outside total", want: "index 12 outside 12 indices", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Jewels[0].SocketIndices[0] = 12
		}},
		{name: "jewel base not in items", want: "cluster jewel " + largeCluster + ": base not in items", change: func(_ *dataset.Tree, d *dataset.Dataset) {
			d.Items.Bases = d.Items.Bases[:len(d.Items.Bases)-1]
		}},
		{name: "jewel without items", want: "cluster jewel without items", change: func(_ *dataset.Tree, d *dataset.Dataset) {
			d.Items, d.Mods = nil, nil
		}},
		{name: "skill of unknown passive", want: "cluster skill 0: passive 9 or mastery 44810 not in tree", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Skills[0].Passive = 9
		}},
		{name: "notable of unknown passive", want: "cluster notable 0: passive 7 not in tree", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Notables[0].Passive = 7
		}},
		{name: "notable stat out of range", want: "cluster notable 0: stat 9 out of range", change: func(t *dataset.Tree, _ *dataset.Dataset) {
			t.Clusters.Notables[0].Stat = 9
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withJewels()
			testCase.change(&d.Trees[0], d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

func TestTreeClusterSocket(t *testing.T) {
	t.Parallel()
	tree := withJewels().Trees[0]
	if s, ok := tree.ClusterSocket(3); !ok || s.Parent != 2 {
		t.Errorf("ClusterSocket(3) = %v, %t; want the medium socket on 2", s, ok)
	}
	if _, ok := tree.ClusterSocket(1); ok {
		t.Error("ClusterSocket(1): found, want none")
	}
}
