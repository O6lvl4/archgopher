package infer

import (
	"strings"

	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/terraform/eval"
)

// readAttribute reads a field's value from a resource, or nil when it is not
// known before apply.
//
//   - A path ending in ".#" counts what it names across every block
//     ("criteria.dimension.values.#" is every value of every dimension of every
//     criterion); nothing written leaves the field to its default. A list whose
//     elements are known only after apply counts the resources it references.
//   - A boolean that points at a block or at a non-boolean value reads whether
//     it is written, even when the value (an id) is known only after apply.
//   - "ref->rest" reads rest on the resource that the attribute ref
//     references, so an instance group reads the machine type of its
//     instance template and a node pool the location of its cluster. The
//     first referenced resource that has the value wins.
func (b *builder) readAttribute(r *eval.Resource, f field.Field) any {
	return b.readPath(r, f, f.TerraformPath(), 0)
}

// maxReferenceHops bounds a path through references (a per-instance config
// reads its group's template: two hops).
const maxReferenceHops = 4

func (b *builder) readPath(r *eval.Resource, f field.Field, path string, depth int) any {
	if ref, rest, through := strings.Cut(path, field.RefStep); through {
		if depth >= maxReferenceHops {
			return nil
		}
		return b.readFirst(r.Refs[ref], f, rest, depth+1)
	}
	if base, ok := strings.CutSuffix(path, ".#"); ok {
		return countOrNil(max(countPath(r.Attrs, base), len(r.Refs[base])))
	}
	v := lookupPath(r.Attrs, path)
	switch {
	case f.Type == field.Number && v == nil:
		// A number that points at a block reads how many are written
		// (replicas, rules).
		return countOrNil(blocks(r.Attrs, path))
	case f.Type == field.Flag:
		return readFlag(r, path, v)
	}
	return v
}

// readFirst reads path on the first of addrs that has a value.
func (b *builder) readFirst(addrs []string, f field.Field, path string, depth int) any {
	for _, addr := range addrs {
		target, ok := b.byAddr[addr]
		if !ok {
			continue
		}
		if v := b.readPath(target, f, path, depth); v != nil {
			return v
		}
	}
	return nil
}

// readFlag reads a boolean field whose value at path is v: a boolean or its
// text as written, and anything else written (a block, an id, a reference)
// as true.
func readFlag(r *eval.Resource, path string, v any) any {
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		if blocks(r.Attrs, path) > 0 || len(r.Refs[path]) > 0 {
			return true
		}
		return nil
	case string:
		return flagText(x)
	}
	return true
}

// flagText reads "true" and "false" as booleans and any other text as
// written, except empty text: written empty is as good as not written.
func flagText(s string) any {
	switch s {
	case "true", "false":
		return s == "true"
	case "":
		return nil
	}
	return true
}

// countOrNil is a count as a field value; zero leaves the field to its default.
func countOrNil(n int) any {
	if n > 0 {
		return float64(n)
	}
	return nil
}
