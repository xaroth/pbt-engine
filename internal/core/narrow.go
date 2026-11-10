package core

import "math"

// u32, u16 and i32 narrow a length or index to the width of an id. Exceeding it would wrap an
// id onto an existing one, so they panic instead.
func u32(n int) uint32 {
	if n < 0 || n > math.MaxUint32 {
		panic("core: value out of uint32 range")
	}
	return uint32(n)
}

func u16(n int) uint16 {
	if n < 0 || n > math.MaxUint16 {
		panic("core: value out of uint16 range")
	}
	return uint16(n)
}

func i32(n int) int32 {
	if n < math.MinInt32 || n > math.MaxInt32 {
		panic("core: value out of int32 range")
	}
	return int32(n)
}
