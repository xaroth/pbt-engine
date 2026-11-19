package poe1

import (
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

const (
	// statSocketedAbyssJewelEffect is Darkness Enthroned's "increased Effect of Socketed Abyss
	// Jewels".
	statSocketedAbyssJewelEffect = "local_socketed_abyss_jewel_effect_+%"
	// statCorruptedMagicJewelEffect is The Adorned's "increased Effect of Jewel Socket Passive
	// Skills containing Corrupted Magic Jewels".
	statCorruptedMagicJewelEffect = "local_unique_jewel_passive_jewel_socket_mod_effect_+%_with_corrupted_magic_jewel_socketed"
	// statEffectPerPassiveToStart is Split Personality's "increased effect per Allocated Passive
	// Skill between it and your Class' starting location".
	statEffectPerPassiveToStart = "local_jewel_+%_effect_per_passive_between_jewel_and_class_start" //nolint:gosec // a stat id, not a credential
)

// corruptedFlag is the item flag of a corrupted item.
const corruptedFlag = "Corrupted"

// AbyssJewelEffect returns the increased effect, in percent, of the abyss jewels socketed in an
// item with stats.
func AbyssJewelEffect(stats []resolve.StatValue) float64 {
	return sumStat(stats, statSocketedAbyssJewelEffect)
}

// SocketJewelEffect returns the increased effect, in percent, of jewel it in an allocated passive
// socket, where adorned is the summed The Adorned stat of the jewels in allocated sockets
// (AdornedEffect). It is adorned for a corrupted magic jewel and 0 for any other.
func SocketJewelEffect(it *build.Item, adorned float64) float64 {
	if it.Rarity != build.Magic || !slices.Contains(it.Flags, corruptedFlag) {
		return 0
	}
	return adorned
}

// AdornedEffect returns the increased effect, in percent, that jewels with stats in allocated
// passive sockets give the corrupted magic jewels there.
func AdornedEffect(stats ...[]resolve.StatValue) float64 {
	var sum float64
	for _, s := range stats {
		sum += sumStat(s, statCorruptedMagicJewelEffect)
	}
	return sum
}

// EffectPerPassiveToStart returns the increased effect, in percent, that a jewel with stats in a
// passive socket gains per allocated passive between the socket and the class start; 0 for a
// jewel without.
func EffectPerPassiveToStart(stats []resolve.StatValue) float64 {
	return sumStat(stats, statEffectPerPassiveToStart)
}

// ScaleJewel scales the stats of jewel it by inc percent increased effect and makes its key follow
// the effect, so the sheet replaces its source when the effect changes. A value is rounded to two
// decimals, then toward zero, as Path of Building does.
func ScaleJewel(it *Item, inc float64) {
	if inc == 0 {
		return
	}
	scale := 1 + inc/percentScale
	stats := make([]resolve.StatValue, len(it.Stats))
	for i, sv := range it.Stats {
		stats[i] = resolve.StatValue{Stat: sv.Stat, Value: math.Trunc(roundHundredths(sv.Value * scale))}
	}
	it.Stats = stats
	it.Key += fmt.Sprintf(" effect %+g%%", inc)
}

func sumStat(stats []resolve.StatValue, stat string) float64 {
	var sum float64
	for _, sv := range stats {
		if sv.Stat == stat {
			sum += sv.Value
		}
	}
	return sum
}
