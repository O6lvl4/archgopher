package aws

import (
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// eventSourceCap reads how many concurrent executions the SQS event sources
// of a function may start: the sum of their maximum_concurrency, counting
// each mapping's instances. A mapping without a cap leaves the function
// limited only by its own concurrency, so nothing is set.
func eventSourceCap(g *infer.Graph, r *eval.Resource) (map[string]any, error) {
	total, found := 0, false
	for _, m := range g.Resources() {
		if m.Type != "aws_lambda_event_source_mapping" || !refers(m, "function_name", r.Address) || !fromQueue(g, m) {
			continue
		}
		cap, ok := maximumConcurrency(m.Attrs)
		if !ok {
			return nil, nil
		}
		total += cap * max(1, m.Instances)
		found = true
	}
	if !found {
		return nil, nil
	}
	return map[string]any{"event_source_max_concurrency": total}, nil
}

func refers(r *eval.Resource, path, addr string) bool {
	for _, a := range r.Refs[path] {
		if a == addr {
			return true
		}
	}
	return false
}

func fromQueue(g *infer.Graph, m *eval.Resource) bool {
	for _, a := range m.Refs["event_source_arn"] {
		if src, ok := g.Resource(a); ok && src.Type == "aws_sqs_queue" {
			return true
		}
	}
	return false
}

func maximumConcurrency(attrs map[string]any) (int, bool) {
	blocks, _ := attrs["scaling_config"].([]any)
	if len(blocks) == 0 {
		return 0, false
	}
	b, _ := blocks[0].(map[string]any)
	switch n := b["maximum_concurrency"].(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}
