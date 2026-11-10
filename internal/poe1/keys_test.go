package poe1_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

func TestConfigKeys(t *testing.T) {
	t.Parallel()
	r, problems := config.Resolve(poe1.ConfigSpecs())
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if v, _ := config.Lookup(r, poe1.Campaign); v != poe1.CampaignComplete {
		t.Errorf("campaign default %q", v)
	}
	if v, _ := config.Lookup(r, poe1.Bandits); v != poe1.BanditsNotCompleted {
		t.Errorf("bandits default %q", v)
	}
	for _, raw := range []string{`"complete"`, `"acts-6-10"`, `"acts-1-5"`} {
		if _, err := poe1.Campaign.Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("campaign %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`""`, `"kill-all"`, `"help-alira"`, `"help-kraityn"`, `"help-oak"`} {
		if _, err := poe1.Bandits.Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("bandits %s: %v", raw, err)
		}
	}
	if _, err := poe1.Campaign.Decode(json.RawMessage(`"act-11"`)); !errors.Is(err, poe1.ErrConfigValue) {
		t.Errorf("bad campaign: %v", err)
	}
	if _, err := poe1.Bandits.Decode(json.RawMessage(`"help-eramir"`)); !errors.Is(err, poe1.ErrConfigValue) {
		t.Errorf("bad bandits: %v", err)
	}
	// "" is a valid bandit choice, so null must not decode to it
	if _, err := poe1.Bandits.Decode(json.RawMessage(`null`)); !errors.Is(err, config.ErrBadValue) {
		t.Errorf("null bandits: %v", err)
	}
}
