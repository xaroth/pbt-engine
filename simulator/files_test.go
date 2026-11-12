package simulator_test

import (
	"encoding/xml"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/simulator"
)

// TestFilesReal runs each call on the pinned 3.29 dataset with the Path of Building files in
// testdata/builds, taken from pbt-conformance's 3.29 test builds, and compares the files the
// dataset opened with the files its function names. A call whose reads depend on the file,
// import and reading items, opens no more than its function names on each build, and all
// of it on some build.
//
//nolint:tparallel // the builds run in parallel inside one group, which ends before the checks over all of them
func TestFilesReal(t *testing.T) {
	t.Parallel()
	m := realdata.Open(t, realdata.Current).Manifest()
	paths, err := filepath.Glob(filepath.Join("testdata", "builds", "*.xml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no test builds: %v", err)
	}
	var mu sync.Mutex
	imported, itemsRead := map[string]bool{}, map[string]bool{}
	t.Run("builds", func(t *testing.T) {
		for _, path := range paths {
			t.Run(filepath.Base(path), func(t *testing.T) {
				t.Parallel()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				b, opened := importRecorded(t, m, data)
				tree := b.Clone()
				tree.Skills = build.Skills{}
				simulateRecorded(t, m, tree)
				simulateRecorded(t, m, b)
				withItems, read := readItemsRecorded(t, m, b, data)
				simulateRecorded(t, m, withItems)
				compareRecorded(t, m, tree)
				mu.Lock()
				defer mu.Unlock()
				for _, f := range opened {
					imported[f] = true
				}
				for _, f := range read {
					itemsRead[f] = true
				}
			})
		}
	})
	checkAll(t, "import", simulator.ImportFiles(m), imported)
	checkAll(t, "reading items", simulator.ItemFiles(m), itemsRead)
}

// checkAll checks that the builds together opened every file declared.
func checkAll(t *testing.T, call string, declared []string, opened map[string]bool) {
	t.Helper()
	if diff := gocmp.Diff(sorted(declared), slices.Sorted(maps.Keys(opened))); diff != "" {
		t.Errorf("files %s opened over all builds (-declared +opened):\n%s", call, diff)
	}
}

// recorded returns a sim over a recorded copy of the pinned 3.29 dataset, and its registry.
func recorded(t *testing.T) (*realdata.Recorder, *dataset.Registry, *simulator.Sim) {
	t.Helper()
	rec := realdata.Record(t, realdata.Current)
	reg := &dataset.Registry{}
	s, err := reg.Add(rec)
	if err != nil {
		t.Fatal(err)
	}
	sim, err := simulator.New(s)
	if err != nil {
		t.Fatal(err)
	}
	return rec, reg, sim
}

// importRecorded imports data and returns the build and the files the import opened.
func importRecorded(t *testing.T, m dataset.Manifest, data []byte) (*build.Build, []string) {
	t.Helper()
	rec, reg, _ := recorded(t)
	b, _, err := pob.FromXML(data, reg)
	if err != nil {
		t.Fatal(err)
	}
	opened := rec.Opened()
	if extra := missing(opened, simulator.ImportFiles(m)); len(extra) > 0 {
		t.Errorf("import opened %v, which ImportFiles lacks", extra)
	}
	return b, opened
}

// simulateRecorded simulates b and explains every output in every view.
func simulateRecorded(t *testing.T, m dataset.Manifest, b *build.Build) {
	t.Helper()
	rec, _, sim := recorded(t)
	st, err := sim.Simulate(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range st.Keys() {
		for _, v := range []simulator.View{simulator.Tree, simulator.Proportional, simulator.Marginal} {
			if _, err := st.Explain(k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if diff := gocmp.Diff(sorted(simulator.SimulateFiles(m, b)), rec.Opened()); diff != "" {
		t.Errorf("simulate and explain, %d gem groups, %d items (-SimulateFiles +opened):\n%s", len(b.Skills.Groups), len(b.Items), diff)
	}
}

// compareRecorded compares b with b less its last node, and sweeps that edit.
func compareRecorded(t *testing.T, m dataset.Manifest, b *build.Build) {
	t.Helper()
	less := b.Clone()
	if len(less.Nodes) > 0 {
		less.Nodes = less.Nodes[:len(less.Nodes)-1]
	}
	rec, _, sim := recorded(t)
	if _, err := sim.Compare(b, less); err != nil {
		t.Fatal(err)
	}
	if diff := gocmp.Diff(sorted(simulator.SimulateFiles(m, b, less)), rec.Opened()); diff != "" {
		t.Errorf("compare (-SimulateFiles +opened):\n%s", diff)
	}
	rec, _, sim = recorded(t)
	if _, err := sim.Sweep(b, [][]build.Edit{{build.Deallocate(b.Nodes...)}}); err != nil {
		t.Fatal(err)
	}
	if diff := gocmp.Diff(sorted(simulator.SimulateFiles(m, b)), rec.Opened()); diff != "" {
		t.Errorf("sweep (-SimulateFiles +opened):\n%s", diff)
	}
}

type pobItem struct {
	ID   string `xml:"id,attr"`
	Text string `xml:",chardata"`
}

// readItemsRecorded reads the items of the file's first item set into b's slots, as a host
// would, and returns b with them and the files reading them opened.
func readItemsRecorded(t *testing.T, m dataset.Manifest, b *build.Build, data []byte) (*build.Build, []string) {
	t.Helper()
	var file struct {
		Items []pobItem `xml:"Items>Item"`
		Slots []struct {
			Name   string `xml:"name,attr"`
			ItemID string `xml:"itemId,attr"`
		} `xml:"Items>ItemSet>Slot"`
	}
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	slots := map[string]build.Slot{
		"Weapon 1": build.SlotWeapon, "Weapon 2": build.SlotOffhand, "Helmet": build.SlotHelmet,
		"Body Armour": build.SlotBodyArmour, "Gloves": build.SlotGloves, "Boots": build.SlotBoots,
		"Amulet": build.SlotAmulet, "Ring 1": build.SlotRing1, "Ring 2": build.SlotRing2, "Belt": build.SlotBelt,
	}
	rec, _, sim := recorded(t)
	out := b.Clone()
	equipped := map[build.Slot]bool{}
	for _, s := range file.Slots {
		slot, ok := slots[s.Name]
		i := slices.IndexFunc(file.Items, func(it pobItem) bool { return it.ID == s.ItemID })
		// Only the first item set: later sets repeat the slots.
		if !ok || i < 0 || equipped[slot] {
			continue
		}
		equipped[slot] = true
		it, _, err := sim.Data().Item(file.Items[i].Text)
		if err != nil {
			continue // such as a Relic, which the reader does not take yet
		}
		if err := out.Apply(build.Equip(slot, it)); err != nil {
			t.Fatal(err)
		}
	}
	opened := rec.Opened()
	if extra := missing(opened, simulator.ItemFiles(m)); len(extra) > 0 {
		t.Errorf("reading items opened %v, which ItemFiles lacks", extra)
	}
	return out, opened
}

func sorted(s []string) []string { return slices.Sorted(slices.Values(s)) }

// missing returns the files of got that want lacks.
func missing(got, want []string) []string {
	var out []string
	for _, f := range got {
		if !slices.Contains(want, f) {
			out = append(out, f)
		}
	}
	return out
}
