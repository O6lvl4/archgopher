package billingcatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

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
	token, err := envToken()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrNoCredentials
	}
	return func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	}, nil
}

// envToken is the OAuth access token the environment gives, or gcloud's for
// the account it names; empty when it names neither.
func envToken() (string, error) {
	if token := os.Getenv("ARCHGOPHER_GCP_TOKEN"); token != "" {
		return token, nil
	}
	account := os.Getenv("ARCHGOPHER_GCP_ACCOUNT")
	if account == "" {
		return "", nil
	}
	out, err := exec.Command("gcloud", "auth", "print-access-token", "--account", account).Output()
	if err != nil {
		return "", fmt.Errorf("gcloud auth print-access-token --account %s: %w", account, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Skus returns every sku of a service.
func (c *Client) Skus(service string) ([]Sku, error) {
	path := filepath.Join(c.CacheDir, service+".json")
	if skus, ok := c.cached(path); ok {
		return skus, nil
	}
	all, err := c.download(service)
	if err != nil {
		return nil, err
	}
	if err := writeCache(path, all); err != nil {
		return nil, err
	}
	return all, nil
}

// cached returns the cached skus when they are recent enough and readable.
func (c *Client) cached(path string) ([]Sku, bool) {
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > c.MaxAge {
		return nil, false
	}
	skus, err := readCache(path)
	return skus, err == nil
}

// download reads every page of a service's skus, with the credentials from
// the environment unless the client was given its own.
func (c *Client) download(service string) ([]Sku, error) {
	if c.Auth == nil {
		auth, err := EnvAuth()
		if err != nil {
			return nil, err
		}
		c.Auth = auth
	}
	var all []Sku
	for token := ""; ; {
		page, next, err := c.page(service, token)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			return all, nil
		}
		token = next
	}
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
