// Package output names the values a simulation reports. Keys are typed in Go and travel as
// their strings, such as "player.life"; each game's rules define theirs.
package output

// Key names an output.
type Key string

func (k Key) String() string { return string(k) }
