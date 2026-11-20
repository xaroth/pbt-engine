package build_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestCheckAcrossLeagues opens one 3.30 Witch build on the pinned dataset of every league. It
// holds a keystone new in 3.29, a runegraft new in 3.28 and the Abyssal bloodline new in 3.29.
// No league removes content, so an older league is where a node, override or bloodline is gone.
func TestCheckAcrossLeagues(t *testing.T) {
	t.Parallel()
	const (
		keystone = 12128 // new in 3.29
		mastery  = 4492  // in every league
	)
	b := &build.Build{
		Format:    build.Format,
		Game:      "poe1",
		Version:   "3.30",
		Class:     "Metadata/Characters/Int/Int",
		Bloodline: "Abyssal",
		Level:     90,
		Nodes: []uint32{
			57226, 21678, 32210, 8948, 38176, 49651, 5296, 19501, keystone, // to the keystone
			27592, 44924, 55114, mastery,
		},
		Overrides: map[uint32]string{mastery: "runegraft_26"},
	}
	bloodline := build.Warning{Code: build.BloodlineUnknown, Subject: "bloodline Abyssal"}
	node := build.Warning{Code: build.NodeUnknown, Subject: "node 12128"}
	testCases := []struct {
		version string
		want    []build.Warning
	}{
		{realdata.Oldest, []build.Warning{
			bloodline,
			node,
			{Code: build.OverrideUnknown, Subject: "node 4492"},
			// The mastery stays allocated without the runegraft that stood for its effect.
			{Code: build.MasteryNotChosen, Subject: "node 4492"},
		}},
		{realdata.Previous, []build.Warning{bloodline, node}},
		{realdata.Current, nil},
		{realdata.InGame, nil},
	}
	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			t.Parallel()
			_, got, err := build.Check(b, realdata.Open(t, tc.version))
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(build.Warning{}, "Message"), cmpopts.EquateEmpty()); d != "" {
				t.Errorf("warnings (-want +got):\n%s", d)
			}
		})
	}
}
