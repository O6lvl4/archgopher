package cloudquotas

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Goog-User-Project") != "p1" {
			t.Errorf("headers %v", r.Header)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/quotaInfos/Instances"):
			reply(t, w, map[string]any{"quotaId": "Instances", "quotaDisplayName": "Instances per region", "dimensionsInfos": []map[string]any{
				{"dimensions": map[string]string{"region": "asia-northeast1"}, "details": map[string]string{"value": "300"}},
				{"dimensions": map[string]string{"region": "europe-west1"}, "details": map[string]string{"value": ""}},
				{"dimensions": map[string]string{"region": "us-west1"}, "details": map[string]string{"value": "-1"}},
				{"dimensions": map[string]string{}, "details": map[string]string{"value": "100"}},
			}})
		case strings.HasSuffix(r.URL.Path, "/quotaInfos"):
			if r.URL.Query().Get("pageToken") == "" {
				reply(t, w, map[string]any{"quotaInfos": []Info{{QuotaID: "A"}}, "nextPageToken": "n"})
				return
			}
			reply(t, w, map[string]any{"quotaInfos": []Info{{QuotaID: "B"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(func(r *http.Request) error { r.Header.Set("Authorization", "Bearer tok"); return nil })
	c.BaseURL = srv.URL
	return c
}

func reply(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}

func TestInfoPicksTheRegionElseTheDefault(t *testing.T) {
	info, err := fake(t).Info("p1", Spec{Service: "run.googleapis.com", Quota: "Instances"})
	if err != nil {
		t.Fatal(err)
	}
	for region, want := range map[string]float64{"asia-northeast1": 300, "us-east4": 100, "europe-west1": 100, "us-west1": 100} {
		if v, err := info.Value(region); err != nil || v != want {
			t.Errorf("%s: %v, %v; want %v", region, v, err, want)
		}
	}
}

func TestListReadsEveryPage(t *testing.T) {
	all, err := fake(t).List("p1", "run.googleapis.com")
	if err != nil || len(all) != 2 || all[1].QuotaID != "B" {
		t.Fatalf("got %+v, %v", all, err)
	}
}

func TestAnUnknownQuotaIsAbsent(t *testing.T) {
	if _, err := fake(t).Info("p1", Spec{Service: "run.googleapis.com", Quota: "Nope"}); !errors.Is(err, ErrAbsent) {
		t.Fatalf("got %v", err)
	}
}
