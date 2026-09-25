package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
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
	offline := &pricelist.Client{
		HTTP:     &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { panic("network access in a test") })},
		CacheDir: "testdata/cache",
		MaxAge:   100 * 365 * 24 * time.Hour,
	}
	s := syncer{sources: sources{aws: offline}, today: "2026-01-02"}
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
		if (w == nil) != (v.Value == nil) || (w != nil && *v.Value != *w) || !v.Verified || v.CheckedAt != "2026-01-02" {
			t.Errorf("%s: %+v, want %v\n%s", row, v, w, data)
		}
	}
}

func f(v float64) *float64 { return &v }
