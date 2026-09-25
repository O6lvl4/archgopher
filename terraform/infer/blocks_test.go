package infer

import "testing"

func TestCountBlocks(t *testing.T) {
	attrs := map[string]any{
		"subnet_mapping": []any{map[string]any{"subnet_id": "a"}, map[string]any{"subnet_id": "b"}},
		"subnet_ids":     []any{"a", "b", "c"},
		"outer":          []any{map[string]any{"inner": []any{map[string]any{}, map[string]any{}, map[string]any{}}}},
		"empty":          []any{},
	}
	for path, want := range map[string]int{"subnet_mapping": 2, "outer.inner": 3} {
		if n, ok := countBlocks(attrs, path); !ok || n != want {
			t.Errorf("%s: %d %v, want %d", path, n, ok, want)
		}
	}
	for _, path := range []string{"subnet_ids", "empty", "missing", "outer.missing"} {
		if n, ok := countBlocks(attrs, path); ok {
			t.Errorf("%s: counted %d, want nothing", path, n)
		}
	}
}
