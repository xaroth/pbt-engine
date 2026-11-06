package semantics

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Coverage renders a markdown report of t: counts per status, every entry that feeds the
// sheet with its modifiers or the reason it is unknown, most used first, and the reasons of
// the remaining unknown entries by kind.
func Coverage(t *Table) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Semantics coverage, %s %s\n\nPoB %s. Stats on standard tree nodes and mastery effects, plus overrides.\n\n", t.Game, t.Version, t.PoB)
	type row struct {
		status Status
		sheet  bool
	}
	counts := map[row]int{}
	uses := map[row]int{}
	for _, e := range t.Entries {
		r := row{e.Status, e.Sheet && e.Status == Unknown}
		counts[r]++
		uses[r] += e.Uses
	}
	b.WriteString("| Status | Stats | Uses |\n|---|---|---|\n")
	for _, r := range []row{{Generated, false}, {Override, false}, {Unknown, true}, {Unknown, false}, {NotSheet, false}} {
		name := string(r.status)
		if r.status == Unknown {
			name += map[bool]string{true: ", feeds the sheet", false: ", may feed the sheet"}[r.sheet]
		}
		fmt.Fprintf(&b, "| %s | %d | %d |\n", name, counts[r], uses[r])
	}

	byUse := slices.Clone(t.Entries)
	slices.SortStableFunc(byUse, func(x, y Entry) int { return cmp.Compare(y.Uses, x.Uses) })

	b.WriteString("\n## Sheet stats\n\n| Stat | Status | Uses | Modifiers |\n|---|---|---|---|\n")
	for _, e := range byUse {
		if e.Status == Generated || e.Status == Override {
			fmt.Fprintf(&b, "| `%s` | %s | %d | %s |\n", e.Stat, e.Status, e.Uses, modsText(e.Mods))
		}
	}
	unknown := func(title string, sheet bool) {
		fmt.Fprintf(&b, "\n## %s\n\n| Stat | Uses | Reason |\n|---|---|---|\n", title)
		for _, e := range byUse {
			if e.Status == Unknown && e.Sheet == sheet {
				fmt.Fprintf(&b, "| `%s` | %d | %s |\n", e.Stat, e.Uses, strings.ReplaceAll(e.Reason, "|", "\\|"))
			}
		}
	}
	unknown("Unknown, feeds the sheet", true)

	// Reasons of the other unknowns quote game text, so only their kinds are counted.
	kinds := map[string]int{}
	for _, e := range t.Entries {
		if e.Status == Unknown && !e.Sheet {
			seen := map[string]bool{}
			for _, r := range strings.Split(e.Reason, "; ") {
				if k := reasonKind(r); !seen[k] {
					seen[k] = true
					kinds[k]++
				}
			}
		}
	}
	b.WriteString("\n## Unknown, may feed the sheet\n\n| Reason | Stats |\n|---|---|\n")
	for _, k := range slices.Sorted(maps.Keys(kinds)) {
		fmt.Fprintf(&b, "| %s | %d |\n", k, kinds[k])
	}
	return b.String()
}

func reasonKind(reason string) string {
	for _, prefix := range []string{"PoB does not parse", "line not in PoB's ModCache", "no description line"} {
		if strings.HasPrefix(reason, prefix) {
			return prefix
		}
	}
	return reason
}

func modsText(mods []Mod) string {
	parts := make([]string, len(mods))
	for i, m := range mods {
		parts[i] = fmt.Sprintf("%s %s", m.Op, m.Stat)
		if m.Op != OpFlag && (m.Mul != 1 || m.Div != 1) {
			parts[i] += fmt.Sprintf(" x%v/%v", m.Mul, m.Div)
		}
	}
	return strings.Join(parts, ", ")
}
