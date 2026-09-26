package servicequotas

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Credentials sign requests; SessionToken is empty for long-lived keys.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// sign adds a Signature Version 4 authorization to a request whose body is
// body, for service in region, at now.
func (c Credentials) sign(r *http.Request, body []byte, service, region string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	r.Header.Set("X-Amz-Date", stamp)
	if c.SessionToken != "" {
		r.Header.Set("X-Amz-Security-Token", c.SessionToken)
	}
	r.Header.Set("Host", r.URL.Host)
	names, canonical := canonicalHeaders(r.Header)
	request := strings.Join([]string{r.Method, canonicalPath(r.URL.EscapedPath()), r.URL.RawQuery, canonical, names, hashHex(body)}, "\n")
	scope := day + "/" + region + "/" + service + "/aws4_request"
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", stamp, scope, hashHex([]byte(request))}, "\n")
	key := hmacSHA256([]byte("AWS4"+c.SecretAccessKey), day)
	for _, part := range []string{region, service, "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, toSign))
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.AccessKeyID+"/"+scope+", SignedHeaders="+names+", Signature="+signature)
}

// canonicalHeaders lists the headers to sign, lower-cased and sorted.
func canonicalHeaders(h http.Header) (names, canonical string) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + ":" + strings.TrimSpace(strings.Join(h.Values(k), ",")) + "\n")
	}
	return strings.Join(keys, ";"), b.String()
}

func canonicalPath(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
