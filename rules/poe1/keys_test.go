package poe1_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
)

func TestConfigKeys(t *testing.T) {
	t.Parallel()
	r, problems := config.Resolve(rules.ConfigSpecs())
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if v, _ := config.Lookup(r, rules.Campaign); v != rules.CampaignComplete {
		t.Errorf("campaign default %q", v)
	}
	if v, _ := config.Lookup(r, rules.Bandits); v != rules.BanditsNotCompleted {
		t.Errorf("bandits default %q", v)
	}
	for _, raw := range []string{`"complete"`, `"acts-6-10"`, `"acts-1-5"`} {
		if _, err := rules.Campaign.Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("campaign %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`""`, `"kill-all"`, `"help-alira"`, `"help-kraityn"`, `"help-oak"`} {
		if _, err := rules.Bandits.Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("bandits %s: %v", raw, err)
		}
	}
	if _, err := rules.Campaign.Decode(json.RawMessage(`"act-11"`)); !errors.Is(err, rules.ErrConfigValue) {
		t.Errorf("bad campaign: %v", err)
	}
	if _, err := rules.Bandits.Decode(json.RawMessage(`"help-eramir"`)); !errors.Is(err, rules.ErrConfigValue) {
		t.Errorf("bad bandits: %v", err)
	}
	// "" is a valid bandit choice, so null must not decode to it
	if _, err := rules.Bandits.Decode(json.RawMessage(`null`)); !errors.Is(err, config.ErrBadValue) {
		t.Errorf("null bandits: %v", err)
	}
}
