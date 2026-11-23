package pob

import (
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/poe1"
)

// config writes the bandit choice into f's <Build> and its config set, the campaign stage as
// the resistance penalty, and the enemy, condition and count keys as PoB's inputs. Without
// a bandit choice PoB's default, kill all, applies.
func (ex *exporter) config(f *outFile) error {
	v := ex.b.Config
	held, heldKeys, unwritable, err := heldInputs(v)
	if err != nil {
		return err
	}
	ex.warnConfigLeftOut(v, heldKeys)
	for _, name := range unwritable {
		ex.warnf(ConfigNotExported, name, "PoB reads 0 in this input as empty and uses its boss preset; not exported")
	}
	var inputs []outInput
	bandit, ok, err := config.Get(v, poe1.Bandits)
	if err != nil {
		return fmt.Errorf("pob: %w", err)
	}
	if ok {
		if f.Build.Bandit, ok = pobBandit(bandit); ok {
			// PoB reads the <Build> attribute only when the config set does not replace it.
			inputs = append(inputs, outInput{Name: banditInput, String: f.Build.Bandit})
		} else {
			ex.warnf(ConfigNotExported, string(poe1.Bandits), "PoB has no choice for the bandit quest not completed; its default, kill all, applies")
		}
	}
	stage, ok, err := config.Get(v, poe1.Campaign)
	if err != nil {
		return fmt.Errorf("pob: %w", err)
	}
	if ok {
		inputs = append(inputs, outInput{Name: penaltyInput, Number: pobPenalty(stage)})
	}
	inputs = append(inputs, held...)
	if len(inputs) > 0 {
		f.Config = &outConfig{Active: "1", Sets: []outConfigSet{{ID: "1", Inputs: inputs}}}
	}
	return nil
}

// heldInputs returns the PoB inputs of v's enemy, condition and count keys and the names of
// the keys, and the enemy inputs PoB cannot be given.
func heldInputs(v config.Values) (inputs []outInput, keys map[string]bool, unwritable []string, err error) {
	enemy, keys, unwritable, err := enemyInputs(v)
	if err != nil {
		return nil, nil, nil, err
	}
	conditions, conditionKeys, err := conditionOutputs(v)
	if err != nil {
		return nil, nil, nil, err
	}
	counts, countKeys, err := countOutputs(v)
	if err != nil {
		return nil, nil, nil, err
	}
	maps.Copy(keys, conditionKeys)
	for _, k := range countKeys {
		keys[k] = true
	}
	return slices.Concat(enemy, conditions, counts), keys, unwritable, nil
}

// warnConfigLeftOut warns of each config key of v that config does not export: not the
// bandits, the campaign or one of exported.
func (ex *exporter) warnConfigLeftOut(v config.Values, exported map[string]bool) {
	for _, name := range v.Names() {
		if name != string(poe1.Bandits) && name != string(poe1.Campaign) && !exported[name] {
			ex.warnf(ConfigNotExported, name, "config value not exported")
		}
	}
}

// pobBandit is the inverse of banditOf; PoB has no choice for the quest not completed.
func pobBandit(b poe1.BanditChoice) (string, bool) {
	switch b {
	case poe1.BanditsKillAll:
		return none, true
	case poe1.BanditsHelpAlira:
		return "Alira", true
	case poe1.BanditsHelpKraityn:
		return "Kraityn", true
	case poe1.BanditsHelpOak:
		return "Oak", true
	case poe1.BanditsNotCompleted:
	}
	return "", false
}

// pobPenalty is the inverse of stageOf; config.Get rejects other stages.
func pobPenalty(s poe1.CampaignStage) string {
	switch s {
	case poe1.CampaignActs6To10:
		return "-30"
	case poe1.CampaignActs1To5:
		return "0"
	case poe1.CampaignComplete:
	}
	return "-60"
}

type outConfig struct {
	Active string         `xml:"activeConfigSet,attr"`
	Sets   []outConfigSet `xml:"ConfigSet"`
}

type outConfigSet struct {
	ID     string     `xml:"id,attr"`
	Inputs []outInput `xml:"Input"`
}

type outInput struct {
	Name    string `xml:"name,attr"`
	Number  string `xml:"number,attr,omitempty"`
	String  string `xml:"string,attr,omitempty"`
	Boolean string `xml:"boolean,attr,omitempty"`
}
