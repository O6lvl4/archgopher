package servicequotas

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The example of the AWS Signature Version 4 test suite ("get-vanilla"):
// GET / on example.amazonaws.com, service "service", us-east-1, 2015-08-30.
func TestSignMatchesTheAWSTestSuite(t *testing.T) {
	r, err := http.NewRequest("GET", "https://example.amazonaws.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"}
	c.sign(r, nil, "service", "us-east-1", time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !strings.HasPrefix(r.Header.Get("X-Amz-Date"), "20150830T123600Z") {
		t.Fatalf("date %s", r.Header.Get("X-Amz-Date"))
	}
}
