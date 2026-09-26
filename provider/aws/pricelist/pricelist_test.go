package pricelist

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// offline serves offer files from testdata and fails the test on any
// network access.
func offline(t *testing.T) *Client {
	return cached(func(r *http.Request) (*http.Response, error) {
		t.Errorf("network access in a test: %s", r.URL)
		return nil, errNoNetwork
	})
}

// cached serves offer files from testdata and sends every download to get.
func cached(get roundTripFunc) *Client {
	return &Client{HTTP: &http.Client{Transport: get}, CacheDir: "testdata/cache", MaxAge: 100 * 365 * 24 * time.Hour}
}

var errNoNetwork = errors.New("no network in a test")

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveTiers(t *testing.T) {
	c := offline(t)
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
	c := offline(t)
	_, err := c.Resolve(Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": ".*GB-Second.*"}}, "ap-northeast-1")
	if err == nil || !strings.Contains(err.Error(), "2 products match") {
		t.Fatalf("want an ambiguity error, got %v", err)
	}
	_, err = c.Resolve(Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": "Nothing"}}, "ap-northeast-1")
	if err == nil || !strings.Contains(err.Error(), "no product matches") {
		t.Fatalf("want a no-match error, got %v", err)
	}
}

func TestOfferRegionIsFixed(t *testing.T) {
	if got := (Spec{OfferRegion: "aws-other"}).For("ap-northeast-1"); got != "aws-other" {
		t.Fatalf("got %s", got)
	}
	if got := (Spec{}).For("us-east-1"); got != "us-east-1" {
		t.Fatalf("got %s", got)
	}
}

func TestBundledListPrices(t *testing.T) {
	s := Spec{ListPer: 1e6}
	if got := s.PerUnit(2.4); got != 2.4e-6 {
		t.Fatalf("a price per 1M tokens is %v per token, want 2.4e-6", got)
	}
	if got := (Spec{}).PerUnit(0.5); got != 0.5 {
		t.Fatalf("without a bundle the price is unchanged, got %v", got)
	}
}

func TestRegionInFilters(t *testing.T) {
	spec := Spec{Filters: map[string]string{"fromRegionCode": "{region}", "usagetype": "x"}}
	got := spec.filtersFor("ap-northeast-1")
	if got["fromRegionCode"] != `ap-northeast-1` || got["usagetype"] != "x" || spec.Filters["fromRegionCode"] != "{region}" {
		t.Fatalf("got %v, spec %v", got, spec.Filters)
	}
}

// A failed read leaves no offer behind: reading the previous one again
// returns it, not nothing.
func TestFailedReadForgetsTheLastOffer(t *testing.T) {
	c := cached(func(*http.Request) (*http.Response, error) { return nil, errNoNetwork })
	spec := Spec{Service: "AWSLambda", Filters: map[string]string{"usagetype": "APN1-Request"}}
	if _, err := c.Resolve(spec, "ap-northeast-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resolve(spec, "eu-west-1"); !errors.Is(err, errNoNetwork) {
		t.Fatalf("eu-west-1 is not cached: %v", err)
	}
	if m, err := c.Resolve(spec, "ap-northeast-1"); err != nil || m.USD != 0.0000002 {
		t.Fatalf("again: %v %v", m.USD, err)
	}
}
