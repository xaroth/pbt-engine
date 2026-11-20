package timeless

import (
	"fmt"

	"github.com/xaroth/pbt-engine/dataset"
)

// SeedRange is the seeds a jewel of a version shows: First to Last, Step apart.
type SeedRange struct {
	First uint32 `json:"first"`
	Last  uint32 `json:"last"`
	Step  uint32 `json:"step"`
}

// Contains tells whether a jewel of the range can show seed.
func (r SeedRange) Contains(seed uint32) bool {
	return r.Step > 0 && seed >= r.First && seed <= r.Last && (seed-r.First)%r.Step == 0
}

// Count is the number of seeds in the range.
func (r SeedRange) Count() int {
	if r.Step == 0 || r.Last < r.First {
		return 0
	}
	return int((r.Last-r.First)/r.Step) + 1
}

// Seeds returns the seed range of each alternate tree version of the tree variant of s, by
// version, made once per store. It is read from the ranges of the jewel mods' seed stat; a
// version no mod names has the zero range.
func Seeds(s *dataset.Store, variant string) ([]SeedRange, error) {
	//nolint:wrapcheck // the build function wraps its errors
	return dataset.Derived(s, seedsKey{variant}, func(s *dataset.Store) ([]SeedRange, error) {
		versions, err := VersionsOf(s, variant)
		if err != nil {
			return nil, err
		}
		stats, err := s.Stats()
		if err != nil {
			return nil, fmt.Errorf("timeless: %w", err)
		}
		mods, err := s.Mods()
		if err != nil {
			return nil, fmt.Errorf("timeless: %w", err)
		}
		return seedRanges(versions, mods, stats), nil
	})
}

// seedsKey keys the seed ranges of a tree variant among a store's derived values.
type seedsKey struct{ variant string }

func seedRanges(versions *Versions, mods []dataset.Mod, stats []dataset.Stat) []SeedRange {
	out := make([]SeedRange, len(versions.rows))
	for _, m := range mods {
		version, seeds := seedStats(&m, stats)
		if versions.Name(version) == "" || seeds.Max < seeds.Min || seeds.Min < 0 {
			continue
		}
		step := uint32(1)
		if versions.rows[version].ID == eternalID {
			step = eternalSeedStep
		}
		r := SeedRange{First: uint32(seeds.Min) * step, Last: uint32(seeds.Max) * step, Step: step} //nolint:gosec // checked not negative above
		if prev := out[version]; prev.Step > 0 {
			r.First, r.Last = min(r.First, prev.First), max(r.Last, prev.Last)
		}
		out[version] = r
	}
	return out
}

// seedStats gives the version m fixes and the range of its seed stat; version -1 when it fixes
// none.
func seedStats(m *dataset.Mod, stats []dataset.Stat) (int, dataset.ModStat) {
	version, seeds := -1, dataset.ModStat{}
	for _, ms := range m.Stats {
		switch stats[ms.Stat].ID {
		case VersionStat:
			if ms.Min == ms.Max {
				version = int(ms.Min)
			}
		case SeedStat:
			seeds = ms
		}
	}
	return version, seeds
}
