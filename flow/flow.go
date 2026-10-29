// Package flow reads what the cost and headroom readings leave out of a
// queue-driven design: how fast each queue is drained against how fast work
// arrives, the backlog and wait a scenario's burst leaves behind, and how many
// connections the consumers can open at once on what they call. It knows no
// provider: the provider says which types are queues, consumers and
// connection-limited stores.
package flow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/model"
)

// Config is the provider's share.
type Config struct {
	Queues map[string]bool
	// Consumers maps a type to where its concurrency cap and duration are.
	Consumers map[string]Consumer
	// Limited are types with a connection limit (databases).
	Limited map[string]bool
}

// Consumer says where a consumer type keeps its cap, in the order to try
// (attributes; a negative value means none), and its duration assumption.
type Consumer struct {
	Caps     []string
	Duration string
}

// Queue is one queue's flow.
type Queue struct {
	Node string `json:"node"`
	// Arrival is the peak rate of messages in, per second.
	Arrival   float64    `json:"arrival"`
	Consumers []Draining `json:"consumers"`
	// Capacity is how many messages per second the consumers drain at most;
	// 0 when a consumer has no cap or no duration.
	Capacity    float64 `json:"capacity"`
	Utilization float64 `json:"utilization,omitempty"`
	// Backlog and WaitMinutes: what a scenario of Minutes above capacity leaves.
	Minutes     float64 `json:"minutes,omitempty"`
	Backlog     float64 `json:"backlog,omitempty"`
	WaitMinutes float64 `json:"waitMinutes,omitempty"`
	Unknown     string  `json:"unknown,omitempty"`
}

// Draining is one consumer of a queue.
type Draining struct {
	Node       string  `json:"node"`
	Cap        int     `json:"cap,omitempty"`
	DurationMs float64 `json:"durationMs,omitempty"`
}

// Store is one connection-limited node and the most connections its callers
// can hold at once.
type Store struct {
	Node    string   `json:"node"`
	Type    string   `json:"type"`
	Callers []string `json:"callers"`
	// Connections is the sum of the callers' caps; Uncapped lists callers
	// without one, whose share is bounded only by the account.
	Connections int      `json:"connections"`
	Uncapped    []string `json:"uncapped,omitempty"`
}

// Result is the flow of one scenario.
type Result struct {
	Scenario string  `json:"scenario"`
	Queues   []Queue `json:"queues"`
	Stores   []Store `json:"stores"`
}

// Read reads the flow of a spec (patterns expanded) and its engine result.
// minutes is how long the peak lasts, 0 for a steady state.
func Read(spec model.Spec, res engine.Result, minutes float64, c Config, scenario string) Result {
	nodes := map[string]model.Node{}
	for _, n := range spec.Nodes {
		nodes[n.ID] = n
	}
	demand := map[string]model.Load{}
	for _, n := range res.Nodes {
		demand[n.ID] = n.Demand.Total()
	}
	out := Result{Scenario: scenario}
	for _, n := range spec.Nodes {
		if !c.Queues[n.Type] {
			continue
		}
		q := Queue{Node: n.ID, Arrival: demand[n.ID].PeakPerSecond, Minutes: minutes}
		known := true
		for _, e := range spec.Edges {
			to := nodes[e.To]
			cc, ok := c.Consumers[to.Type]
			if e.From != n.ID || !ok {
				continue
			}
			d := Draining{Node: to.ID, Cap: capOf(to, cc), DurationMs: number(to.Assumptions[cc.Duration])}
			q.Consumers = append(q.Consumers, d)
			switch {
			case d.Cap == 0:
				known, q.Unknown = false, to.ID+" has no concurrency cap"
			case d.DurationMs <= 0:
				known, q.Unknown = false, to.ID+" has no duration"
			default:
				q.Capacity += float64(d.Cap) / (d.DurationMs / 1000)
			}
		}
		if len(q.Consumers) == 0 {
			q.Unknown = "nothing consumes it"
			known = false
		}
		if !known {
			q.Capacity = 0
		} else if q.Capacity > 0 {
			q.Utilization = q.Arrival / q.Capacity
			if minutes > 0 && q.Arrival > q.Capacity {
				q.Backlog = (q.Arrival - q.Capacity) * minutes * 60
				q.WaitMinutes = q.Backlog / q.Capacity / 60
			}
		}
		out.Queues = append(out.Queues, q)
	}
	for _, n := range spec.Nodes {
		if !c.Limited[n.Type] {
			continue
		}
		st := Store{Node: n.ID, Type: n.Type}
		for _, e := range spec.Edges {
			from := nodes[e.From]
			cc, ok := c.Consumers[from.Type]
			if e.To != n.ID || !ok {
				continue
			}
			st.Callers = append(st.Callers, from.ID)
			if cap := capOf(from, cc); cap > 0 {
				st.Connections += cap * from.Count()
			} else {
				st.Uncapped = append(st.Uncapped, from.ID)
			}
		}
		if len(st.Callers) > 0 {
			sort.Strings(st.Callers)
			out.Stores = append(out.Stores, st)
		}
	}
	return out
}

func capOf(n model.Node, c Consumer) int {
	for _, a := range c.Caps {
		if v := number(n.Attributes[a]); v > 0 {
			return int(v)
		}
	}
	return 0
}

func number(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// Text renders results, one per scenario.
func Text(rs []Result) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "## %s\n\n", r.Scenario)
		if len(r.Queues) > 0 {
			fmt.Fprintln(&b, "| Queue | Arrival /s | Drain /s | Utilization | Backlog | Wait |")
			fmt.Fprintln(&b, "| --- | ---: | ---: | ---: | ---: | ---: |")
			for _, q := range r.Queues {
				if q.Unknown != "" {
					fmt.Fprintf(&b, "| %s | %.2f | - | - | - | unknown: %s |\n", q.Node, q.Arrival, q.Unknown)
					continue
				}
				wait := "-"
				if q.Minutes > 0 {
					wait = fmt.Sprintf("%.0f min", q.WaitMinutes)
				}
				fmt.Fprintf(&b, "| %s | %.2f | %.2f | %.0f%% | %.0f | %s |\n", q.Node, q.Arrival, q.Capacity, 100*q.Utilization, q.Backlog, wait)
			}
			fmt.Fprintln(&b)
		}
		if len(r.Stores) > 0 {
			fmt.Fprintln(&b, "| Store | Callers | Most connections at once |")
			fmt.Fprintln(&b, "| --- | --- | ---: |")
			for _, s := range r.Stores {
				most := fmt.Sprint(s.Connections)
				if len(s.Uncapped) > 0 {
					most += " + uncapped " + strings.Join(s.Uncapped, ", ")
				}
				fmt.Fprintf(&b, "| %s | %s | %s |\n", s.Node, strings.Join(s.Callers, ", "), most)
			}
			fmt.Fprintln(&b)
		}
	}
	return b.String()
}
