package statdesc_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

//nolint:gochecknoglobals // the testing package parses flags before any test runs, so the flag must exist at package init
var update = flag.Bool("update", false, "rewrite the read golden files from $PBT_DATASETS")

const (
	realVersion = "3.29.3.3"
	mainScope   = "Metadata/StatDescriptions/stat_descriptions.txt"
)

// realText opens the English text of the real dataset under $PBT_DATASETS, and skips without
// it.
func realText(tb testing.TB) (*statdesc.Set, statdesc.Scope) {
	tb.Helper()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		tb.Skip("PBT_DATASETS not set")
	}
	reg, err := dataset.OpenRoot(os.DirFS(root))
	if reg == nil {
		tb.Fatal(err)
	}
	res, err := reg.Resolve("poe1", realVersion)
	if err != nil || res.Store.Manifest().Version != realVersion {
		tb.Skipf("PBT_DATASETS has no %s", realVersion)
	}
	text, err := res.Store.Text("en")
	if err != nil {
		tb.Fatal(err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		tb.Fatal(err)
	}
	i := slices.IndexFunc(text.Descriptions, func(s statdesc.Scope) bool { return s.Name == mainScope })
	if i < 0 {
		tb.Fatalf("no scope %s", mainScope)
	}
	return set, text.Descriptions[i]
}

// TestReadRoundTrip renders every variant of the main description file with sample values and
// reads the text back. Variants that do not read back to their own stats are listed in the
// golden file with the reason; so are those whose text another description renders too and
// that the game's order does not prefer.
func TestReadRoundTrip(t *testing.T) {
	t.Parallel()
	set, sc := realText(t)
	var failed, ambiguous []string
	variants, rendered, readBack, first := 0, 0, 0, 0
	for i, d := range sc.Descriptions {
		for j, v := range d.Variants {
			variants++
			id := fmt.Sprintf("%d/%d %s", i, j, strings.Join(d.Stats, ","))
			text, why := renderVariant(set, d, v)
			if why != "" {
				failed = append(failed, id+" | "+why)
				continue
			}
			rendered++
			readings, err := set.Read(mainScope, strings.Split(text, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			k := slices.IndexFunc(readings, func(r statdesc.Reading) bool { return slices.Equal(r.Stats, d.Stats) })
			switch {
			case k < 0:
				failed = append(failed, fmt.Sprintf("%s | %s | %q", id, noReading(readings), text))
				continue
			case k > 0:
				ambiguous = append(ambiguous, fmt.Sprintf("%s | read first as %s | %q", id, strings.Join(readings[0].Stats, ","), text))
			default:
				first++
			}
			readBack++
		}
	}
	t.Logf("%d variants, %d rendered, %d read back (%.2f%% of rendered, %.2f%% of all), %d preferred by the game's order",
		variants, rendered, readBack, pct(readBack, rendered), pct(readBack, variants), first)
	if pct(readBack, rendered) < 99 {
		t.Errorf("%.2f%% of rendered variants read back, want at least 99%%", pct(readBack, rendered))
	}
	header := fmt.Sprintf("# Variants of %s on %s that do not read back to their stats (description/variant stats | reason | text),\n"+
		"# then those read back but not first. %d variants, %d rendered, %d read back, %d first.\n",
		mainScope, realVersion, variants, rendered, readBack, first)
	golden(t, "read-roundtrip-"+realVersion+".txt", header+strings.Join(failed, "\n")+"\n\n# Read back but not first\n"+strings.Join(ambiguous, "\n")+"\n")
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }

func noReading(readings []statdesc.Reading) string {
	if len(readings) == 0 {
		return "no reading"
	}
	others := make([]string, 0, len(readings))
	for _, r := range readings {
		others = append(others, strings.Join(r.Stats, ","))
	}
	return "read only as " + strings.Join(others, "; ")
}

// renderVariant renders v with sample values inside its limits, through Describe, as the game
// would show the description's stats; the reason when it renders no line of d or another
// variant.
func renderVariant(set *statdesc.Set, d statdesc.Description, v statdesc.Variant) (string, string) {
	stats := make(map[string]int32, len(d.Stats))
	for i, st := range d.Stats {
		stats[st] = sample(v.Limits[i], i)
	}
	lines, err := set.Describe(mainScope, stats)
	if err != nil {
		return "", "not rendered: " + err.Error()
	}
	for _, l := range lines {
		if slices.Equal(l.Stats, d.Stats) {
			return l.Text, ""
		}
	}
	return "", "not rendered: another description of its stats renders them"
}

// sample returns a value inside l for stat i: a multiple of 60, so most handlers that round
// show it exactly, negated or clamped to l when it does not fit.
func sample(l statdesc.Limit, i int) int32 {
	p := int32(120 + 60*i)
	if l.Not {
		return p + 60*int32(boolInt(l.Min == p))
	}
	for _, v := range []int32{p, -p} {
		if v >= l.Min && v <= l.Max {
			return v
		}
	}
	if l.Max > 0 && l.Max < p {
		return l.Max
	}
	return l.Min
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// golden compares got with testdata/name, or writes it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs; run with -update and review the diff", path)
	}
}

// TestReadItemLines reads the mod lines of real items, one item per paragraph of the sample,
// and lists the first reading of each line or lines, and how many others they have.
func TestReadItemLines(t *testing.T) {
	t.Parallel()
	set, _ := realText(t)
	data, err := os.ReadFile(filepath.Join("testdata", "item-lines.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	lines, matched, ambiguous := 0, 0, 0
	for item := range strings.SplitSeq(string(data), "\n\n") {
		var mods []string
		for l := range strings.Lines(item) {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				mods = append(mods, l)
			}
		}
		for i := 0; i < len(mods); {
			readings, err := set.Read(mainScope, mods[i:])
			if err != nil {
				t.Fatal(err)
			}
			if len(readings) == 0 {
				out = append(out, mods[i]+" | no reading")
				lines++
				i++
				continue
			}
			r := readings[0]
			lines += r.Lines
			matched += r.Lines
			s := strings.Join(mods[i:i+r.Lines], " / ") + " | " + describe(r)
			if len(readings) > 1 {
				ambiguous += r.Lines
				s += fmt.Sprintf(" | %d other readings", len(readings)-1)
			}
			out = append(out, s)
			i += r.Lines
		}
	}
	t.Logf("%d lines, %d read (%.2f%%), %d of them ambiguous", lines, matched, pct(matched, lines), ambiguous)
	golden(t, "read-item-lines-"+realVersion+".txt", "# lines | first reading | readings beside it\n"+strings.Join(out, "\n")+"\n")
}

func describe(r statdesc.Reading) string {
	parts := make([]string, len(r.Stats))
	for i, st := range r.Stats {
		parts[i] = fmt.Sprintf("%s=%d", st, r.Values[i])
	}
	return strings.Join(parts, " ")
}

// BenchmarkRead reads one line on the real main scope, its index already built.
func BenchmarkRead(b *testing.B) {
	set, _ := realText(b)
	lines := []string{"+101 to maximum Life"}
	if _, err := set.Read(mainScope, lines); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := set.Read(mainScope, lines); err != nil {
			b.Fatal(err)
		}
	}
}
