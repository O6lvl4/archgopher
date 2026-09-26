package servicequotas

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fake answers ListAWSDefaultServiceQuotas in two pages and GetServiceQuota
// for one quota, and checks every call is signed.
func fake(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKID/") {
			t.Errorf("unsigned call: %q", r.Header.Get("Authorization"))
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		switch r.Header.Get("X-Amz-Target") {
		case "ServiceQuotasV20190624.ListAWSDefaultServiceQuotas":
			if in["NextToken"] == nil {
				reply(t, w, map[string]any{"Quotas": []Quota{{QuotaCode: "L-1", QuotaName: "Concurrent executions", Value: 1000}}, "NextToken": "p2"})
				return
			}
			reply(t, w, map[string]any{"Quotas": []Quota{{QuotaCode: "L-2", QuotaName: "Rate", Value: 5, Unit: "None"}}})
		case "ServiceQuotasV20190624.GetServiceQuota":
			if in["QuotaCode"] != "L-1" {
				w.WriteHeader(http.StatusBadRequest)
				reply(t, w, map[string]string{"__type": "NoSuchResourceException", "message": "no"})
				return
			}
			reply(t, w, map[string]any{"Quota": Quota{QuotaCode: "L-1", Value: 3000}})
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient()
	c.CacheDir = t.TempDir()
	c.Endpoint = func(string) string { return srv.URL + "/" }
	c.Credentials = func() (Credentials, error) { return Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}, nil }
	return c
}

func reply(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}

func TestResolveReadsEveryPageOfDefaults(t *testing.T) {
	c := fake(t)
	q, err := c.Resolve(Spec{Service: "lambda", Quota: "L-2"}, "ap-northeast-1")
	if err != nil || q.Value != 5 {
		t.Fatalf("got %+v, %v", q, err)
	}
	if _, err := c.Resolve(Spec{Service: "lambda", Quota: "L-9"}, "ap-northeast-1"); !errors.Is(err, ErrAbsent) {
		t.Fatalf("a quota not listed: %v", err)
	}
}

func TestAppliedReadsTheAccountsValue(t *testing.T) {
	c := fake(t)
	q, err := c.Applied(Spec{Service: "lambda", Quota: "L-1"}, "ap-northeast-1")
	if err != nil || q.Value != 3000 {
		t.Fatalf("got %+v, %v", q, err)
	}
	if _, err := c.Applied(Spec{Service: "lambda", Quota: "L-9"}, "ap-northeast-1"); !errors.Is(err, ErrAbsent) {
		t.Fatalf("an unknown quota: %v", err)
	}
}

func TestNoProfileIsNoCredentials(t *testing.T) {
	t.Setenv("ARCHGOPHER_AWS_PROFILE", "")
	t.Setenv("AWS_PROFILE", "")
	if _, err := CLICredentials(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}
