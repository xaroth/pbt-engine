package dataset_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

// withLayout is sample with a second class and a layout for its standard tree: node 1 at the
// centre of group 3, node 2 on orbit 1 of group 7.
func withLayout() *dataset.Dataset {
	d := sample()
	d.Trees[0].Classes = append(d.Trees[0].Classes, dataset.Class{Character: "Metadata/Characters/Dex/Dex", StartNode: 1})
	d.Layouts = []dataset.Layout{{
		Variant: "standard",
		MinX:    -100, MinY: -50, MaxX: 200, MaxY: 150,
		Orbits: []dataset.Orbit{
			{Radius: 0, Slots: 1},
			{Radius: 82, Slots: 16},
		},
		Groups: []dataset.LayoutGroup{
			{ID: 3, X: -10.5, Y: 20.25, Orbits: []uint8{0}},
			{ID: 7, X: 100, Y: 50, Orbits: []uint8{0, 1}, Background: &dataset.GroupBackground{Image: "PSGroupBackground1", HalfImage: true}, Proxy: true},
		},
		Nodes: []dataset.LayoutNode{
			{ID: 1, Group: 3, Icon: "Art/2DArt/SkillIcons/passives/Start.png"},
			{
				ID: 2, Group: 7, Orbit: 1, OrbitIndex: 2, Icon: "Art/2DArt/SkillIcons/passives/Mastery.png",
				MasteryInactiveIcon: "Art/inactive.png", MasteryActiveIcon: "Art/active.png", MasteryActiveEffect: "Art/effect.png",
			},
		},
		ClassArt: []dataset.ClassArt{{Class: 0, Image: "Art/2DArt/BaseClassIllustrations/Str.png", X: -4560.69, Y: 292.425}},
	}}
	return d
}

func TestLayoutRoundTrip(t *testing.T) {
	t.Parallel()
	d := withLayout()
	dir := t.TempDir()
	m, err := dataset.Write(dir, d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := readAll(os.DirFS(dir))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := gocmp.Diff(d.Layouts, got.Layouts, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
	if got := m.Layouts(); len(got) != 1 || got[0] != "standard" {
		t.Errorf("Manifest.Layouts: got %v, want [standard]", got)
	}
}

func TestLayoutPosition(t *testing.T) {
	t.Parallel()
	l := &withLayout().Layouts[0]
	l.Nodes = append(l.Nodes, dataset.LayoutNode{ID: 9, Icon: "Art/x.png"})
	testCases := []struct {
		name   string
		id     uint32
		x, y   float64
		wantOK bool
	}{
		{name: "group centre", id: 1, x: -10.5, y: 20.25, wantOK: true},
		// 45 degrees clockwise from up: right and up by 82/sqrt(2).
		{name: "on an orbit", id: 2, x: 100 + 82/math.Sqrt2, y: 50 - 82/math.Sqrt2, wantOK: true},
		{name: "not drawn", id: 9},
		{name: "not in layout", id: 5},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			x, y, ok := l.Position(testCase.id)
			if ok != testCase.wantOK || math.Abs(x-testCase.x) > 1e-9 || math.Abs(y-testCase.y) > 1e-9 {
				t.Errorf("Position(%d): got (%v, %v, %v), want (%v, %v, %v)", testCase.id, x, y, ok, testCase.x, testCase.y, testCase.wantOK)
			}
		})
	}
}

func TestLayoutWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(l *dataset.Layout)
	}{
		{name: "no tree", want: "layout desert: no tree of that variant", change: func(l *dataset.Layout) { l.Variant = "desert" }},
		{name: "bounds", want: "bounds min above max", change: func(l *dataset.Layout) { l.MinX = 300 }},
		{name: "orbit without slots", want: "orbit 1 has no slots", change: func(l *dataset.Layout) { l.Orbits[1].Slots = 0 }},
		{name: "unknown ring orbit", want: "group 7: ring orbit 2 not in layout", change: func(l *dataset.Layout) { l.Groups[1].Orbits = []uint8{0, 2} }},
		{name: "groups unsorted", want: "group 3: id 0, not sorted or repeated", change: func(l *dataset.Layout) {
			l.Groups[0], l.Groups[1] = l.Groups[1], l.Groups[0]
		}},
		{name: "group repeated", want: "group 3: id 0, not sorted or repeated", change: func(l *dataset.Layout) { l.Groups[1].ID = 3 }},
		{name: "group zero", want: "group 0: id 0", change: func(l *dataset.Layout) { l.Groups[0].ID = 0; l.Nodes[0].Group = 0 }},
		{name: "background without image", want: "group 7: background without image", change: func(l *dataset.Layout) { l.Groups[1].Background.Image = "" }},
		{name: "node missing", want: "1 nodes, tree has 2", change: func(l *dataset.Layout) { l.Nodes = l.Nodes[:1] }},
		{name: "node not in tree", want: "node 5 not in tree", change: func(l *dataset.Layout) { l.Nodes[1].ID = 5 }},
		{name: "no icon", want: "node 2: no icon", change: func(l *dataset.Layout) { l.Nodes[1].Icon = "" }},
		{name: "orbit without group", want: "node 2: orbit without group", change: func(l *dataset.Layout) { l.Nodes[1].Group = 0 }},
		{name: "unknown group", want: "node 2: group 8 not in layout", change: func(l *dataset.Layout) { l.Nodes[1].Group = 8 }},
		{name: "unknown orbit", want: "node 2: orbit 2 index 2 not in layout", change: func(l *dataset.Layout) { l.Nodes[1].Orbit = 2 }},
		{name: "unknown orbit index", want: "node 2: orbit 1 index 16 not in layout", change: func(l *dataset.Layout) { l.Nodes[1].OrbitIndex = 16 }},
		{name: "unknown class", want: "class art for class 2: no such class", change: func(l *dataset.Layout) { l.ClassArt[0].Class = 2 }},
		{name: "class art repeated", want: "class art for class 0: not sorted or repeated", change: func(l *dataset.Layout) {
			l.ClassArt = append(l.ClassArt, l.ClassArt[0])
		}},
		{name: "class art unsorted", want: "class art for class 0: not sorted or repeated", change: func(l *dataset.Layout) {
			l.ClassArt = append([]dataset.ClassArt{{Class: 1, Image: "Art/x.png"}}, l.ClassArt...)
		}},
		{name: "class art without image", want: "class art for class 0 has no image", change: func(l *dataset.Layout) { l.ClassArt[0].Image = "" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withLayout()
			testCase.change(&d.Layouts[0])
			_, err := dataset.Write(t.TempDir(), d)
			if !errors.Is(err, dataset.ErrInvalid) || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Write: got %v, want ErrInvalid containing %q", err, testCase.want)
			}
		})
	}
	t.Run("repeated", func(t *testing.T) {
		t.Parallel()
		d := withLayout()
		d.Layouts = append(d.Layouts, d.Layouts[0])
		if _, err := dataset.Write(t.TempDir(), d); !errors.Is(err, dataset.ErrInvalid) {
			t.Errorf("Write: got %v, want ErrInvalid", err)
		}
	})
}

// withLayoutFile returns the files of fsys with the layout file stored as name with content b,
// and the manifest listing it so.
func withLayoutFile(t *testing.T, fsys fstest.MapFS, name string, b []byte) fstest.MapFS {
	t.Helper()
	out := maps.Clone(fsys)
	var m dataset.Manifest
	if err := json.Unmarshal(fsys[dataset.ManifestName].Data, &m); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(m.Files, func(f dataset.File) bool { return f.Name == "layout-standard.fb" })
	sum := sha256.Sum256(b)
	m.Files[i] = dataset.File{Name: name, Size: len(b), SHA256: hex.EncodeToString(sum[:])}
	delete(out, "layout-standard.fb")
	out[name] = &fstest.MapFile{Data: b}
	m.ID = dataset.ContentID(m)
	mb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	out[dataset.ManifestName] = &fstest.MapFile{Data: mb}
	return out
}

func TestLayoutReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, withLayout())
	layout := good["layout-standard.fb"].Data
	wrongID := bytes.Clone(layout)
	copy(wrongID[4:8], "PBTR")

	_, err := dataset.Open(withLayoutFile(t, good, "layout-desert.fb", layout))
	if !errors.Is(err, dataset.ErrInvalid) || !strings.Contains(err.Error(), "layout desert has no tree") {
		t.Errorf("Open with a layout without tree: got %v, want ErrInvalid", err)
	}
	testCases := []struct {
		name, variant, want string
		fsys                fstest.MapFS
		wantErr             error
	}{
		{name: "variant differs from file name", variant: "ruthless", want: `holds layout variant "standard"`,
			fsys: withLayoutFile(t, good, "layout-ruthless.fb", layout), wantErr: dataset.ErrInvalid},
		{name: "wrong identifier", variant: "standard", want: "wrong file identifier",
			fsys: withLayoutFile(t, good, "layout-standard.fb", wrongID), wantErr: dataset.ErrInvalid},
		{name: "not in dataset", variant: "ruthless", want: "no ruthless layout", fsys: good, wantErr: dataset.ErrNotInDataset},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Layout(testCase.variant); !errors.Is(err, testCase.wantErr) || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Layout(%s): got %v, want %v containing %q", testCase.variant, err, testCase.wantErr, testCase.want)
			}
		})
	}
}

func TestCompareLayouts(t *testing.T) {
	t.Parallel()
	a, b := withLayout(), withLayout()
	l := &b.Layouts[0]
	l.MaxX = 300
	l.Groups[0].X = 0
	l.Groups = append(l.Groups, dataset.LayoutGroup{ID: 9, Orbits: []uint8{}})
	l.Nodes[1].Icon = "Art/other.png"
	b.Trees[1].Nodes = []dataset.Node{{ID: 4}}
	b.Layouts = append(b.Layouts, dataset.Layout{Variant: "ruthless", Nodes: []dataset.LayoutNode{{ID: 4, Icon: "Art/x.png"}}})
	got := compare(t, a, b)
	want := []dataset.LayoutDiff{{
		Variant: "standard", BoundsChanged: true, GroupsAdded: []uint32{9}, GroupsChanged: []uint32{3},
		NodesMoved: []uint32{1}, ArtChanged: []uint32{2},
	}}
	if diff := gocmp.Diff(want, got.Layouts, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare layouts mismatch (-want +got):\n%s", diff)
	}
	if diff := gocmp.Diff([]string{"ruthless"}, got.LayoutVariantsAdded); diff != "" {
		t.Errorf("LayoutVariantsAdded mismatch (-want +got):\n%s", diff)
	}
	same := compare(t, withLayout(), withLayout())
	if diff := gocmp.Diff([]dataset.LayoutDiff{{Variant: "standard"}}, same.Layouts, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare same layouts: want no changes (-want +got):\n%s", diff)
	}
}

func TestOrbitAngle(t *testing.T) {
	t.Parallel()
	evenly := func(n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = 360 * float64(i) / float64(n)
		}
		return out
	}
	testCases := []struct {
		slots uint32
		want  []float64
	}{
		{slots: 1, want: []float64{0}},
		{slots: 6, want: []float64{0, 60, 120, 180, 240, 300}},
		// skilltree-export README, 3.17.0
		{slots: 16, want: []float64{0, 30, 45, 60, 90, 120, 135, 150, 180, 210, 225, 240, 270, 300, 315, 330}},
		// Path of Building, PassiveTree.lua CalcOrbitAngles
		{slots: 40, want: []float64{
			0, 10, 20, 30, 40, 45, 50, 60, 70, 80, 90, 100, 110, 120, 130, 135, 140, 150, 160, 170,
			180, 190, 200, 210, 220, 225, 230, 240, 250, 260, 270, 280, 290, 300, 310, 315, 320, 330, 340, 350,
		}},
		{slots: 72, want: evenly(72)},
	}
	for _, testCase := range testCases {
		t.Run(strconv.Itoa(int(testCase.slots)), func(t *testing.T) {
			t.Parallel()
			o := dataset.Orbit{Slots: testCase.slots}
			got := make([]float64, testCase.slots)
			for i := range got {
				got[i] = o.Angle(uint16(i))
			}
			if diff := gocmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("angles mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCompareLayoutArt(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		change func(n *dataset.LayoutNode)
	}{
		{name: "icon", change: func(n *dataset.LayoutNode) { n.Icon = "Art/other.png" }},
		{name: "mastery inactive icon", change: func(n *dataset.LayoutNode) { n.MasteryInactiveIcon = "Art/other.png" }},
		{name: "mastery active icon", change: func(n *dataset.LayoutNode) { n.MasteryActiveIcon = "Art/other.png" }},
		{name: "mastery active effect", change: func(n *dataset.LayoutNode) { n.MasteryActiveEffect = "Art/other.png" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := withLayout()
			testCase.change(&b.Layouts[0].Nodes[1])
			want := []dataset.LayoutDiff{{Variant: "standard", ArtChanged: []uint32{2}}}
			if diff := gocmp.Diff(want, compare(t, withLayout(), b).Layouts, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Compare layouts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
