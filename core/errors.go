package core

import "errors"

// Errors callers can tell apart with errors.Is. Returned errors wrap them with detail.
var (
	// ErrBadHandle is an id that names no actor, stat, context, source or modifier of the
	// model or session it was used with.
	ErrBadHandle = errors.New("core: bad handle")
	// ErrDuplicateName is a stat, dimension or dimension value registered twice, or a
	// dimension value whose "dimension.value" name another dimension already has.
	ErrDuplicateName = errors.New("core: duplicate name")
	// ErrOutOfRange is a parameter outside its allowed range, such as an unknown Op, a scale
	// step that is not positive or more context bits than MaxBits.
	ErrOutOfRange = errors.New("core: out of range")
)
