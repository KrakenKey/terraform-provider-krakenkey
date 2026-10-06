package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIErrorMessages(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header string
		body   string
		want   string
	}{
		{"string message", 402, "", `{"statusCode":402,"message":"Total active certificate limit reached"}`,
			"KrakenKey API returned 402: Total active certificate limit reached"},
		{"validation array", 400, "", `{"statusCode":400,"message":["csrPem must be a string","csrPem should not be empty"]}`,
			"returned 400: csrPem must be a string; csrPem should not be empty"},
		{"rate limited", 429, "3593", `{"statusCode":429,"message":"ThrottlerException: Too Many Requests"}`,
			"(rate limited; try again in 1h0m0s)"},
		{"no JSON body", 502, "", `bad gateway`, "returned 502: 502 Bad Gateway"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := New(srv.URL, "kk_test", "test").CreateCertificate(context.Background(), "csr")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
