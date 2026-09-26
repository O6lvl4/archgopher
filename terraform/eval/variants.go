package eval

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/zclconf/go-cty/cty"
)

// Variant is a group of instances of one counted resource block that are
// configured alike: for_each over plans of different sizes gives one per size.
type Variant struct {
	// Key is the for_each key or count index of its first instance.
	Key string
	// Address is the Terraform address of that instance.
	Address string
	Attrs   map[string]any
	// Instances is how many instances the group has, times its modules'.
	Instances int
}

// eachInstance is the each or count object of one instance, and its key.
type eachInstance struct {
	key   string
	index string // the address suffix: ["key"] or [0]
	extra map[string]cty.Value
}

// allInstances lists the instances of a block when count or for_each is
// known; nil when neither is set or the collection is unknown.
func (ev *evaluator) allInstances(rb *config.ResourceBlock, in *instance) []eachInstance {
	if rb.ForEach != nil {
		return ev.forEachInstances(ev.eval(rb.ForEach, in, nil))
	}
	if rb.Count == nil {
		return nil
	}
	n := ev.instances(rb.Count, nil, in)
	out := make([]eachInstance, 0, max(n, 0))
	for i := range max(n, 0) {
		idx := cty.ObjectVal(map[string]cty.Value{"index": cty.NumberIntVal(int64(i))})
		out = append(out, eachInstance{key: strconv.Itoa(i), index: fmt.Sprintf("[%d]", i), extra: map[string]cty.Value{"count": idx}})
	}
	return out
}

func (ev *evaluator) forEachInstances(v cty.Value) []eachInstance {
	if !v.IsWhollyKnown() || v.IsNull() || !v.CanIterateElements() {
		return nil
	}
	var out []eachInstance
	for it := v.ElementIterator(); it.Next(); {
		k, e := it.Element()
		if v.Type().IsSetType() {
			k = e
		}
		if k.Type() != cty.String {
			return nil
		}
		each := cty.ObjectVal(map[string]cty.Value{"key": k, "value": e})
		out = append(out, eachInstance{key: k.AsString(), index: fmt.Sprintf("[%q]", k.AsString()), extra: map[string]cty.Value{"each": each}})
	}
	return out
}

// variants groups the instances of a counted block by their configuration,
// in the order of their first instance. It returns nil when every instance
// is configured alike, which is one node as before.
func (ev *evaluator) variants(rb *config.ResourceBlock, in *instance, addr string) []Variant {
	insts := ev.allInstances(rb, in)
	if len(insts) < 2 {
		return nil
	}
	var out []Variant
	byConfig := map[string]int{}
	for _, inst := range insts {
		attrs := ev.body(rb.Body, in, inst.extra)
		sig, err := json.Marshal(attrs)
		if err != nil {
			return nil
		}
		if i, ok := byConfig[string(sig)]; ok {
			out[i].Instances += in.copies
			continue
		}
		byConfig[string(sig)] = len(out)
		out = append(out, Variant{Key: inst.key, Address: addr + inst.index, Attrs: attrs, Instances: in.copies})
	}
	if len(out) < 2 {
		return nil
	}
	return out
}
