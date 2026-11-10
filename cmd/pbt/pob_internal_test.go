package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pobXML is a PoB build of a level 40 Marauder with ascendancy and nodes as given, two items
// and the acts 6 to 10 resistance penalty.
func pobXML(ascendancy, nodes string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<PathOfBuilding>
	<Build level="40" className="Marauder" ascendClassName="` + ascendancy + `" bandit="None"/>
	<Tree activeSpec="1"><Spec treeVersion="3_29" nodes="` + nodes + `"/></Tree>
	<Config activeConfigSet="1"><ConfigSet id="1"><Input name="resistancePenalty" number="-30"/></ConfigSet></Config>
	<Items><Item id="1">Rarity: RARE
Doom Crown
Royal Burgonet</Item><Item id="2">Rarity: UNIQUE
Kaom's Heart
Glorious Plate</Item></Items>
</PathOfBuilding>`
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildImport(t *testing.T) {
	t.Parallel()
	root, path := writeRoot(t), writeFile(t, "pob.xml", pobXML("Ancestral Commander", "1,12345"))
	var out, notes bytes.Buffer
	if err := run(t.Context(), []string{"build", "import", path, "--root", root}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"ascendancy": "Juggernaut"`, `"level": 40`, `"campaign": "acts-6-10"`, `12345`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("build JSON lacks %s:\n%s", want, out.String())
		}
	}
	if got := strings.Count(notes.String(), "warning pob-item-ignored"); got != 2 {
		t.Errorf("notes list %d item warnings, want 2:\n%s", got, notes.String())
	}
	// The imported JSON is a build file every command takes.
	imported := writeFile(t, "build.json", out.String())
	if err := run(t.Context(), []string{"build", "check", imported, "--root", root}, &out, &notes); err != nil {
		t.Errorf("check of the imported build: %v", err)
	}
	if err := run(t.Context(), []string{"build", "import", imported, "--root", root}, &out, &notes); !errors.Is(err, errNotPoB) {
		t.Errorf("import of build JSON: got %v, want errNotPoB", err)
	}
}

// Commands that take a build take a PoB file too, with its import warnings folded.
func TestCommandsTakePoB(t *testing.T) {
	t.Parallel()
	root, path := writeSimRoot(t), writeFile(t, "pob.xml", pobXML("None", "1,2"))
	for _, args := range [][]string{
		{"build", "check", path},
		{"eval", path},
		{"compare", path, path},
		{"power", path},
	} {
		var out, notes bytes.Buffer
		if err := run(t.Context(), append(args, "--root", root), &out, &notes); err != nil {
			t.Errorf("%v: %v", args, err)
			continue
		}
		if !strings.Contains(notes.String(), "warning pob-item-ignored: 2 (Doom Crown, ...)") {
			t.Errorf("%v: notes lack the folded item warnings:\n%s", args, notes.String())
		}
	}
	var out, notes bytes.Buffer
	if err := run(t.Context(), []string{"eval", path, "--root", root, "--warnings", "all"}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(notes.String(), "warning pob-item-ignored"); got != 2 {
		t.Errorf("--warnings all: %d item warnings, want 2:\n%s", got, notes.String())
	}
}
