package simulator

import (
	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
)

// IdleSheets exposes the pool size to tests.
func (s *Sim) IdleSheets() int { return s.idleSheets() }

// MaxIdle is the pool limit.
const MaxIdle = maxIdle

// FastPath reports whether a sweep over base would run edits on the fast path.
func FastPath(s *Sim, base *build.Build, edits []build.Edit) (bool, error) {
	sw, err := s.newSweep(base, nil)
	if err != nil {
		return false, err
	}
	_, ok := sw.fastPlan(edits)
	return ok, nil
}

// ReadChanges exposes the fast path's output reads.
func ReadChanges(keys []output.Key, before map[output.Key]float64, read func(output.Key) (float64, error)) (map[output.Key]Change, error) {
	return readChanges(keys, before, read)
}
