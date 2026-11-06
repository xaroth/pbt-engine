package core

import "math"

// NearWrap moves the revision and epoch counters next to their wraparound.
func NearWrap(s *Session) {
	s.rev = math.MaxUint32 - 3
	s.epoch = math.MaxUint32 - 2
	for i := range s.blocks {
		for j := range s.blocks[i].recs {
			r := &s.blocks[i].recs[j]
			if r.epoch != 0 {
				r.epoch = s.epoch
				r.verified = s.rev
			}
		}
	}
}
