// Package pricelist reads the public AWS Price List bulk files. They need no
// credentials, so prices can be verified by anyone, including CI.
package pricelist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BaseURL is the public Price List endpoint.
const BaseURL = "https://pricing.us-east-1.amazonaws.com"

// Offer is the part of an offer file that pricing needs.
type Offer struct {
	Products map[string]Product `json:"products"`
	Terms    struct {
		OnDemand map[string]map[string]Term `json:"OnDemand"`
	} `json:"terms"`
}

// Product is one SKU.
type Product struct {
	SKU           string            `json:"sku"`
	ProductFamily string            `json:"productFamily"`
	Attributes    map[string]string `json:"attributes"`
}

// Term holds the price dimensions of an on-demand offer term.
type Term struct {
	PriceDimensions map[string]Dimension `json:"priceDimensions"`
}

// Dimension is one price, possibly one tier of several.
type Dimension struct {
	Unit         string            `json:"unit"`
	Description  string            `json:"description"`
	BeginRange   string            `json:"beginRange"`
	EndRange     string            `json:"endRange"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

// Spec says how to find one price. Filters are regular expressions that must
// match the whole attribute value; productFamily is matched like an attribute.
type Spec struct {
	Service string            `json:"service"`
	Filters map[string]string `json:"filters"`
	// Tier picks a dimension when there are several: "first" (default), "last",
	// or the beginRange of the tier as a number.
	Tier string `json:"tier,omitempty"`
	// OfferRegion reads another section of the service's files (CloudFront's
	// edge prices live in "aws-other" whatever the resource region).
	OfferRegion string `json:"offerRegion,omitempty"`
	// RegionFilters override Filters per book region (US- or JP- edge prefixes).
	RegionFilters map[string]map[string]string `json:"regionFilters,omitempty"`
	// ListPer is how many units the Price List price covers when its unit is a
	// bundle ("1M Input Tokens" is 1e6). 0 means one unit.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts a Price List price to the price of one unit.
func (s Spec) PerUnit(usd float64) float64 {
	if s.ListPer == 0 {
		return usd
	}
	return usd / s.ListPer
}

// For returns the filters and offer region to use for one book region.
func (s Spec) For(region string) (map[string]string, string) {
	f := map[string]string{}
	for k, v := range s.Filters {
		f[k] = v
	}
	for k, v := range s.RegionFilters[region] {
		f[k] = v
	}
	offer := region
	if s.OfferRegion != "" {
		offer = s.OfferRegion
	}
	return f, offer
}

// Match is one priced dimension of a matching product.
type Match struct {
	Product   Product
	Dimension Dimension
	USD       float64
}

// Client downloads and caches offer files.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	MaxAge   time.Duration
}

// NewClient caches under the user cache directory for a day.
func NewClient() *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Minute}, CacheDir: filepath.Join(dir, "archgopher", "pricelist"), MaxAge: 24 * time.Hour}
}

// Offer returns the offer file of a service in a region.
func (c *Client) Offer(service, region string) (*Offer, error) {
	path := filepath.Join(c.CacheDir, service, region+".json")
	if st, err := os.Stat(path); err != nil || time.Since(st.ModTime()) > c.MaxAge {
		if err := c.download(service, region, path); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var o Offer
	if err := json.NewDecoder(f).Decode(&o); err != nil {
		return nil, fmt.Errorf("%s %s: %w", service, region, err)
	}
	return &o, nil
}

func (c *Client) download(service, region, path string) error {
	url := fmt.Sprintf("%s/offers/v1.0/aws/%s/current/%s/index.json", BaseURL, service, region)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find returns every priced dimension of the products that match filters.
func (o *Offer) Find(filters map[string]string) ([]Match, error) {
	res := map[string]*regexp.Regexp{}
	for k, v := range filters {
		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", k, err)
		}
		res[k] = re
	}
	var out []Match
	for sku, p := range o.Products {
		if !matches(p, res) {
			continue
		}
		for _, term := range o.Terms.OnDemand[sku] {
			for _, d := range term.PriceDimensions {
				usd, err := strconv.ParseFloat(d.PricePerUnit["USD"], 64)
				if err != nil {
					continue
				}
				out = append(out, Match{Product: p, Dimension: d, USD: usd})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Product.SKU != b.Product.SKU {
			return a.Product.Attributes["usagetype"]+a.Product.SKU < b.Product.Attributes["usagetype"]+b.Product.SKU
		}
		return begin(a.Dimension) < begin(b.Dimension)
	})
	return out, nil
}

func matches(p Product, res map[string]*regexp.Regexp) bool {
	for k, re := range res {
		v := p.Attributes[k]
		if k == "productFamily" {
			v = p.ProductFamily
		}
		if !re.MatchString(v) {
			return false
		}
	}
	return true
}

func begin(d Dimension) float64 {
	v, err := strconv.ParseFloat(d.BeginRange, 64)
	if err != nil {
		return 0
	}
	return v
}

// Resolve finds exactly one price for a spec, or explains why it cannot.
func (c *Client) Resolve(spec Spec, region string) (Match, error) {
	filters, offerRegion := spec.For(region)
	o, err := c.Offer(spec.Service, offerRegion)
	if err != nil {
		return Match{}, err
	}
	ms, err := o.Find(filters)
	if err != nil {
		return Match{}, err
	}
	skus := map[string][]Match{}
	for _, m := range ms {
		skus[m.Product.SKU] = append(skus[m.Product.SKU], m)
	}
	switch len(skus) {
	case 0:
		return Match{}, fmt.Errorf("no product matches %v", filters)
	case 1:
	default:
		var names []string
		for _, list := range skus {
			names = append(names, list[0].Product.Attributes["usagetype"])
		}
		sort.Strings(names)
		return Match{}, fmt.Errorf("%d products match %v: %s", len(skus), filters, strings.Join(names, ", "))
	}
	for _, list := range skus {
		return pickTier(list, spec.Tier)
	}
	panic("unreachable")
}

func pickTier(list []Match, tier string) (Match, error) {
	switch tier {
	case "", "first":
		return list[0], nil
	case "last":
		return list[len(list)-1], nil
	}
	want, err := strconv.ParseFloat(tier, 64)
	if err != nil {
		return Match{}, fmt.Errorf("tier %q: want first, last or a beginRange", tier)
	}
	for _, m := range list {
		if begin(m.Dimension) == want {
			return m, nil
		}
	}
	return Match{}, fmt.Errorf("no tier begins at %s", tier)
}
