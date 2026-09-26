// Package cloudquotas reads Google Cloud quotas from the Cloud Quotas API:
// the value a project runs against, by region where the quota has regions.
// The API needs credentials and must be enabled in the project.
package cloudquotas

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Source names this source in a quota's code.
const Source = "cloudquotas"

// ErrAbsent means the service lists no such quota, or none for the region.
var ErrAbsent = errors.New("not in Cloud Quotas")

// Spec names one quota.
type Spec struct {
	Source string `json:"source"`
	// Service is the API's name ("run.googleapis.com").
	Service string `json:"service"`
	// Quota is the quota id ("InstancesPerProjectPerRegion").
	Quota string `json:"quota"`
	// ListPer is how many book units one listed unit covers. 0 is 1.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts a listed value into the book's unit.
func (s Spec) PerUnit(v float64) float64 {
	if s.ListPer == 0 {
		return v
	}
	return v / s.ListPer
}

// Info is one quota as the API lists it.
type Info struct {
	QuotaID     string          `json:"quotaId"`
	DisplayName string          `json:"quotaDisplayName"`
	Metric      string          `json:"metricDisplayName"`
	Unit        string          `json:"metricUnit"`
	Dimensions  []DimensionInfo `json:"dimensionsInfos"`
}

// DimensionInfo is the value for one set of dimensions; none applies
// wherever no other matches.
type DimensionInfo struct {
	Dimensions map[string]string `json:"dimensions"`
	Details    struct {
		Value string `json:"value"`
	} `json:"details"`
}

// Value is the quota's value in region: the region's own, else the one
// without dimensions.
func (i Info) Value(region string) (float64, error) {
	var fallback *DimensionInfo
	for k := range i.Dimensions {
		d := &i.Dimensions[k]
		if !d.set() {
			continue
		}
		switch {
		case d.Dimensions["region"] == region:
			return strconv.ParseFloat(d.Details.Value, 64)
		case len(d.Dimensions) == 0:
			fallback = d
		}
	}
	if fallback == nil {
		return 0, fmt.Errorf("%s in %s: %w", i.QuotaID, region, ErrAbsent)
	}
	return strconv.ParseFloat(fallback.Details.Value, 64)
}

// set reports whether the dimensions carry a limit: an empty value is not
// set, and -1 means no limit.
func (d DimensionInfo) set() bool {
	return d.Details.Value != "" && d.Details.Value != "-1"
}

// Label names the quota in a report.
func (i Info) Label() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	return i.QuotaID
}

// Client reads the Cloud Quotas API.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	// Auth sets credentials on a request.
	Auth func(*http.Request) error
}

// NewClient reads the public API with auth.
func NewClient(auth func(*http.Request) error) *Client {
	return &Client{HTTP: &http.Client{Timeout: time.Minute}, BaseURL: "https://cloudquotas.googleapis.com/v1", Auth: auth}
}

// Info reads one quota of a project.
func (c *Client) Info(project string, spec Spec) (Info, error) {
	u := fmt.Sprintf("%s/projects/%s/locations/global/services/%s/quotaInfos/%s", c.BaseURL, url.PathEscape(project), url.PathEscape(spec.Service), url.PathEscape(spec.Quota))
	var info Info
	return info, c.get(project, u, &info)
}

// List reads every quota a service has in a project.
func (c *Client) List(project, service string) ([]Info, error) {
	var all []Info
	token := ""
	for {
		u := fmt.Sprintf("%s/projects/%s/locations/global/services/%s/quotaInfos?pageSize=100", c.BaseURL, url.PathEscape(project), url.PathEscape(service))
		if token != "" {
			u += "&pageToken=" + url.QueryEscape(token)
		}
		var page struct {
			QuotaInfos    []Info `json:"quotaInfos"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.get(project, u, &page); err != nil {
			return nil, err
		}
		all = append(all, page.QuotaInfos...)
		if token = page.NextPageToken; token == "" {
			return all, nil
		}
	}
}

func (c *Client) get(project, u string, out any) error {
	r, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if err := c.Auth(r); err != nil {
		return err
	}
	// User credentials bill the call to the project they read.
	r.Header.Set("X-Goog-User-Project", project)
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return json.Unmarshal(data, out)
	case http.StatusNotFound:
		return fmt.Errorf("%s: %w", u, ErrAbsent)
	}
	var e struct {
		Error struct{ Message string } `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	return fmt.Errorf("Cloud Quotas API: HTTP %d %s", resp.StatusCode, e.Error.Message)
}
