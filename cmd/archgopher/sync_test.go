package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
	"github.com/O6lvl4/archgopher/provider/gcp/billingcatalog"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A table is synced row by row: the row key fills the filter, quoted, so
// "t3.micro" never matches "t3xmicro"; a wrong value is corrected, a missing
// one filled and a row the price list lacks and nobody priced recorded as not offered.
func TestSyncTableRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	in := `{"aws.ec2.linux": {"unit": "hour", "source": "test",
	  "sync": {"service": "AmazonEC2", "filters": {"instanceType": "{row}", "operatingSystem": "Linux"}},
	  "rows": {"t3.micro": {"ap-northeast-1": 0.01}, "t3.small": {"ap-northeast-1": null}, "x9.huge": {"ap-northeast-1": null}}}}`
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	s := syncer{sources: sources{aws: offlinePriceList(t)}, today: "2026-01-02"}
	if err := s.file(&bytes.Buffer{}, path, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := book.Load(os.DirFS(filepath.Dir(path)), ".")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]*float64{"t3.micro": f(0.0136), "t3.small": f(0.0272), "x9.huge": nil}
	for row, w := range want {
		_, v, err := b.Prices.Lookup("aws.ec2.linux."+row, "ap-northeast-1")
		if err != nil {
			t.Fatal(err)
		}
		if !equal(w, v.Value) || !v.Verified || v.CheckedAt != "2026-01-02" {
			t.Errorf("%s: %+v, want %v\n%s", row, v, w, data)
		}
	}
}

// offlinePriceList reads the Price List from the test cache only; reaching
// the network fails the test.
func offlinePriceList(t *testing.T) *pricelist.Client {
	return &pricelist.Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			t.Errorf("network access in a test: %s", r.URL)
			return nil, errors.New("network access in a test")
		})},
		CacheDir: "testdata/cache",
		MaxAge:   100 * 365 * 24 * time.Hour,
	}
}

func f(v float64) *float64 { return &v }

// equal reports whether two optional prices are exactly equal.
func equal(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type fixed map[string]float64

func (f fixed) quote(region string) (quote, error) {
	v, ok := f[region]
	if !ok {
		return quote{}, fmt.Errorf("%w: none in %s", billingcatalog.ErrAbsent, region)
	}
	return quote{usdPerUnit: v}, nil
}

func (f fixed) global() bool { return false }

// A composed row is the sum of its parts, each times its count; outside the
// regions it is offered in, or where a part is not sold, it is not offered.
func TestCompositeSumsItsParts(t *testing.T) {
	c := composite{in: map[string]bool{"r1": true, "r2": true}, parts: []weighted{
		{src: fixed{"r1": 0.03, "r2": 0.04, "r3": 0.05}, times: 8, name: "core"},
		{src: fixed{"r1": 0.004, "r3": 0.005}, times: 32, name: "ram"},
	}}
	q, err := c.quote("r1")
	if err != nil || math.Abs(q.usdPerUnit-(8*0.03+32*0.004)) > 1e-12 {
		t.Fatalf("r1: %v %v", q, err)
	}
	if _, err := c.quote("r2"); !absent(err) {
		t.Errorf("r2 lacks the ram part: %v", err)
	}
	if _, err := c.quote("r3"); !absent(err) {
		t.Errorf("r3 is outside the regions the row is offered in: %v", err)
	}
}

// A part fills {part} with its name quoted, or its pattern as written, and
// lays its own keys over the spec.
func TestPartSpec(t *testing.T) {
	raw := `{"source":"gcp","service":"s","filters":{"description":"{part} running in .+"}}`
	got, err := partSpec(raw, book.Part{Name: "N2 Instance Core (x)"}, nil)
	if err != nil || !strings.Contains(string(got), `N2 Instance Core \\(x\\) running in`) {
		t.Errorf("name: %s %v", got, err)
	}
	got, err = partSpec(raw, book.Part{Name: "ext", Match: "N2D AMD Custom Extended( Instance)? Ram"}, json.RawMessage(`{"region":"global"}`))
	if err != nil || !strings.Contains(string(got), `Extended( Instance)? Ram running`) || !strings.Contains(string(got), `"region":"global"`) {
		t.Errorf("match and with: %s %v", got, err)
	}
}

// A part with a source of its own in one region reads it there only.
func TestCompositeRegionalPart(t *testing.T) {
	c := composite{parts: []weighted{{src: fixed{"r1": 1, "r2": 1}, times: 2, name: "core", in: map[string]priceSource{"r2": fixed{"r2": 3}}}}}
	if q, _ := c.quote("r1"); q.usdPerUnit != 2 {
		t.Errorf("r1: %v", q.usdPerUnit)
	}
	if q, _ := c.quote("r2"); q.usdPerUnit != 6 {
		t.Errorf("r2: %v", q.usdPerUnit)
	}
}

func TestTierRowIsANumberNotAPattern(t *testing.T) {
	raw := `{"filters": {"sku": "{row}"}, "tier": "{row}"}`
	got := tierRow.ReplaceAllLiteralString(raw, `"tier": `+strconv.Quote("0.5"))
	if want := `{"filters": {"sku": "{row}"}, "tier": "0.5"}`; got != want {
		t.Fatalf("got %s", got)
	}
}
