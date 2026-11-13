package timeless

// rng is the game's generator for alternate passives: TinyMT32 (Saito and Matsumoto) with the
// default parameters below, seeded per node through init_by_array with the key {node id, seed}.
// The game's state transition differs from the reference: it shifts the fourth word left and the
// mixed word right, where the reference does the opposite.
type rng struct {
	s [words]uint32
}

const (
	mat1 = 0x8f7011ee
	mat2 = 0xfc78ff1f
	tmat = 0x3793fdff

	words      = 4
	mid        = 1
	lag        = 1
	minRounds  = 8
	preRounds  = 8
	mix1       = 1664525
	mix2       = 1566083941
	temperBits = 8
)

func newRNG(node, seed uint32) rng {
	g := rng{s: [words]uint32{0, mat1, mat2, tmat}}
	key := [...]uint32{node, seed}

	// init_by_array.
	r := initMix1(g.s[0] ^ g.s[1] ^ g.s[words-1])
	g.s[1] += r
	r += uint32(len(key))
	g.s[2] += r
	g.s[0] = r

	i := uint32(1)
	for _, k := range key {
		g.addRound(i, k+i)
		i = (i + 1) % words
	}
	for range minRounds - 1 - len(key) {
		g.addRound(i, i)
		i = (i + 1) % words
	}
	for range words {
		r = initMix2(g.s[i] + g.s[(i+mid)%words] + g.s[(i+words-1)%words])
		g.s[(i+mid)%words] ^= r
		r -= i
		g.s[(i+mid+lag)%words] ^= r
		g.s[i] = r
		i = (i + 1) % words
	}
	for range preRounds {
		g.next()
	}
	return g
}

func (g *rng) addRound(i, add uint32) {
	r := initMix1(g.s[i] ^ g.s[(i+mid)%words] ^ g.s[(i+words-1)%words])
	g.s[(i+mid)%words] += r
	r += add
	g.s[(i+mid+lag)%words] += r
	g.s[i] = r
}

func initMix1(x uint32) uint32 { return (x ^ x>>27) * mix1 }

func initMix2(x uint32) uint32 { return (x ^ x>>27) * mix2 }

func (g *rng) next() {
	g.s[0], g.s[1], g.s[2], g.s[3] = step(g.s[0], g.s[1], g.s[2], g.s[3])
}

func (g *rng) uint32() uint32 {
	g.next()
	return temper(g.s[0], g.s[2], g.s[3])
}

// step is the state transition on the four words, kept in values so hot loops hold them in
// registers.
func step(s0, s1, s2, s3 uint32) (uint32, uint32, uint32, uint32) {
	a := s3
	b := s0&0x7fffffff ^ s1 ^ s2
	a ^= a << 1
	b ^= b>>1 ^ a
	mask := -(b & 1)
	return s1, s2 ^ mask&mat1, a ^ b<<10 ^ mask&mat2, b
}

func temper(s0, s2, s3 uint32) uint32 {
	t := s0 + s2>>temperBits
	v := s3 ^ t
	if t&1 != 0 {
		v ^= tmat
	}
	return v
}

// pick is how the game chooses one of several weighted rows: one draw per row below the running
// total of weights, keeping the last row whose draw falls under its own weight. It gives that
// row's position, or -1.
func (g *rng) pick(weights []uint32) int {
	s0, s1, s2, s3 := g.s[0], g.s[1], g.s[2], g.s[3]
	picked := -1
	var total uint32
	for i, w := range weights {
		total += w
		if total == 0 {
			continue
		}
		s0, s1, s2, s3 = step(s0, s1, s2, s3)
		if temper(s0, s2, s3)%total < w {
			picked = i
		}
	}
	g.s = [words]uint32{s0, s1, s2, s3}
	return picked
}

// below draws a value in [0, n). n is never 0.
func (g *rng) below(n uint32) uint32 {
	return g.uint32() % n
}

// between draws a value in [lo, hi].
func (g *rng) between(lo, hi int32) int32 {
	return lo + int32(g.below(uint32(hi-lo)+1)) //nolint:gosec // wraps as the game's unsigned draw does
}
