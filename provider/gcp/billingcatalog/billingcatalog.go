// Package billingcatalog reads the Google Cloud Billing Catalog API. Unlike
// the AWS and Azure price lists it needs credentials: an OAuth access token
// (ARCHGOPHER_GCP_TOKEN, or gcloud's for ARCHGOPHER_GCP_ACCOUNT) or an API key
// (ARCHGOPHER_GCP_API_KEY). The prices themselves are public.
package billingcatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source names this price source in a book's sync spec.
const Source = "gcp"

// BaseURL is the Cloud Billing Catalog endpoint.
const BaseURL = "https://cloudbilling.googleapis.com/v1"

// ErrAbsent means the catalog lists nothing that matches in the region.
var ErrAbsent = errors.New("not in the Google Cloud Billing Catalog")

// ErrNoCredentials means no token or key is configured.
var ErrNoCredentials = errors.New("no Google Cloud credentials: set ARCHGOPHER_GCP_TOKEN, ARCHGOPHER_GCP_ACCOUNT (gcloud) or ARCHGOPHER_GCP_API_KEY")

// Money is the catalog's price: whole units and nanos.
type Money struct {
	Units string `json:"units"`
	Nanos int64  `json:"nanos"`
}

// USD is the price as a float.
func (m Money) USD() float64 {
	u, _ := strconv.ParseFloat(m.Units, 64)
	return u + float64(m.Nanos)/1e9
}

// Rate is one tier of a price.
type Rate struct {
	Start     float64 `json:"startUsageAmount"`
	UnitPrice Money   `json:"unitPrice"`
}

// Sku is one priced item.
type Sku struct {
	SkuID       string `json:"skuId"`
	Description string `json:"description"`
	Category    struct {
		ServiceDisplayName string `json:"serviceDisplayName"`
		ResourceFamily     string `json:"resourceFamily"`
		ResourceGroup      string `json:"resourceGroup"`
		UsageType          string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string `json:"serviceRegions"`
	// GeoTaxonomy names the regions of a sku listed under "global" whose
	// region is only in its description (Firestore: "Read Ops Tokyo").
	GeoTaxonomy struct {
		Type    string   `json:"type"`
		Regions []string `json:"regions"`
	} `json:"geoTaxonomy"`
	PricingInfo []struct {
		PricingExpression struct {
			UsageUnit            string  `json:"usageUnit"`
			UsageUnitDescription string  `json:"usageUnitDescription"`
			TieredRates          []Rate  `json:"tieredRates"`
			BaseUnitFactor       float64 `json:"baseUnitConversionFactor"`
		} `json:"pricingExpression"`
	} `json:"pricingInfo"`
}

// Field reads an attribute by name, for filters.
func (s Sku) Field(name string) string {
	switch name {
	case "description":
		return s.Description
	case "resourceFamily":
		return s.Category.ResourceFamily
	case "resourceGroup":
		return s.Category.ResourceGroup
	case "usageType":
		return s.Category.UsageType
	case "usageUnit":
		return s.Unit()
	case "skuId":
		return s.SkuID
	}
	return ""
}

// Unit is the usage unit the tiers are priced in ("GiBy.s", "h", "count").
func (s Sku) Unit() string {
	if len(s.PricingInfo) == 0 {
		return ""
	}
	return s.PricingInfo[0].PricingExpression.UsageUnit
}

// Rates are the tiers of the current price.
func (s Sku) Rates() []Rate {
	if len(s.PricingInfo) == 0 {
		return nil
	}
	return s.PricingInfo[0].PricingExpression.TieredRates
}

// In reports whether the sku is sold in a region ("global" counts everywhere
// only when the spec asks for it).
func (s Sku) In(region string) bool {
	for _, r := range s.ServiceRegions {
		if r == region {
			return true
		}
	}
	if s.GeoTaxonomy.Type == "REGIONAL" {
		for _, r := range s.GeoTaxonomy.Regions {
			if r == region {
				return true
			}
		}
	}
	return false
}

// Spec says how to find one price. Service is the catalog's service id
// ("152E-C115-5142" is Cloud Run); filters are regular expressions that must
// match the whole attribute; usageType defaults to OnDemand.
type Spec struct {
	Source  string            `json:"source"`
	Service string            `json:"service"`
	Filters map[string]string `json:"filters"`
	// Tier picks a tier: the default is the first with a price (a free grant
	// comes first at zero), or "first", "last" or a startUsageAmount.
	Tier string `json:"tier,omitempty"`
	// Region reads the sku sold in a fixed region ("global") whatever the
	// book region; its row is "*".
	Region string `json:"region,omitempty"`
	// ListPer is how many book units the catalog price covers. 0 is one.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts a catalog price to the price of one book unit.
func (s Spec) PerUnit(usd float64) float64 {
	if s.ListPer == 0 {
		return usd
	}
	return usd / s.ListPer
}

// For returns the region to look in for one book region.
func (s Spec) For(region string) string {
	if s.Region != "" {
		return s.Region
	}
	return region
}

// Client downloads and caches a service's skus.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	MaxAge   time.Duration
	BaseURL  string
	// Auth sets credentials on a request; nil means read them from the
	// environment on first use.
	Auth func(*http.Request) error

	// A price table resolves thousands of rows against one service: its
	// skus stay in memory, and so do the skus each set of filters matches
	// in any region.
	mu      sync.Mutex
	skus    map[string][]Sku
	matched map[string][]Sku
}

// NewClient caches under the user cache directory for a day.
func NewClient() *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Client{HTTP: &http.Client{Timeout: 2 * time.Minute}, CacheDir: filepath.Join(dir, "archgopher", "gcp-prices"), MaxAge: 24 * time.Hour, BaseURL: BaseURL}
}

// EnvAuth reads credentials from the environment.
func EnvAuth() (func(*http.Request) error, error) {
	if key := os.Getenv("ARCHGOPHER_GCP_API_KEY"); key != "" {
		return func(r *http.Request) error {
			q := r.URL.Query()
			q.Set("key", key)
			r.URL.RawQuery = q.Encode()
			return nil
		}, nil
	}
	token := os.Getenv("ARCHGOPHER_GCP_TOKEN")
	if token == "" {
		if account := os.Getenv("ARCHGOPHER_GCP_ACCOUNT"); account != "" {
			out, err := exec.Command("gcloud", "auth", "print-access-token", "--account", account).Output()
			if err != nil {
				return nil, fmt.Errorf("gcloud auth print-access-token --account %s: %w", account, err)
			}
			token = strings.TrimSpace(string(out))
		}
	}
	if token == "" {
		return nil, ErrNoCredentials
	}
	return func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	}, nil
}

// Skus returns every sku of a service.
func (c *Client) Skus(service string) ([]Sku, error) {
	path := filepath.Join(c.CacheDir, service+".json")
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) <= c.MaxAge {
		if skus, err := readCache(path); err == nil {
			return skus, nil
		}
	}
	if c.Auth == nil {
		auth, err := EnvAuth()
		if err != nil {
			return nil, err
		}
		c.Auth = auth
	}
	var all []Sku
	token := ""
	for {
		page, next, err := c.page(service, token)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		token = next
	}
	if err := writeCache(path, all); err != nil {
		return nil, err
	}
	return all, nil
}

func (c *Client) page(service, token string) ([]Sku, string, error) {
	u := fmt.Sprintf("%s/services/%s/skus?currencyCode=USD&pageSize=5000", c.BaseURL, url.PathEscape(service))
	if token != "" {
		u += "&pageToken=" + url.QueryEscape(token)
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	if err := c.Auth(req); err != nil {
		return nil, "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("GET services/%s/skus: %s: %s", service, resp.Status, body)
	}
	var p struct {
		Skus          []Sku  `json:"skus"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, "", err
	}
	return p.Skus, p.NextPageToken, nil
}

// Find returns the skus sold in region that match filters (usageType
// defaults to OnDemand).
func Find(skus []Sku, region string, filters map[string]string) ([]Sku, error) {
	f := map[string]string{"usageType": "OnDemand"}
	for k, v := range filters {
		f[k] = v
	}
	res := map[string]*regexp.Regexp{}
	for k, v := range f {
		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", k, err)
		}
		res[k] = re
	}
	var out []Sku
	for _, s := range skus {
		if region != "" && !s.In(region) {
			continue
		}
		ok := true
		for k, re := range res {
			if !re.MatchString(s.Field(k)) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Description+out[i].SkuID < out[j].Description+out[j].SkuID })
	return out, nil
}

// Price is a resolved sku tier.
type Price struct {
	Sku  Sku
	Rate Rate
}

// matching returns the skus of the spec's service that match its filters in
// any region, remembered per service and filters.
func (c *Client) matching(spec Spec) ([]Sku, error) {
	key, err := json.Marshal([]any{spec.Service, spec.Filters})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if m, ok := c.matched[string(key)]; ok {
		return m, nil
	}
	skus, ok := c.skus[spec.Service]
	if !ok {
		if skus, err = c.Skus(spec.Service); err != nil {
			return nil, err
		}
		if c.skus == nil {
			c.skus = map[string][]Sku{}
		}
		c.skus[spec.Service] = skus
	}
	m, err := Find(skus, "", spec.Filters)
	if err != nil {
		return nil, err
	}
	if c.matched == nil {
		c.matched = map[string][]Sku{}
	}
	c.matched[string(key)] = m
	return m, nil
}

// Resolve finds exactly one price for a spec, or explains why it cannot.
func (c *Client) Resolve(spec Spec, region string) (Price, error) {
	all, err := c.matching(spec)
	if err != nil {
		return Price{}, err
	}
	var ms []Sku
	for _, s := range all {
		if s.In(spec.For(region)) {
			ms = append(ms, s)
		}
	}
	switch len(ms) {
	case 0:
		return Price{}, fmt.Errorf("%w: no sku matches %v in %s", ErrAbsent, spec.Filters, spec.For(region))
	case 1:
	default:
		var names []string
		for _, m := range ms {
			names = append(names, m.Description)
		}
		return Price{}, fmt.Errorf("%d skus match %v: %s", len(ms), spec.Filters, strings.Join(names, "; "))
	}
	rate, err := pickTier(ms[0].Rates(), spec.Tier, spec.ListPer)
	if err != nil {
		return Price{}, err
	}
	return Price{Sku: ms[0], Rate: rate}, nil
}

func pickTier(rates []Rate, tier string, listPer float64) (Rate, error) {
	if len(rates) == 0 {
		return Rate{}, fmt.Errorf("the sku has no price")
	}
	switch tier {
	case "":
		for _, r := range rates {
			if r.UnitPrice.USD() > 0 {
				return r, nil
			}
		}
		return rates[0], nil
	case "first":
		return rates[0], nil
	case "last":
		return rates[len(rates)-1], nil
	}
	want, err := strconv.ParseFloat(tier, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("tier %q: want first, last or where the tier starts", tier)
	}
	if listPer == 0 {
		listPer = 1
	}
	// The start is in the book's units: startUsageAmount × listPer. A region
	// whose SKU does not break there bills the rate in effect at that point.
	var in *Rate
	for i, r := range rates {
		if start := r.Start * listPer; start <= want*(1+1e-12) {
			if in == nil || r.Start >= in.Start {
				in = &rates[i]
			}
		}
	}
	if in == nil {
		return Rate{}, fmt.Errorf("no tier is in effect at %s", tier)
	}
	return *in, nil
}

func readCache(path string) ([]Sku, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var skus []Sku
	return skus, json.Unmarshal(data, &skus)
}

func writeCache(path string, skus []Sku) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(skus)
	if err != nil {
		return err
	}
	// A temporary file of its own, so processes caching the same service at
	// once never write into one file.
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
