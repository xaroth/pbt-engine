package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

var (
	errSet  = errors.New("--set wants key=value")
	errView = errors.New("--view wants tree, proportional or marginal")
)

// simFor reads a build file and returns it with a sim over the dataset its version (or
// version, when set) resolves to under --root.
func simFor(in invocation, path, version string) (*build.Build, *simulator.Sim, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read build: %w", err)
	}
	b, err := build.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	res, err := resolveVersions(in.notes, in.opts["root"], b.Game, cmp.Or(version, b.Version))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	s, err := simulator.New(res[0].Store)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, s, nil
}

// setOptions turns --set key=value options into call options. A value that is not JSON is
// taken as a string, so --set campaign=acts-1-5 works.
func setOptions(sets string) ([]simulator.Option, error) {
	var out []simulator.Option
	for line := range strings.SplitSeq(strings.TrimSpace(sets), "\n") {
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%w: %q", errSet, line)
		}
		raw := json.RawMessage(v)
		if !json.Valid(raw) {
			var err error
			if raw, err = json.Marshal(v); err != nil {
				return nil, fmt.Errorf("--set %s: %w", k, err)
			}
		}
		out = append(out, simulator.SetRaw(k, raw))
	}
	return out, nil
}

func view(name string) (poe1.View, error) {
	for _, v := range []poe1.View{poe1.Tree, poe1.Proportional, poe1.Marginal} {
		if v.String() == name {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%w: %q", errView, name)
}

func eval(in invocation) error {
	b, s, err := simFor(in, in.pos[1], in.opts["version"])
	if err != nil {
		return err
	}
	opts, err := setOptions(in.opts["set"])
	if err != nil {
		return err
	}
	st, err := s.Simulate(b, opts...)
	if err != nil {
		return fmt.Errorf("%s: %w", in.pos[1], err)
	}
	p := &printer{w: in.w}
	p.printf("%s %s on dataset %s\n", b.Game, cmp.Or(b.Version, "?"), st.Version())
	printSheet(p, st)
	printConfig(p, st)
	printSimWarnings(p, st.Warnings(), in.opts["warnings"] == "all")
	if in.opts["explain"] != "" {
		if err := explain(in, p, st); err != nil {
			return err
		}
	}
	return p.err
}

// printSheet prints every output: label, value and unit, and how far a capped resistance is over.
func printSheet(p *printer, st *simulator.Stats) {
	for _, o := range poe1.Outputs() {
		v, ok := st.Get(o.Key)
		if !ok {
			continue
		}
		p.printf("%s: %s%s", o.Label, number(v, o.Places), o.Unit)
		if total, ok := st.Get(uncapped(o.Key)); ok && total > v {
			p.printf(" (+%s%s over)", number(total-v, o.Places), o.Unit)
		}
		p.printf("\n")
	}
}

// uncapped returns the uncapped output of a capped resistance, such as player.uncapped_fire_resistance.
func uncapped(k output.Key) output.Key {
	name, ok := strings.CutPrefix(string(k), "player.")
	if !ok || !strings.HasSuffix(name, "_resistance") || strings.HasPrefix(name, "maximum_") || strings.HasPrefix(name, "uncapped_") {
		return ""
	}
	return output.Key("player.uncapped_" + name)
}

func number(v float64, places int) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.*f", max(places, 1), v)
}

func printConfig(p *printer, st *simulator.Stats) {
	for _, spec := range poe1.ConfigSpecs() {
		if e, ok := st.Config(namedKey(spec.Name)); ok {
			p.printf("config %s = %q (%s)\n", spec.Name, fmt.Sprint(e.Value), e.Source)
		}
	}
}

// namedKey names a config key by its string.
type namedKey string

func (k namedKey) Name() string { return string(k) }

func explain(in invocation, p *printer, st *simulator.Stats) error {
	v, err := view(in.opts["view"])
	if err != nil {
		return err
	}
	ex, err := st.Explain(output.Key(in.opts["explain"]), v)
	if err != nil {
		return fmt.Errorf("explain: %w", err)
	}
	text, err := st.Text(in.opts["language"])
	if err != nil {
		return fmt.Errorf("explain: %w", err)
	}
	nm, err := poe1.NewNamer(text)
	if err != nil {
		return fmt.Errorf("explain: %w", err)
	}
	if p.err != nil {
		return p.err
	}
	p.printf("\n")
	if err := poe1.Render(p.w, ex, nm); err != nil {
		return fmt.Errorf("explain: %w", err)
	}
	return nil
}

func compare(in invocation) error {
	a, sa, err := simFor(in, in.pos[1], in.opts["version"])
	if err != nil {
		return err
	}
	b, sb, err := simFor(in, in.pos[2], cmp.Or(in.opts["version-b"], in.opts["version"]))
	if err != nil {
		return err
	}
	opts, err := setOptions(in.opts["set"])
	if err != nil {
		return err
	}
	var d *simulator.Diff
	if sa.Store() == sb.Store() || sa.Store().Manifest().ID == sb.Store().Manifest().ID {
		d, err = sa.Compare(a, b, opts...)
	} else {
		d, err = diffAcross(sa, sb, a, b, opts)
	}
	if err != nil {
		return fmt.Errorf("compare: %w", err)
	}
	return printDiff(in.w, d, in.opts["warnings"] == "all")
}

func diffAcross(sa, sb *simulator.Sim, a, b *build.Build, opts []simulator.Option) (*simulator.Diff, error) {
	x, err := sa.Simulate(a, opts...)
	if err != nil {
		return nil, fmt.Errorf("simulate: %w", err)
	}
	y, err := sb.Simulate(b, opts...)
	if err != nil {
		return nil, fmt.Errorf("simulate: %w", err)
	}
	d, err := x.Diff(y)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	return d, nil
}

func printDiff(w io.Writer, d *simulator.Diff, all bool) error {
	p := &printer{w: w}
	p.printf("%s -> %s\n", d.Before.Version(), d.After.Version())
	changed := d.Changed()
	if len(changed) == 0 {
		p.printf("no output changed\n")
	}
	for _, o := range poe1.Outputs() {
		if !slices.Contains(changed, o.Key) {
			continue
		}
		c := d.Get(o.Key)
		p.printf("%s: %s -> %s%s (%+g", o.Label, number(c.Before, o.Places), number(c.After, o.Places), o.Unit, round2(c.Abs))
		if !math.IsNaN(c.Pct) {
			p.printf(", %+.2f%%", c.Pct)
		}
		p.printf(")\n")
	}
	for _, e := range d.Edits {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("edit: %w", err)
		}
		p.printf("edit %s\n", data)
	}
	for _, nc := range d.DataChanges {
		p.printf("data node %d changed (%s): %v -> %v\n", nc.Node, strings.Join(nc.Fields, ", "), nc.Before, nc.After)
	}
	printSimWarnings(p, d.Before.Warnings(), all)
	printSimWarnings(p, d.After.Warnings(), all)
	return p.err
}

// printSimWarnings prints warnings, folding stats left out into one line unless all.
func printSimWarnings(p *printer, ws []simulator.Warning, all bool) {
	left := 0
	for _, w := range ws {
		if w.Code == simulator.UnmodelledStat && !all {
			left++
			continue
		}
		p.printf("warning %s\n", w)
	}
	if left > 0 {
		p.printf("warning %d stats left out as not modelled yet (--warnings all lists them)\n", left)
	}
}

// hundredths rounds a change to two decimals for display.
const hundredths = 100

func round2(v float64) float64 { return math.Round(v*hundredths) / hundredths }
