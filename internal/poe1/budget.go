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

// Budget is the points a character can spend: Passive is Level + Quests + Granted, the parts
// it is made of; Ascendancy is what the labyrinths give, for the ascendancy and the bloodline
// together.
type Budget struct {
	Passive    int `json:"passive"`
	Ascendancy int `json:"ascendancy"`
	// Level is the points of the character's level: level - 1.
	Level int `json:"level"`
	// Quests is the passive points of the quests the campaign stage reaches; killing all
	// bandits gives one more.
	Quests int `json:"quests"`
	// Granted is the passive points the allocated nodes grant.
	Granted int `json:"granted"`
}

// PointBudget returns the points c can spend under cfg, and warns when c allocates more.
// Ascendancy points are at most what all labyrinths give, since the config does not say which
// are done. A bloodline's nodes spend from the same ascendancy points and have no cap of their
// own (https://www.poewiki.net/wiki/Ascendancy_class). Nodes stay allocated. It fails when the quests group does not load.
func PointBudget(c *build.Checked, cfg config.Resolved) (Budget, []build.Warning, error) {
	quests, err := c.Store().Quests()
	if err != nil {
		return Budget{}, nil, fmt.Errorf("point budget: %w", err)
	}
	stage, bandits := configValues(cfg)
	b := c.Build()
	passive, ascendancy, granted := c.Points()
	bud := Budget{Level: b.Level - 1, Granted: granted}
	for _, q := range quests {
		bud.Ascendancy += int(q.AscendancyPoints)
		if questReached(q, stage, bandits) {
			bud.Quests += int(q.PassivePoints)
		}
	}
	bud.Passive = bud.Level + bud.Quests + bud.Granted
	var out []build.Warning
	if passive > bud.Passive {
		out = append(out, build.Warning{
			Code: PointsOverBudget, Subject: "passive points",
			Message: fmt.Sprintf("%d allocated, at most %d (level %d: %d, quests: %d, nodes: %d); kept",
				passive, bud.Passive, b.Level, bud.Level, bud.Quests, bud.Granted),
		})
	}
	if ascendancy > bud.Ascendancy {
		out = append(out, build.Warning{
			Code: AscendancyPointsOverBudget, Subject: "ascendancy points",
			Message: fmt.Sprintf("%d allocated, at most %d from the labyrinths; kept", ascendancy, bud.Ascendancy),
		})
	}
	return bud, out, nil
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
