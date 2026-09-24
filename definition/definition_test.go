package definition

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
)

const queue = `
type: test_queue
label: Queue
category: Messaging
kinds: [send]
assumptions:
  - { key: messageKb, label: Message size, type: number, default: 1 }
  - { key: fifo, label: FIFO, type: boolean, default: false }
let:
  - chunks: ceilDiv(messageKb, 64)
readings:
  - fail: { when: 'messageKb > 256', message: "messages over 256 KB, got {messageKb}" }
  - requests: { name: Requests, price: 'q.{fifo ? "fifo" : "standard"}', count: total.monthly * 3, chunkKb: 64, sizeKb: messageKb }
  - rate: { when: fifo, name: FIFO rate, unit: messages/second, quota: q.tps, peak: total.peak }
`

func books() book.Books {
	one := 1.0
	v := map[string]book.Value{book.AnyRegion: {Value: &one, Verified: true}}
	return book.Books{
		Prices: book.Book{"q.standard": {Unit: "request", Source: "t", Values: v}, "q.fifo": {Unit: "request", Source: "t", Values: v}},
		Quotas: book.Book{"q.tps": {Unit: "messages/second", Source: "t", Values: v}},
		SLAs:   book.Book{},
	}
}

func load(t *testing.T, src string) *Resource {
	t.Helper()
	u, err := Load(fstest.MapFS{"c/test_queue/resource.yaml": {Data: []byte(src)}}, "c/test_queue")
	if err != nil {
		t.Fatal(err)
	}
	return u.Resource
}

func TestResourceReads(t *testing.T) {
	r := load(t, queue)
	rec := meter.NewRecorder("r", books())
	d := model.Demand{"send": {Monthly: 100, PeakPerSecond: 2}}
	if err := r.Scout(model.Node{Assumptions: map[string]any{"messageKb": 100, "fifo": true}}, d, rec); err != nil {
		t.Fatal(err)
	}
	c := rec.Costs()
	if len(c) != 1 || c[0].PriceID != "q.fifo" || c[0].Quantity != 600 {
		t.Fatalf("costs: %+v", c)
	}
	if l := rec.Limits(); len(l) != 1 || l[0].Demand != 2 {
		t.Fatalf("limits: %+v", l)
	}
	rec = meter.NewRecorder("r", books())
	err := r.Scout(model.Node{Assumptions: map[string]any{"messageKb": 300}}, d, rec)
	if err == nil || !strings.Contains(err.Error(), "messages over 256 KB, got 300") || len(rec.Costs()) != 0 {
		t.Fatalf("a fail reading stops the node: %v %+v", err, rec.Costs())
	}
}

// Mistakes in a definition surface when it loads, not when a user runs it.
func TestDefinitionMistakesFailAtLoad(t *testing.T) {
	cases := map[string]string{
		"a typo in an expression": strings.Replace(queue, "total.monthly * 3", "total.monthy * 3", 1),
		"an unknown reading":      strings.Replace(queue, "  - rate:", "  - rates:", 1),
		"an unknown parameter":    strings.Replace(queue, "chunkKb: 64", "chunkKB: 64", 1),
		"a missing parameter":     strings.Replace(queue, "unit: messages/second, ", "", 1),
		"a type mismatch":         strings.Replace(queue, "total.monthly * 3", `total.monthly * "3"`, 1),
		"a directory mismatch":    strings.Replace(queue, "type: test_queue", "type: other", 1),
		"a bad default":           strings.Replace(queue, "type: number, default: 1", "type: number, default: one", 1),
		"a field named region":    strings.Replace(queue, "key: fifo,", "key: region,", 1),
	}
	for name, src := range cases {
		if _, err := Load(fstest.MapFS{"c/test_queue/resource.yaml": {Data: []byte(src)}}, "c/test_queue"); err == nil {
			t.Errorf("%s: want a load error", name)
		}
	}
}

func TestUnknownAssumptionsAreErrors(t *testing.T) {
	r := load(t, queue)
	err := r.Scout(model.Node{Assumptions: map[string]any{"messageKB": 1}}, model.Demand{}, meter.NewRecorder("r", books()))
	if err == nil || !strings.Contains(err.Error(), `unknown assumption "messageKB"`) {
		t.Fatalf("got %v", err)
	}
}
