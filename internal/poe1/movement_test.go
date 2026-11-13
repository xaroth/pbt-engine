package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestMovementSpeed(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		stats []resolve.StatValue
		want  float64
	}{
		{name: "base", want: 100},
		{name: "increased", stats: []resolve.StatValue{{Stat: "base_movement_velocity_+%", Value: 30}, {Stat: "base_movement_velocity_+%", Value: 5}}, want: 135},
		{name: "reduced", stats: []resolve.StatValue{{Stat: "base_movement_velocity_+%", Value: -3}}, want: 97},
		{
			name: "per frenzy charge",
			stats: []resolve.StatValue{
				{Stat: "base_movement_velocity_+%", Value: 25}, {Stat: "max_frenzy_charges", Value: 3}, {Stat: "base_minimum_frenzy_charges", Value: 2},
				{Stat: "movement_velocity_+%_per_frenzy_charge", Value: 4},
			},
			want: 133,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(marauder(1, testCase.stats...)...))
			if got := value(t, sh, "movement_speed"); got != testCase.want {
				t.Errorf("movement_speed: got %v, want %v", got, testCase.want)
			}
		})
	}
}
