package poe1

// Engine stats of the character. number_of_allocated_masteries is the allocated Mastery
// passives: the sum of the tree's counters, since every allocatable mastery carries its group's
// count at 1. devotion is what passives conquered by the Templars and items give; modifiers
// scale with it or hold from a threshold.
const (
	statLevel                      = "level"
	statNumberOfAllocatedMasteries = "number_of_allocated_masteries"
	statDevotion                   = "devotion"
)

func characterMechanic() mechanic {
	return mechanic{name: "character", stats: []string{
		statLevel,
		statNumberOfAllocatedMasteries,
		statDevotion,
	}}
}
