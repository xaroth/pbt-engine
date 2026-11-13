package jsonshape

// ShapeErrors lets black-box tests check JSON against definitions given inline.
func ShapeErrors(defs map[string]map[string]any, sch map[string]any, v any, path string) []string {
	return shapeErrors(defs, sch, v, path)
}
