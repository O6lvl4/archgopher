package infer

import (
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
)

// A link connects two nodes; when the link's own type is a node, it sits in
// between, so a subscription with its own cost carries the topic's messages
// to its push endpoint.
func TestLinkThatIsANodeSitsOnThePath(t *testing.T) {
	ev := &eval.Evaluated{Resources: []*eval.Resource{
		{Type: "topic", Name: "t", Mode: "managed", Address: "topic.t", Instances: 1},
		{Type: "svc", Name: "s", Mode: "managed", Address: "svc.s", Instances: 1},
		{Type: "sub", Name: "x", Mode: "managed", Address: "sub.x", Instances: 1, Refs: map[string][]string{
			"topic":                     {"topic.t"},
			"push_config.push_endpoint": {"svc.s"},
		}},
	}}
	link := Link{Type: "sub", From: "topic", To: []string{"push_config.push_endpoint"}}
	for _, tc := range []struct {
		name string
		node bool
		want string
	}{
		{"a helper", false, "t>s"},
		{"a node", true, "t>x x>s"},
	} {
		rules := Rules{NodeTypes: map[string]bool{"topic": true, "svc": true, "sub": tc.node}, Links: []Link{link}, IgnoreRefs: []string{"topic"}}
		spec, _ := Build(ev, rules, "test")
		var got []string
		for _, e := range spec.Edges {
			got = append(got, e.From+">"+e.To)
		}
		sort.Strings(got)
		if strings.Join(got, " ") != tc.want {
			t.Errorf("%s: edges %v, want %s", tc.name, got, tc.want)
		}
	}
}

// A map key with dots in it (an annotation) is read whole.
func TestLookupPathReadsKeysWithDots(t *testing.T) {
	attrs := map[string]any{"template": []any{map[string]any{"metadata": []any{map[string]any{
		"annotations": map[string]any{"autoscaling.knative.dev/minScale": "2", "run.googleapis.com/cpu-throttling": "false"},
	}}}}}
	for path, want := range map[string]any{
		"template.metadata.annotations.autoscaling.knative.dev/minScale":  "2",
		"template.metadata.annotations.run.googleapis.com/cpu-throttling": "false",
		"template.metadata.annotations.autoscaling.knative.dev/maxScale":  nil,
		"template.metadata.name": nil,
	} {
		if got := lookupPath(attrs, path); got != want {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}
