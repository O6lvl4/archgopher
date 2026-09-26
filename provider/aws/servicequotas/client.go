package servicequotas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Client reads Service Quotas, caching each service's defaults per region.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	MaxAge   time.Duration
	// Endpoint makes the API's URL for a region; tests point it elsewhere.
	Endpoint func(region string) string
	// Credentials returns the keys to sign with; nil reads them from the
	// AWS CLI on first use.
	Credentials func() (Credentials, error)

	mu       sync.Mutex
	creds    *Credentials
	defaults map[string][]Quota
}

// NewClient caches under the user cache directory for a day.
func NewClient() *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Client{
		HTTP:     &http.Client{Timeout: time.Minute},
		CacheDir: filepath.Join(dir, "archgopher", "servicequotas"),
		MaxAge:   24 * time.Hour,
		Endpoint: func(region string) string { return "https://servicequotas." + region + ".amazonaws.com/" },
	}
}

// EnvProfile is the AWS CLI profile the environment names.
func EnvProfile() string {
	if p := os.Getenv("ARCHGOPHER_AWS_PROFILE"); p != "" {
		return p
	}
	return os.Getenv("AWS_PROFILE")
}

// CLICredentials reads temporary keys from the AWS CLI for the profile the
// environment names.
func CLICredentials() (Credentials, error) { return ProfileCredentials(EnvProfile()) }

// ProfileCredentials reads temporary keys from the AWS CLI for a profile.
func ProfileCredentials(profile string) (Credentials, error) {
	if profile == "" {
		return Credentials{}, ErrNoCredentials
	}
	out, err := exec.Command("aws", "configure", "export-credentials", "--profile", profile, "--format", "process").Output()
	if err != nil {
		return Credentials{}, fmt.Errorf("aws configure export-credentials --profile %s (log in with aws sso login): %w", profile, err)
	}
	var c struct{ AccessKeyId, SecretAccessKey, SessionToken string }
	if err := json.Unmarshal(out, &c); err != nil {
		return Credentials{}, err
	}
	return Credentials{AccessKeyID: c.AccessKeyId, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken}, nil
}

func (c *Client) credentials() (Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil {
		return *c.creds, nil
	}
	read := c.Credentials
	if read == nil {
		read = CLICredentials
	}
	creds, err := read()
	if err != nil {
		return Credentials{}, err
	}
	c.creds = &creds
	return creds, nil
}

// call posts one Service Quotas operation in region and decodes its answer.
func (c *Client) call(region, op string, in, out any) error {
	creds, err := c.credentials()
	if err != nil {
		return err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	r, err := http.NewRequest(http.MethodPost, c.Endpoint(region), bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/x-amz-json-1.1")
	r.Header.Set("X-Amz-Target", "ServiceQuotasV20190624."+op)
	creds.sign(r, body, "servicequotas", region, time.Now())
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return apiError(op, region, resp.StatusCode, data)
	}
	return json.Unmarshal(data, out)
}

// apiError is a failed call; a quota the region does not list is ErrAbsent.
func apiError(op, region string, status int, data []byte) error {
	var e struct {
		Type    string `json:"__type"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &e)
	if status == http.StatusBadRequest && (e.Type == "NoSuchResourceException" || e.Type == "com.amazonaws.servicequotas.model#NoSuchResourceException") {
		return fmt.Errorf("%s in %s: %w", op, region, ErrAbsent)
	}
	return fmt.Errorf("%s in %s: HTTP %d %s %s", op, region, status, e.Type, e.Message)
}
