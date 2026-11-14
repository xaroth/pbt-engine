package resolve

import (
	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// GemRules are a game's rules for the levels and quality of gems.
type GemRules struct {
	// Bonuses are the stats that add levels or quality to gems.
	Bonuses []GemBonus
	// MatchingSocketQuality is the quality a gem gains in a socket of its own colour.
	MatchingSocketQuality int
	// AlwaysMatchingStat is an item stat with which every gem socketed in the item gains
	// MatchingSocketQuality, whatever the socket's colour.
	AlwaysMatchingStat string
}

// socketColours returns the colour of the socket each gem of groups is in, by group and gem;
// 0 for a gem in no socket. The gems of the groups in one slot fill the item's sockets in group
// and gem order, as the build does not say which socket holds which gem.
func socketColours(groups []build.SkillGroup, items build.Items) [][]dataset.GemColour {
	out := make([][]dataset.GemColour, len(groups))
	used := map[build.Slot]int{}
	for i, g := range groups {
		out[i] = make([]dataset.GemColour, len(g.Gems))
		if g.Slot == "" {
			continue
		}
		sockets := socketsOf(items[g.Slot].Sockets)
		for j := range g.Gems {
			if k := used[g.Slot] + j; k < len(sockets) {
				out[i][j] = sockets[k]
			}
		}
		used[g.Slot] += len(g.Gems)
	}
	return out
}

// socketsOf returns the colours of the sockets item text writes as s, such as "R-G B", in
// order; a socket of no gem colour, such as an abyssal one, is 0.
func socketsOf(s string) []dataset.GemColour {
	var out []dataset.GemColour
	for _, c := range s {
		switch c {
		case '-', ' ':
		case 'R':
			out = append(out, dataset.Red)
		case 'G':
			out = append(out, dataset.Green)
		case 'B':
			out = append(out, dataset.Blue)
		case 'W':
			out = append(out, dataset.White)
		default:
			out = append(out, 0)
		}
	}
	return out
}

// alwaysMatching returns the slots of the items with stat, outside the weapon swap.
func alwaysMatching(items build.Items, stat string) map[build.Slot]bool {
	out := map[build.Slot]bool{}
	if stat == "" {
		return out
	}
	for s, it := range items {
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				if sv.Stat == stat && sv.Value != 0 && !s.Swap() {
					out[s] = true
				}
			}
		}
	}
	return out
}

// socketQuality returns the quality gem src gains from its socket's colour.
func (r *skillResolver) socketQuality(src GemSource, gem build.Gem) int {
	colour := r.sockets[src.Group][src.Gem]
	if colour == 0 {
		return 0
	}
	dg := r.data.Gem(gem.ID)
	if dg == nil {
		return 0
	}
	if dg.Colour == colour || r.alwaysMatching[r.groups[src.Group].Slot] {
		return r.matchingQuality
	}
	return 0
}
