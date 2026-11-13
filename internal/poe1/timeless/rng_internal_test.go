package timeless

// The generator is unexported; these tests pin it below Conquer, where a broken draw would only
// show as a different pick.

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The state after init_by_array's first round with a key of two words, from TinyMT32's default
// parameters, is the starting state the public timeless jewel emulators (TimelessEmulator,
// timeless-jewels) seed from: 0x40336050, 0xCFA3723C, 0x3CAC5F6F, 0x3793FDFF.
func TestRNGFirstRound(t *testing.T) {
	t.Parallel()
	s := [4]uint32{0, mat1, mat2, tmat}
	r := initMix1(s[0] ^ s[1] ^ s[3])
	s[1] += r
	r += 2
	s[2] += r
	s[0] = r
	want := [4]uint32{0x40336050, 0xcfa3723c, 0x3cac5f6f, 0x3793fdff}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("state (-want +got):\n%s", diff)
	}
}

// TestRNGSequence pins the first draws for one key, as this generator gives them; the generator
// agrees with Path of Building's timeless jewel lookup tables on every node and seed they hold.
func TestRNGSequence(t *testing.T) {
	t.Parallel()
	g := newRNG(1234, 5678)
	got := []uint32{g.uint32(), g.uint32(), g.uint32()}
	want := []uint32{3321434796, 233575785, 3004654072}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("draws (-want +got):\n%s", diff)
	}
}
