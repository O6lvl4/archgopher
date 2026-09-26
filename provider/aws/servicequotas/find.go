package servicequotas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Defaults lists a service's default quotas in region, from the cache when
// it is fresh.
func (c *Client) Defaults(service, region string) ([]Quota, error) {
	key := service + "@" + region
	c.mu.Lock()
	if q, ok := c.defaults[key]; ok {
		c.mu.Unlock()
		return q, nil
	}
	c.mu.Unlock()
	path := filepath.Join(c.CacheDir, region, service+".json")
	quotas, err := readCache(path, c.MaxAge)
	if err != nil {
		if quotas, err = c.listDefaults(service, region); err != nil {
			return nil, err
		}
		writeCache(path, quotas)
	}
	c.mu.Lock()
	if c.defaults == nil {
		c.defaults = map[string][]Quota{}
	}
	c.defaults[key] = quotas
	c.mu.Unlock()
	return quotas, nil
}

func (c *Client) listDefaults(service, region string) ([]Quota, error) {
	var all []Quota
	token := ""
	for {
		in := map[string]any{"ServiceCode": service, "MaxResults": 100}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			Quotas    []Quota
			NextToken string
		}
		if err := c.call(region, "ListAWSDefaultServiceQuotas", in, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Quotas...)
		if token = out.NextToken; token == "" {
			return all, nil
		}
	}
}

// Resolve finds the default value of the quota a spec names in region.
func (c *Client) Resolve(spec Spec, region string) (Quota, error) {
	if spec.Region != "" {
		region = spec.Region
	}
	quotas, err := c.Defaults(spec.Service, region)
	if err != nil {
		return Quota{}, err
	}
	for _, q := range quotas {
		if q.QuotaCode == spec.Quota {
			return q, nil
		}
	}
	return Quota{}, fmt.Errorf("%s %s in %s: %w", spec.Service, spec.Quota, region, ErrAbsent)
}

// Applied reads the value the account runs against: the default, or what
// an increase raised it to.
func (c *Client) Applied(spec Spec, region string) (Quota, error) {
	if spec.Region != "" {
		region = spec.Region
	}
	var out struct{ Quota Quota }
	err := c.call(region, "GetServiceQuota", map[string]string{"ServiceCode": spec.Service, "QuotaCode": spec.Quota}, &out)
	return out.Quota, err
}

func readCache(path string, maxAge time.Duration) ([]Quota, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > maxAge {
		return nil, fmt.Errorf("%s is stale", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var q []Quota
	return q, json.Unmarshal(data, &q)
}

// writeCache keeps a listing for the next run; a failure only costs a
// download next time.
func writeCache(path string, quotas []Quota) {
	data, err := json.Marshal(quotas)
	if err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".quotas-*")
	if err != nil {
		return
	}
	if _, err := tmp.Write(data); err == nil && tmp.Close() == nil {
		_ = os.Rename(tmp.Name(), path)
		return
	}
	_ = tmp.Close()
	_ = os.Remove(tmp.Name())
}
