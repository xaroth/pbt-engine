package jsonshape_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/internal/jsonshape"
)

// An object is closed unless additionalProperties gives a schema, which then checks the
// values of the keys properties does not name.
func TestObjectKeys(t *testing.T) {
	t.Parallel()
	defs := map[string]map[string]any{
		"Closed": {"type": "object", "required": []any{"a"}, "properties": map[string]any{"a": map[string]any{"type": "integer"}}},
		"Map":    {"type": "object", "additionalProperties": map[string]any{"type": "string"}},
	}
	testCases := []struct {
		def, data string
		want      []string
	}{
		{"Closed", `{"a": 1}`, nil},
		{"Closed", `{"a": 1, "b": 2}`, []string{"x: unknown key b"}},
		{"Closed", `{}`, []string{"x: missing a"}},
		{"Map", `{"a": "s", "b": "t"}`, nil},
		{"Map", `{"a": "s", "b": 2}`, []string{"x.b: integer, want string"}},
	}
	for _, testCase := range testCases {
		var v any
		if err := json.Unmarshal([]byte(testCase.data), &v); err != nil {
			t.Fatal(err)
		}
		got := jsonshape.ShapeErrors(defs, map[string]any{"$ref": "#/$defs/" + testCase.def}, v, "x")
		if diff := cmp.Diff(testCase.want, got); diff != "" {
			t.Errorf("%s %s (-want +got):\n%s", testCase.def, testCase.data, diff)
		}
	}
}
