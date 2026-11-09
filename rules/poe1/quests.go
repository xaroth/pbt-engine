package poe1

import (
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/resolve"
)

// questGrants says, per quest flag with stats, under which config a character has the reward:
// the two Kitava defeats (the resistance penalties) by campaign stage, the bandit helps by
// choice. Flags are the game's QuestFlags ids, as in QuestStaticRewards.
func questGrants() map[string]func(CampaignStage, BanditChoice) bool {
	return map[string]func(CampaignStage, BanditChoice) bool{
		"a5q6-CompletedBoss":    func(c CampaignStage, _ BanditChoice) bool { return c != CampaignActs1To5 },
		"EndGameAvailable":      func(c CampaignStage, _ BanditChoice) bool { return c == CampaignComplete },
		"a2q1-QuestRewardTaken": func(_ CampaignStage, b BanditChoice) bool { return b == BanditsHelpKraityn },
		"a2q2-QuestRewardTaken": func(_ CampaignStage, b BanditChoice) bool { return b == BanditsHelpAlira },
		"a2q3-QuestRewardTaken": func(_ CampaignStage, b BanditChoice) bool { return b == BanditsHelpOak },
	}
}

// DefaultConfig returns the poe1 config with every key at its default.
func DefaultConfig() config.Resolved {
	r, _ := config.Resolve(ConfigSpecs())
	return r
}

// configValues returns the campaign stage and bandit choice of cfg, defaults for keys it
// lacks.
func configValues(cfg config.Resolved) (CampaignStage, BanditChoice) {
	c, b := CampaignComplete, BanditsNotCompleted
	if v, src := config.Lookup(cfg, Campaign); src != config.Default || v != "" {
		c = v
	}
	if v, _ := config.Lookup(cfg, Bandits); v != "" {
		b = v
	}
	return c, b
}

// applyQuests turns each quest reward's source on or off by cfg. A reward these rules have no
// grant for stays off, with a note.
func (sh *Sheet) applyQuests(cfg config.Resolved) {
	c, b := configValues(cfg)
	grants := questGrants()
	for id, src := range sh.sources {
		if src.Bundle == nil {
			continue
		}
		q, ok := src.Bundle.Source.(resolve.QuestSource)
		if !ok {
			continue
		}
		grant, known := grants[q.Flag]
		if !known {
			sh.Notes = append(sh.Notes, Note{Kind: NotApplied, Source: q, Scope: src.Bundle.Scope, Text: "no campaign rule grants quest flag " + q.Flag})
		}
		// id is a source of this session, so this cannot fail.
		_ = sh.session.SetSourceEnabled(core.SourceID(id), known && grant(c, b))
	}
}
