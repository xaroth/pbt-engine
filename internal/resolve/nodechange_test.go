package resolve_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func TestChangeNode(t *testing.T) {
	t.Parallel()
	node := func() *dataset.Node {
		return &dataset.Node{
			ID: 7, Kind: dataset.Notable, Neighbours: []uint32{1, 9},
			Stats:          []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 8}},
			Buffs:          [][]dataset.StatValue{{{Stat: 5, Value: 5}}},
			MasteryEffects: []dataset.MasteryEffect{{ID: 1}},
		}
	}
	replace := resolve.NodeChange{Replace: true, Stats: []dataset.StatValue{{Stat: 4, Value: 5}}}
	add := resolve.NodeChange{Stats: []dataset.StatValue{{Stat: 4, Value: 2}, {Stat: 6, Value: 1}}}
	replaced, added, both := node(), node(), node()
	replaced.Stats, replaced.Buffs, replaced.MasteryEffects = []dataset.StatValue{{Stat: 4, Value: 5}}, nil, nil
	added.Stats = []dataset.StatValue{{Stat: 3, Value: 20}, {Stat: 4, Value: 10}, {Stat: 6, Value: 1}}
	both.Stats, both.Buffs, both.MasteryEffects = []dataset.StatValue{{Stat: 4, Value: 7}, {Stat: 6, Value: 1}}, nil, nil
	testCases := []struct {
		name    string
		changes []resolve.NodeChange
		want    *dataset.Node
	}{
		{"none", nil, node()},
		{"replacement", []resolve.NodeChange{replace}, replaced},
		{"addition", []resolve.NodeChange{add}, added},
		{"replacement then addition", []resolve.NodeChange{replace, add}, both},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			n := node()
			got := resolve.ChangeNode(n, testCase.changes...)
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(node(), n); diff != "" {
				t.Errorf("node changed (-want +got):\n%s", diff)
			}
		})
	}
}
