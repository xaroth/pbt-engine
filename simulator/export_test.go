package simulator

import "github.com/xaroth/pbt-engine/build"

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
