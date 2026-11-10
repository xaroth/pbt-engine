package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of PointBudget.
const (
	PointsOverBudget           build.Code = "points-over-budget"
	AscendancyPointsOverBudget build.Code = "ascendancy-points-over-budget"
)

// banditKillAllFlag grants the passive point for killing all three bandits.
const banditKillAllFlag = "a2q7-QuestRewardTaken"

// PointBudget warns when c allocates more passive or ascendancy points than a character can
// have under cfg: level - 1, plus the passive points of the quests the campaign stage reaches
// (killing all bandits gives one more), plus points allocated nodes grant; ascendancy points
// are at most what the labyrinths give, since the config does not say which are done. Nodes
// stay allocated. It fails when the quests group does not load.
func PointBudget(c *build.Checked, cfg config.Resolved) ([]build.Warning, error) {
	quests, err := c.Store().Quests()
	if err != nil {
		return nil, fmt.Errorf("point budget: %w", err)
	}
	stage, bandits := configValues(cfg)
	questPoints, labPoints := 0, 0
	for _, q := range quests {
		labPoints += int(q.AscendancyPoints)
		if questReached(q, stage, bandits) {
			questPoints += int(q.PassivePoints)
		}
	}
	b := c.Build()
	passive, ascendancy, granted := c.Points()
	var out []build.Warning
	if budget := b.Level - 1 + questPoints + granted; passive > budget {
		out = append(out, build.Warning{
			Code: PointsOverBudget, Subject: "passive points",
			Message: fmt.Sprintf("%d allocated, at most %d (level %d: %d, quests: %d, nodes: %d); kept",
				passive, budget, b.Level, b.Level-1, questPoints, granted),
		})
	}
	if ascendancy > labPoints {
		out = append(out, build.Warning{
			Code: AscendancyPointsOverBudget, Subject: "ascendancy points",
			Message: fmt.Sprintf("%d allocated, at most %d from the labyrinths; kept", ascendancy, labPoints),
		})
	}
	return out, nil
}

// questReached reports whether a character at campaign stage s with bandit choice b has the
// reward of q. Acts up to the stage count, except the act-ending Kitava quest of a stage not
// yet past it; the bandit point needs the kill.
func questReached(q dataset.Quest, s CampaignStage, b BanditChoice) bool {
	switch {
	case q.Flag == banditKillAllFlag:
		return b == BanditsKillAll
	case q.Quest == "":
		return false
	case s == CampaignActs1To5:
		return q.Act <= 5 && q.Quest != "a5q6"
	case s == CampaignActs6To10:
		return q.Act <= 10 && q.Quest != "a10q3"
	}
	return true
}
