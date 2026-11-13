// Package output names the values a simulation reports. Keys are typed in Go and travel as
// their strings, such as "player.life"; each game's rules define theirs.
package output

// Key names an output.
type Key string

func (k Key) String() string { return string(k) }

// Better says which way an output's value improves a build. It travels as its string; the
// zero value is unset.
type Better string

// The ways an output improves.
const (
	// Higher is better, as for life.
	Higher Better = "higher"
	// Lower is better, as for a cost or a penalty.
	Lower Better = "lower"
	// Neither way is better in itself.
	Neither Better = "neither"
)

// Valid reports whether b is one of Higher, Lower and Neither.
func (b Better) Valid() bool { return b == Higher || b == Lower || b == Neither }
