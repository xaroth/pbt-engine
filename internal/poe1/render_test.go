package poe1_test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// render writes ex as text for goldens, naming sources by id.
func render(t *testing.T, ex *poe1.Explanation) string {
	t.Helper()
	var b strings.Builder
	r := &renderer{w: &b}
	r.header(ex)
	switch ex.View {
	case poe1.Tree:
		r.tree(ex.Tree, 1)
	case poe1.Proportional:
		r.proportional(ex)
	case poe1.Marginal:
		r.marginal(ex)
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	return b.String()
}

type renderer struct {
	w   io.Writer
	err error
}

func (r *renderer) printf(format string, args ...any) {
	if r.err == nil {
		_, r.err = fmt.Fprintf(r.w, format, args...)
	}
}

func (r *renderer) header(ex *poe1.Explanation) {
	r.printf("%s: %s%s", ex.Output.Label, num(ex.Value), ex.Output.Unit)
	if over := ex.Amount - ex.Effective; over > 0 && strings.HasSuffix(ex.Output.Unit, poe1.UnitPercent) {
		r.printf(" (+%s%s over)", num(over), ex.Output.Unit)
	}
	r.printf("  [%s, dataset %s]\n", ex.View, ex.Version)
	if ex.View == poe1.Marginal {
		r.printf("  change without each source: value (total before caps)\n")
	}
	if num(ex.Amount) != num(ex.Effective) {
		r.printf("  total %s, effective %s", num(ex.Amount), num(ex.Effective))
		if len(ex.CapBy) > 0 {
			r.printf(", capped by %s", strings.Join(ex.CapBy, ", "))
		}
		r.printf("\n")
	}
}

func (r *renderer) proportional(ex *poe1.Explanation) {
	for _, l := range ex.Lines {
		r.printf("  %10s  %s", num(l.Value), l.Source)
		if len(l.Parts) == 1 && len(l.Parts[0].Via) == 0 && l.Parts[0].From == nil {
			r.printf(" (%s)\n", opName(l.Parts[0].Op))
			continue
		}
		r.printf("\n")
		for _, p := range l.Parts {
			r.printf("  %10s    %s %s", "", opName(p.Op), num(p.Value))
			if len(p.Via) > 0 {
				r.printf(" via %s", strings.Join(p.Via, " < "))
			}
			if p.From != nil {
				r.printf(" applying %s", *p.From)
			}
			r.printf("\n")
		}
	}
}

func (r *renderer) marginal(ex *poe1.Explanation) {
	for _, l := range ex.Lines {
		r.printf("  %10s  %s", num(-l.Value), l.Source)
		if num(l.Total) != num(l.Value) {
			r.printf(" (total %s)", num(-l.Total))
		}
		r.printf("\n")
	}
}

func (r *renderer) tree(t *poe1.TreeNode, depth int) {
	pad := strings.Repeat("  ", depth)
	if t.Repeat {
		r.printf("%s%s = %s (above)\n", pad, t.Stat, num(t.Value))
		return
	}
	r.printf("%s%s = %s", pad, t.Stat, num(t.Value))
	if t.Formula != "" {
		r.printf(" [%s]", t.Formula)
	}
	if num(t.Total) != num(t.Capped) {
		r.printf(" (total %s, effective %s)", num(t.Total), num(t.Capped))
	}
	if t.Overridden {
		r.printf(" (overridden)")
	}
	r.printf("\n")
	for _, tt := range t.Terms {
		r.printf("%s  %s %s  %s", pad, opName(tt.Op), num(tt.Value), tt.Source)
		if tt.Stat != "" {
			r.printf(" on %s", tt.Stat)
		}
		if tt.Rate != 1 {
			r.printf(" at %s%%", num(tt.Rate*100))
		}
		if !tt.Held {
			r.printf(" (condition not met)")
		}
		r.printf("\n")
	}
	for _, c := range t.Reads {
		r.tree(c, depth+1)
	}
}

func opName(op core.Op) string {
	if op == core.OpInc {
		return "increased"
	}
	return op.String()
}

// num formats v with at most two decimals.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
