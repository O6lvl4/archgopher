package infer

import (
	"fmt"
	"strings"
)

// countPath counts the elements "a.b" holds, fanning out over every block
// instance on the way: list elements and blocks count one each, a single
// value counts one, nothing written counts zero.
func countPath(m map[string]any, path string) int {
	cur := []any{m}
	for _, part := range strings.Split(path, ".") {
		cur = fanOut(cur, part)
	}
	n := 0
	for _, c := range cur {
		if list, ok := c.([]any); ok {
			n += len(list)
		} else {
			n++
		}
	}
	return n
}

// fanOut steps into part of every block in cur, where each element is a
// block or a list of block instances; unset values drop out.
func fanOut(cur []any, part string) []any {
	var next []any
	for _, c := range cur {
		list, ok := c.([]any)
		if !ok {
			list = []any{c}
		}
		for _, e := range list {
			if obj, ok := e.(map[string]any); ok && obj[part] != nil {
				next = append(next, obj[part])
			}
		}
	}
	return next
}

// lookupPath reads "a.b" from nested maps, taking the first element of block lists.
// A map key may itself hold dots (annotations such as
// "autoscaling.knative.dev/minScale"): when a part is not a key, the shortest
// run of the following parts that is one is taken. A last step "#" counts
// the blocks or list elements written ("scratch_disk.#"), and a step "*" reads
// the rest of the path in every block, one text per block in order and ""
// where a block leaves it unset ("disk.*.disk_size_gb"). A part written
// "block[key=value]" takes the first block whose key is value:
// "setting[name=InstanceType].value" reads one option out of a list of
// name/value blocks.
func lookupPath(m map[string]any, path string) any {
	var cur any = m
	parts := strings.Split(path, ".")
	for i := 0; i < len(parts); {
		switch parts[i] {
		case "#":
			return listLen(cur)
		case "*":
			return eachBlock(cur, strings.Join(parts[i+1:], "."))
		}
		obj, ok := firstBlock(cur)
		if !ok {
			return nil
		}
		if cur, i, ok = stepInto(obj, parts, i); !ok {
			return nil
		}
	}
	return leafValue(cur)
}

// listLen counts the elements of a list, or is nil for anything else.
func listLen(v any) any {
	if list, ok := v.([]any); ok {
		return float64(len(list))
	}
	return nil
}

// firstBlock is v as a block, taking the first instance of a block list.
func firstBlock(v any) (map[string]any, bool) {
	if list, ok := v.([]any); ok {
		if len(list) == 0 {
			return nil, false
		}
		v = list[0]
	}
	obj, ok := v.(map[string]any)
	return obj, ok
}

// stepInto reads parts[i] from obj, as a selector or as the shortest run of
// parts that is a key, and returns the value and the index of the next part.
func stepInto(obj map[string]any, parts []string, i int) (any, int, bool) {
	if name, key, want, selects := selector(parts[i]); selects {
		return pick(obj[name], key, want), i + 1, true
	}
	for j := i + 1; j <= len(parts); j++ {
		if v, ok := obj[strings.Join(parts[i:j], ".")]; ok {
			return v, j, true
		}
	}
	return nil, i, false
}

// leafValue is the value a path ends at; a list of blocks is not a value.
func leafValue(v any) any {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v
	}
	if _, isBlock := list[0].(map[string]any); isBlock {
		return nil
	}
	return v
}

// eachBlock reads rest in every block of list, as text so the values of
// several paths line up block by block.
func eachBlock(list any, rest string) any {
	blocks, ok := list.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(blocks))
	for _, blk := range blocks {
		var v any
		if obj, ok := blk.(map[string]any); ok {
			v = obj
			if rest != "" {
				v = lookupPath(obj, rest)
			}
		}
		switch v.(type) {
		case string, bool, int, int64, float64:
			out = append(out, fmt.Sprint(v))
		default:
			out = append(out, "")
		}
	}
	return out
}

// selector splits "block[key=value]" into its parts.
func selector(part string) (name, key, value string, ok bool) {
	open := strings.IndexByte(part, '[')
	if open < 0 || !strings.HasSuffix(part, "]") {
		return part, "", "", false
	}
	key, value, ok = strings.Cut(part[open+1:len(part)-1], "=")
	return part[:open], key, value, ok
}

// pick returns the first block of a list whose key holds value, or nil.
func pick(v any, key, value string) any {
	list, _ := v.([]any)
	for _, el := range list {
		if b, ok := el.(map[string]any); ok && fmt.Sprint(b[key]) == value {
			return b
		}
	}
	return nil
}

// blocks counts the blocks written at "a.b", empty ones included; the parent
// path is walked through the first instance of each block.
func blocks(m map[string]any, path string) int {
	parts := strings.Split(path, ".")
	parent, last := m, parts[len(parts)-1]
	if len(parts) > 1 {
		p, ok := lookupBlock(m, strings.Join(parts[:len(parts)-1], "."))
		if !ok {
			return 0
		}
		parent = p
	}
	switch v := parent[last].(type) {
	case []any:
		n := 0
		for _, b := range v {
			if _, ok := b.(map[string]any); ok {
				n++
			}
		}
		return n
	case map[string]any:
		return 1
	}
	return 0
}

// lookupBlock walks "a.b" through blocks and returns the first instance.
func lookupBlock(m map[string]any, path string) (map[string]any, bool) {
	cur := m
	for _, part := range strings.Split(path, ".") {
		switch v := cur[part].(type) {
		case []any:
			if len(v) == 0 {
				return nil, false
			}
			next, ok := v[0].(map[string]any)
			if !ok {
				return nil, false
			}
			cur = next
		case map[string]any:
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}
