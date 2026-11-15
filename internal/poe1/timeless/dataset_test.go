package timeless_test

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// keystoneID is a keystone row's id: its conqueror's index, and _v2 for the second revision.
var keystoneID = regexp.MustCompile(`^[a-z]+_keystone_(\d+)(_v2)?$`)

// Each keystone row's conqueror, which a jewel's stats are matched against, agrees with its row
// id on every pinned dataset that has the alternate tree tables.
func TestKeystoneConquerors(t *testing.T) {
	t.Parallel()
	for _, version := range realdata.Versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			tree, err := realdata.Open(t, version).Tree(build.DefaultVariant)
			if err != nil {
				t.Fatal(err)
			}
			if len(tree.TimelessVersions) == 0 {
				return
			}
			keystones := 0
			for _, v := range tree.TimelessVersions {
				for _, r := range v.Replacements {
					want, ok := conquerorOfID(r.ID)
					if ok != (len(r.Kinds) == 1 && r.Kinds[0] == dataset.TimelessKeystone) {
						t.Errorf("%s %s: kinds %v", v.ID, r.ID, r.Kinds)
					} else if ok && r.Conqueror != want {
						t.Errorf("%s %s: conqueror %+v, want %+v", v.ID, r.ID, r.Conqueror, want)
					}
					if ok {
						keystones++
					}
				}
			}
			if keystones == 0 {
				t.Error("no keystone rows")
			}
		})
	}
}

// conquerorOfID reads the conqueror from a keystone row's id, or reports false for another row.
func conquerorOfID(id string) (dataset.TimelessConqueror, bool) {
	m := keystoneID.FindStringSubmatch(id)
	if m == nil {
		return dataset.TimelessConqueror{}, false
	}
	index, err := strconv.ParseUint(m[1], 10, 32)
	if err != nil {
		return dataset.TimelessConqueror{}, false
	}
	c := dataset.TimelessConqueror{Index: uint32(index)}
	if m[2] != "" {
		c.Version = 1
	}
	return c, true
}
