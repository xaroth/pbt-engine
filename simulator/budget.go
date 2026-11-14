package simulator

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// Budget returns the points the build c checked can spend, as Simulate gives it in
// Stats.Budget, without simulating. Config resolves as in Simulate; options other than config
// values are ignored. It fails when the quests group of c's dataset does not load.
func (s *Sim) Budget(c *build.Checked, opts ...Option) (Budget, error) {
	co, err := readCallOptions(opts)
	if err != nil {
		return Budget{}, err
	}
	cfg, _ := resolveConfig(co, s.Config(), c.Build().Config)
	b, _, err := poe1.PointBudget(c, cfg)
	if err != nil {
		return Budget{}, fmt.Errorf("simulator: %w", err)
	}
	return b, nil
}
