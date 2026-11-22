package poe1

import (
	"reflect"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestSkillKeyStat(t *testing.T) {
	t.Parallel()
	skill := func(dmg float64) *resolve.Skill {
		return &resolve.Skill{
			Source: resolve.GemSource{ID: "Fireball"}, GrantedEffect: "Fireball", Level: 20,
			StatSets: []resolve.SkillStatSet{{ID: "Fireball", Stats: []resolve.StatValue{
				{Stat: "spell_minimum_base_fire_damage", Value: dmg}, {Stat: "base_cast_speed", Value: 1},
			}}},
		}
	}
	a, b := appendSkill(nil, skill(100)), appendSkill(nil, skill(101))
	if string(a) == string(b) {
		t.Error("skills that differ in one stat value: same key")
	}
	if string(a) != string(appendSkill(nil, skill(100))) {
		t.Error("equal skills: different keys")
	}
}

// Changing any one field of a skill, at any depth, or the length of any of its slices, changes
// its key; this fails when a field is added to resolve.Skill and not to appendSkill.
func TestSkillKeyEveryField(t *testing.T) {
	t.Parallel()
	base := func() *resolve.Skill {
		sk := &resolve.Skill{}
		fillOne(reflect.ValueOf(sk).Elem())
		return sk
	}
	want := string(appendSkill(nil, base()))
	for _, p := range skillPaths(reflect.TypeFor[resolve.Skill](), nil, "Skill") {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			sk := base()
			v := reflect.ValueOf(sk).Elem()
			for _, i := range p.steps {
				if v.Kind() == reflect.Slice {
					v = v.Index(i)
				} else {
					v = v.Field(i)
				}
			}
			change(t, v)
			if string(appendSkill(nil, sk)) == want {
				t.Errorf("%s changed: same key", p.name)
			}
		})
	}
}

type skillPath struct {
	name  string
	steps []int
}

// skillPaths returns the path to every value of type t, slices included, through the first
// element of each slice.
func skillPaths(t reflect.Type, steps []int, name string) []skillPath {
	out := []skillPath{{name, steps}}
	switch t.Kind() {
	case reflect.Struct:
		for i := range t.NumField() {
			out = append(out, skillPaths(t.Field(i).Type, append(slices.Clip(steps), i), name+"."+t.Field(i).Name)...)
		}
		return out[1:]
	case reflect.Slice:
		out = append(out, skillPaths(t.Elem(), append(slices.Clip(steps), 0), name+"[0]")...)
	default:
	}
	return out
}

// fillOne gives every slice under v one element and every interface a GemSource.
func fillOne(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for _, field := range v.Fields() {
			fillOne(field)
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fillOne(v.Index(0))
	case reflect.Interface:
		v.Set(reflect.ValueOf(resolve.GemSource{}))
	default:
	}
}

func change(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "x")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(v.Uint() + 1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 1)
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	case reflect.Interface:
		v.Set(reflect.ValueOf(resolve.ItemSource{Name: "x"}))
	default:
		t.Fatalf("no change for a %s", v.Kind())
	}
}
