package infer

import "testing"

// blocks counts written blocks; a list of values, an empty list and a missing
// path are none.
func TestBlocks(t *testing.T) {
	attrs := map[string]any{
		"subnet_mapping": []any{map[string]any{"subnet_id": "a"}, map[string]any{"subnet_id": "b"}},
		"subnet_ids":     []any{"a", "b", "c"},
		"outer":          []any{map[string]any{"inner": []any{map[string]any{}, map[string]any{}, map[string]any{}}}},
		"empty":          []any{},
	}
	for path, want := range map[string]int{"subnet_mapping": 2, "outer.inner": 3} {
		if n := blocks(attrs, path); n != want {
			t.Errorf("%s: %d, want %d", path, n, want)
		}
	}
	for _, path := range []string{"subnet_ids", "empty", "missing", "outer.missing"} {
		if n := blocks(attrs, path); n != 0 {
			t.Errorf("%s: counted %d, want nothing", path, n)
		}
	}
}
