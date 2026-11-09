package poe1

import (
	"slices"

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

// SetConfig applies cfg to the sheet: each quest reward's source goes on or off by the campaign
// stage and bandit choice. A reward these rules have no grant for stays off, noted once.
func (sh *Sheet) SetConfig(cfg config.Resolved) {
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
		if !known && !sh.noted(core.SourceID(id)) {
			sh.notes = append(sh.notes, sourcedNote{
				note:   Note{Kind: NotApplied, Source: q, Scope: src.Bundle.Scope, Text: "no campaign rule grants quest flag " + q.Flag},
				src:    core.SourceID(id),
				always: true,
			})
		}
		// id is a source of this session, so this cannot fail.
		_ = sh.session.SetSourceEnabled(core.SourceID(id), known && grant(c, b))
	}
}

func (sh *Sheet) noted(src core.SourceID) bool {
	return slices.ContainsFunc(sh.notes, func(n sourcedNote) bool { return n.always && n.src == src })
}

// SetLevel switches the sheet to character level: the level bundle of that level goes on,
// the others off. A level not seen yet gets its bundle added once, so a sheet serves every
// level without being opened again.
func (sh *Sheet) SetLevel(level int) error {
	found := false
	for id, src := range sh.sources {
		if src.Bundle == nil {
			continue
		}
		if _, ok := src.Bundle.Source.(resolve.LevelSource); !ok {
			continue
		}
		on := levelOf(src.Bundle) == float64(level)
		found = found || on
		// id is a source of this session, so this cannot fail.
		_ = sh.session.SetSourceEnabled(core.SourceID(id), on)
	}
	if found {
		return nil
	}
	bd := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: resolve.LevelSource{},
		Stats: []resolve.StatValue{{Stat: resolve.LevelStat, Value: float64(level)}}}
	return sh.add(&bd, sh.table, sh.gameVersion)
}

func levelOf(bd *resolve.Bundle) float64 {
	for _, sv := range bd.Stats {
		if sv.Stat == resolve.LevelStat {
			return sv.Value
		}
	}
	return 0
}
