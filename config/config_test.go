package config_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/xaroth/pbt-engine/config"
)

type stage string

var errStage = errors.New("bad stage")

func (s stage) Valid() error {
	if s != "early" && s != "late" {
		return errStage
	}
	return nil
}

const (
	keyStage   config.Key[stage] = "stage"
	keyCharges config.Key[int]   = "charges"
)

func specs() []config.Spec {
	return []config.Spec{config.SpecOf(keyStage, stage("late")), config.SpecOf(keyCharges, 0)}
}

func TestDecode(t *testing.T) {
	t.Parallel()
	if v, err := keyStage.Decode(json.RawMessage(`"early"`)); err != nil || v != "early" {
		t.Errorf("valid: got %q, %v", v, err)
	}
	for name, raw := range map[string]string{"not in set": `"middle"`, "wrong type": `3`, "trailing": `"early" 1`, "null": ` null `} {
		if _, err := keyStage.Decode(json.RawMessage(raw)); !errors.Is(err, config.ErrBadValue) {
			t.Errorf("%s: got %v, want ErrBadValue", name, err)
		}
	}
	if _, err := keyStage.Decode(json.RawMessage(`"middle"`)); !errors.Is(err, errStage) {
		t.Errorf("Valid's error not wrapped: %v", err)
	}
}

func TestValues(t *testing.T) {
	t.Parallel()
	var v config.Values
	if err := config.Set(&v, keyStage, "early"); err != nil {
		t.Fatal(err)
	}
	if err := config.Set(&v, keyStage, "middle"); !errors.Is(err, config.ErrBadValue) {
		t.Errorf("set invalid: got %v", err)
	}
	if got, ok, err := config.Get(v, keyStage); err != nil || !ok || got != "early" {
		t.Errorf("get: %q, %v, %v", got, ok, err)
	}
	if _, ok, err := config.Get(v, keyCharges); ok || err != nil {
		t.Errorf("get absent: %v, %v", ok, err)
	}
	c := v.Clone()
	c["stage"][1] = 'x'
	if got, _, _ := config.Get(v, keyStage); got != "early" {
		t.Errorf("clone shares bytes: original now %q", got)
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) != `{"stage":"early"}` {
		t.Errorf("json: %s, %v", b, err)
	}
	v.Clear("stage")
	if len(v) != 0 {
		t.Errorf("clear left %v", v)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	build := config.Values{"stage": json.RawMessage(`"early"`), "charges": json.RawMessage(`2`), "unknown": json.RawMessage(`1`)}
	sim := config.Values{"charges": json.RawMessage(`3`)}
	call := config.Values{"charges": json.RawMessage(`"four"`)}
	r, problems := config.Resolve(specs(),
		config.Layer{Source: config.FromCall, Values: call},
		config.Layer{Source: config.FromBuild, Values: build},
		config.Layer{Source: config.FromSim, Values: sim})
	if v, src := config.Lookup(r, keyCharges); v != 3 || src != config.FromSim {
		t.Errorf("charges: %v from %s, want 3 from sim (the call's value is bad)", v, src)
	}
	if v, src := config.Lookup(r, keyStage); v != "early" || src != config.FromBuild {
		t.Errorf("stage: %v from %s", v, src)
	}
	if len(problems) != 2 || !errors.Is(problems[0].Err, config.ErrBadValue) || problems[0].Source != config.FromCall ||
		!errors.Is(problems[1].Err, config.ErrUnknownKey) || problems[1].Key != "unknown" {
		t.Errorf("problems: %v", problems)
	}
	// null is not a value: it is reported and the build's value stays
	r, problems = config.Resolve(specs(), config.Layer{Source: config.FromSim, Values: config.Values{"stage": json.RawMessage(`null`)}},
		config.Layer{Source: config.FromBuild, Values: build})
	if v, src := config.Lookup(r, keyStage); v != "early" || src != config.FromBuild || len(problems) != 2 || !errors.Is(problems[0].Err, config.ErrBadValue) {
		t.Errorf("null in a higher layer: %v from %s, problems %v", v, src, problems)
	}
	r, _ = config.Resolve(specs())
	if v, src := config.Lookup(r, keyStage); v != "late" || src != config.Default {
		t.Errorf("default: %v from %s", v, src)
	}
	if v, src := config.Lookup(r, config.Key[bool]("absent")); v || src != config.Default {
		t.Errorf("absent key: %v from %s", v, src)
	}
}
