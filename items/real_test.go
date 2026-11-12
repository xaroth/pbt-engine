package items_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
)

// realReader opens the reader of the current real dataset, and skips without $PBT_DATASETS.
func realReader(tb testing.TB) *items.Reader {
	tb.Helper()
	r, err := items.NewReader(realdata.Open(tb, realdata.Current))
	if err != nil {
		tb.Fatal(err)
	}
	return r
}

// TestSampleBases parses every item of the statdesc sample and checks the bases of one name:
// each Two-Stone Ring is the one whose implicit its implicit line reads as.
func TestSampleBases(t *testing.T) {
	t.Parallel()
	r := realReader(t)
	want := map[string]string{
		"rare Two-Stone Ring (Fire/Cold)": "Metadata/Items/Rings/Ring14",
		"rare Two-Stone Ring":             "Metadata/Items/Rings/Ring12",
	}
	for _, s := range sampleItems(t) {
		it, _, err := r.Parse(s.text)
		if err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		if w, ok := want[s.label]; ok && it.Base != w {
			t.Errorf("%s: base %s, want %s", s.label, it.Base, w)
		}
	}
}

type sampleItem struct {
	label, text string
}

// sampleItems returns the items of the statdesc sample as Path of Building text, labelled by
// rarity and base as the sample names them.
func sampleItems(t *testing.T) []sampleItem {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "statdesc", "testdata", "item-lines.txt"))
	if err != nil {
		t.Fatal(err)
	}
	head := regexp.MustCompile(`(?m)^# (normal|magic|rare|unique) (.+?), (\d+) implicits?((?:, [^,\n]+ Item)*)$`)
	var out []sampleItem
	for p := range strings.SplitSeq(string(data), "\n\n") {
		m := head.FindStringSubmatch(p)
		if m == nil {
			t.Fatalf("paragraph without its item: %q", p)
		}
		var mods []string
		for l := range strings.Lines(p) {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				mods = append(mods, l)
			}
		}
		name := m[2]
		if m[1] == "rare" || m[1] == "unique" {
			name = "Sample\n" + name
		}
		name += strings.ReplaceAll(m[4], ", ", "\n")
		text := fmt.Sprintf("Rarity: %s\n%s\nImplicits: %s\n%s\n", strings.ToUpper(m[1]), name, m[3], strings.Join(mods, "\n"))
		out = append(out, sampleItem{label: m[1] + " " + m[2], text: text})
	}
	return out
}
