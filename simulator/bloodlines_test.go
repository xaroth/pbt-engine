package simulator_test

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/simulator"
)

// TestBloodlines lists the 16 bloodlines of the 3.29 standard tree, each open to all seven
// classes, with the Warden, Warlock and Primalist marked legacy.
func TestBloodlines(t *testing.T) {
	t.Parallel()
	store := realdata.Open(t, realdata.Current)
	got, err := simulator.Bloodlines(store, build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 16 {
		t.Errorf("got %d bloodlines, want 16", len(got))
	}
	var legacy []string
	for _, b := range got {
		if len(b.Classes) != 7 {
			t.Errorf("%s: %d classes, want 7", b.ID, len(b.Classes))
		}
		if b.Legacy {
			legacy = append(legacy, b.ID)
		}
	}
	if diff := gocmp.Diff([]string{"Warden", "Warlock", "Primalist"}, legacy); diff != "" {
		t.Errorf("legacy (-want +got):\n%s", diff)
	}
}
