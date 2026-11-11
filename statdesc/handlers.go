package statdesc

// handler transforms a stat value for display. invert undoes apply up to its rounding, so
// reading text back finds the value; a zero handler changes nothing.
type handler struct {
	apply  func(number) number
	invert func(float64) float64
}

// handlers maps handler names to value transforms. Zero entries mark handlers that only affect
// reminder text or tooling.
//
//nolint:mnd // each handler name states its factor, so a named constant would only repeat it
func handlers() map[string]handler {
	return map[string]handler{
		"canonical_line": {},
		"canonical_stat": {},
		"reminderstring": {},

		"negate":                         scale(-1, 0),
		"negate_and_double":              scale(-2, 0),
		"double":                         scale(2, 0),
		"times_twenty":                   scale(20, 0),
		"times_one_point_five":           scale(1.5, 0),
		"30%_of_value":                   scale(0.3, 0),
		"60%_of_value":                   scale(0.6, 0),
		"plus_two_hundred":               scale(1, 200),
		"multiplicative_damage_modifier": scale(1, 100),

		"divide_by_three":                       scale(1.0/3, 0),
		"divide_by_four":                        scale(1.0/4, 0),
		"divide_by_five":                        scale(1.0/5, 0),
		"divide_by_six":                         scale(1.0/6, 0),
		"divide_by_twelve":                      scale(1.0/12, 0),
		"divide_by_twenty":                      scale(1.0/20, 0),
		"divide_by_one_hundred":                 scale(1.0/100, 0),
		"divide_by_one_thousand":                scale(1.0/1000, 0),
		"divide_by_two_0dp":                     decimals(scale(1.0/2, 0), 0, false),
		"divide_by_ten_0dp":                     decimals(scale(1.0/10, 0), 0, false),
		"divide_by_fifteen_0dp":                 decimals(scale(1.0/15, 0), 0, false),
		"divide_by_ten_1dp":                     decimals(scale(1.0/10, 0), 1, true),
		"divide_by_ten_1dp_if_required":         decimals(scale(1.0/10, 0), 1, false),
		"divide_by_one_hundred_2dp":             decimals(scale(1.0/100, 0), 2, true),
		"divide_by_one_hundred_2dp_if_required": decimals(scale(1.0/100, 0), 2, false),
		"divide_by_one_hundred_and_negate":      scale(-1.0/100, 0),
		"divide_by_twenty_then_double_0dp": {
			apply: func(n number) number {
				n = decimals(scale(1.0/20, 0), 0, false).apply(n)
				n.v = roundTo(n.v, 0) * 2
				return n
			},
			invert: func(v float64) float64 { return v * 10 },
		},

		"per_minute_to_per_second":                 decimals(scale(1.0/60, 0), 1, false),
		"per_minute_to_per_second_0dp":             decimals(scale(1.0/60, 0), 0, false),
		"per_minute_to_per_second_1dp":             decimals(scale(1.0/60, 0), 1, true),
		"per_minute_to_per_second_2dp":             decimals(scale(1.0/60, 0), 2, true),
		"per_minute_to_per_second_2dp_if_required": decimals(scale(1.0/60, 0), 2, false),
		"permyriad_per_minute_to_%_per_second":     decimals(scale(1.0/6000, 0), 1, false),

		"milliseconds_to_seconds":                 scale(1.0/1000, 0),
		"milliseconds_to_seconds_0dp":             decimals(scale(1.0/1000, 0), 0, false),
		"milliseconds_to_seconds_1dp":             decimals(scale(1.0/1000, 0), 1, true),
		"milliseconds_to_seconds_2dp":             decimals(scale(1.0/1000, 0), 2, true),
		"milliseconds_to_seconds_2dp_if_required": decimals(scale(1.0/1000, 0), 2, false),
		"deciseconds_to_seconds":                  scale(1.0/10, 0),
		"locations_to_metres":                     scale(1.0/10, 0),
	}
}

func scale(mul, add float64) handler {
	return handler{
		apply: func(n number) number {
			n.v = n.v*mul + add
			return n
		},
		invert: func(v float64) float64 { return (v - add) / mul },
	}
}

// decimals rounds h's result to dp decimals for display, keeping trailing zeros if fixed.
func decimals(h handler, dp int, fixed bool) handler {
	return handler{
		apply: func(n number) number {
			n = h.apply(n)
			n.dp, n.fixed, n.set = dp, fixed, true
			return n
		},
		invert: h.invert,
	}
}

func roundTo(v float64, dp int) float64 {
	return number{v: v, dp: dp, set: true}.rounded()
}
