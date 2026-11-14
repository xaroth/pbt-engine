package resolve

import (
	"fmt"

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
	// LinkedSupports are item stats with which the supports socketed in the item also support
	// the skills socketed in another slot.
	LinkedSupports []LinkedSupport
}

// LinkedSupport is an item stat with which the supports socketed in the item, the gems' and
// those the item grants, also support the skills socketed in slot To.
type LinkedSupport struct {
	Stat string
	To   build.Slot
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

// socketQuality returns the quality gem src gains from its socket's colour. White is no colour a
// socket and a gem can match by, as in Path of Building.
func (r *skillResolver) socketQuality(src GemSource, gem build.Gem) int {
	colour := r.sockets[src.Group][src.Gem]
	if colour == 0 {
		return 0
	}
	dg := r.data.Gem(gem.ID)
	if dg == nil {
		return 0
	}
	if dg.Colour == colour && colour != dataset.White || r.alwaysMatching[r.groups[src.Group].Slot] {
		return r.matchingQuality
	}
	return 0
}

// socketedSupports returns the supports the items in socketed slots grant to the gems socketed
// in them, by slot, in the order of their mod lines.
func (r *skillResolver) socketedSupports(store *dataset.Store, items build.Items) (map[build.Slot][]supportGem, error) {
	out := map[build.Slot][]supportGem{}
	var index *dataset.ItemSupports
	for _, slot := range build.Slots() {
		it, ok := items[slot]
		if !ok {
			continue
		}
		for _, m := range it.Mods {
			for _, sv := range m.Stats {
				if index == nil {
					var err error
					if index, err = store.ItemSupports(); err != nil {
						return nil, fmt.Errorf("resolve: item supports: %w", err)
					}
				}
				for _, g := range index.Supports(sv.Stat, sv.Value) {
					if ge := r.data.GrantedEffect(g.Effect); ge != nil {
						out[slot] = append(out[slot], supportGem{src: ItemSource{Slot: slot, Name: it.Name}, effect: ge, level: int(g.Level)})
					}
				}
			}
		}
	}
	return out, nil
}

// linkedSlots returns, by slot, the slots of the items whose stats in links make their
// supports also support the skills socketed in it. Items in the weapon swap link none.
func linkedSlots(items build.Items, links []LinkedSupport) map[build.Slot][]build.Slot {
	out := map[build.Slot][]build.Slot{}
	for _, slot := range build.Slots() {
		it, ok := items[slot]
		if !ok || slot.Swap() {
			continue
		}
		for _, l := range links {
			if hasStat(it, l.Stat) {
				out[l.To] = append(out[l.To], slot)
			}
		}
	}
	return out
}

func hasStat(it build.Item, stat string) bool {
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			if sv.Stat == stat && sv.Value != 0 {
				return true
			}
		}
	}
	return false
}
