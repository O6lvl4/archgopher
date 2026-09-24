package pricelist

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// offline serves offer files from testdata and fails on any network access.
func offline() *Client {
	return &Client{
		HTTP:     &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { panic("network access in a test") })},
		CacheDir: "testdata/cache",
		MaxAge:   100 * 365 * 24 * time.Hour,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveTiers(t *testing.T) {
	c := offline()
	spec := Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": "([A-Z]+[0-9]-)?Lambda-GB-Second"}}
	first, err := c.Resolve(spec, "ap-northeast-1")
	if err != nil || first.USD != 0.0000166667 {
		t.Fatalf("first tier: %v %v", first.USD, err)
	}
	spec.Tier = "last"
	last, err := c.Resolve(spec, "ap-northeast-1")
	if err != nil || last.USD != 0.000015 {
		t.Fatalf("last tier: %v %v", last.USD, err)
	}
	spec.Tier = "6000000000"
	if m, err := c.Resolve(spec, "ap-northeast-1"); err != nil || m.USD != 0.000015 {
		t.Fatalf("tier by beginRange: %v %v", m.USD, err)
	}
}

func TestResolveNeedsExactlyOneProduct(t *testing.T) {
	c := offline()
	_, err := c.Resolve(Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": ".*GB-Second.*"}}, "ap-northeast-1")
	if err == nil || !strings.Contains(err.Error(), "2 products match") {
		t.Fatalf("want an ambiguity error, got %v", err)
	}
	_, err = c.Resolve(Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": "Nothing"}}, "ap-northeast-1")
	if err == nil || !strings.Contains(err.Error(), "no product matches") {
		t.Fatalf("want a no-match error, got %v", err)
	}
}

func TestRegionFiltersOverride(t *testing.T) {
	spec := Spec{
		Filters:       map[string]string{"group": "g", "usagetype": "base"},
		RegionFilters: map[string]map[string]string{"ap-northeast-1": {"usagetype": "JP-x"}},
		OfferRegion:   "aws-other",
	}
	f, offer := spec.For("ap-northeast-1")
	if f["usagetype"] != "JP-x" || f["group"] != "g" || offer != "aws-other" {
		t.Fatalf("got %v %s", f, offer)
	}
	if f, _ := spec.For("us-east-1"); f["usagetype"] != "base" {
		t.Fatalf("got %v", f)
	}
}
