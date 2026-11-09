package simulator

// IdleSheets exposes the pool size to tests.
func (s *Sim) IdleSheets() int { return s.idleSheets() }

// MaxIdle is the pool limit.
const MaxIdle = maxIdle
