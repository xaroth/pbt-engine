package build_test

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
)

// A build with jewels encodes them under "jewels" and parses back; one without leaves the
// field out.
func TestJewelsRoundTrip(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Nodes = []uint32{1, 3, 10}
	if err := b.Apply(build.SocketJewel(70, jewel(jewelBase))); err != nil {
		t.Fatal(err)
	}
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(b, got); diff != "" {
		t.Errorf("Parse(Encode) (-want +got):\n%s\n%s", diff, data)
	}
	if err := b.Apply(build.UnsocketJewel(70)); err != nil {
		t.Fatal(err)
	}
	if data, _ := b.Encode(); bytes.Contains(data, []byte(`"jewels"`)) {
		t.Errorf("empty jewels encoded:\n%s", data)
	}
}

func randJewels(r *rand.Rand) build.Jewels {
	var out build.Jewels
	for range r.IntN(3) {
		if out == nil {
			out = build.Jewels{}
		}
		it := jewel(jewelBase)
		it.Mods[0].Stats[0].Value = int32(r.IntN(3))
		out[uint32(70+r.IntN(3))] = it
	}
	return out
}
