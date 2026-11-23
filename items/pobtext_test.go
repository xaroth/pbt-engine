package items_test

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/items"
)

// TestPoBTextRoundTrip reads item text, writes it with PoBText and reads that back to the same
// item. The two Diamond Rings differ in their implicit's range: 20% to 30% on Ring11, 30% to 40%
// on RingRoyale3. An Unstable catalyst at 20 quality scales Ring11's to 24% to 36%.
func TestPoBTextRoundTrip(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	ring := "Rarity: RARE\nX\nDiamond Ring\n%sImplicits: 1\n%d%% increased Global Critical Strike Chance\n+59 to maximum Mana\n"
	const ring11, royale = "Metadata/Items/Rings/Ring11", "Metadata/Items/Rings/RingRoyale3"
	for _, c := range []struct {
		name, text, base, catalyst string
		quality                    int
	}{
		{"Diamond Ring", fmt.Sprintf(ring, "", 25), ring11, "", 0},
		{"the other Diamond Ring", fmt.Sprintf(ring, "", 38), royale, "", 0},
		{"catalysed", fmt.Sprintf(ring, "Catalyst: Unstable\nCatalystQuality: 20\n", 36), ring11, "Unstable", 20},
		{"catalyst at 20 when the quality is not given", fmt.Sprintf(ring, "Catalyst: Unstable\n", 36), ring11, "Unstable", 20},
		{"catalyst at 0 quality", fmt.Sprintf(ring, "Catalyst: Unstable\nCatalystQuality: 0\n", 25), ring11, "Unstable", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			it, _, err := r.Parse(c.text)
			if err != nil {
				t.Fatal(err)
			}
			if it.Base != c.base || it.Catalyst != c.catalyst || it.CatalystQuality != c.quality {
				t.Fatalf("base %s, catalyst %q at %d; want %s, %q at %d", it.Base, it.Catalyst, it.CatalystQuality, c.base, c.catalyst, c.quality)
			}
			back, _, err := r.Parse(items.PoBText(it, "Diamond Ring"))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(it, back); diff != "" {
				t.Errorf("read back (-want +got):\n%s", diff)
			}
		})
	}
}
