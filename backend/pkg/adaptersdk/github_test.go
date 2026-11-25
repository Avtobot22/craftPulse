package adaptersdk

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGitHubStatus(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		status  int
		headers http.Header
		body    string
		code    AdapterErrorCode
		delay   time.Duration
	}{
		{"bad token", 401, nil, "", AdapterErrorCodeAuth, 0},
		{"forbidden", 403, nil, "", AdapterErrorCodeAuth, 0},
		{"primary", 403, http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"1795255320"}}, "", AdapterErrorCodeRateLimited, 2 * time.Minute},
		{"secondary 403", 403, nil, `{"message":"You have exceeded a secondary rate limit"}`, AdapterErrorCodeRateLimited, time.Minute},
		{"secondary 429", 429, http.Header{"Retry-After": {"15"}}, "", AdapterErrorCodeRateLimited, 15 * time.Second},
		{"upstream failure", 503, nil, "", AdapterErrorCodeUnavailable, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyGitHubStatus(&http.Response{StatusCode: tc.status, Header: tc.headers, Body: io.NopCloser(strings.NewReader(tc.body))}, now)
			if got.Code != tc.code {
				t.Errorf("want %q, got %q", tc.code, got.Code)
			}
			if tc.delay == 0 && got.RetryAt != nil {
				t.Errorf("unexpected retry time: %v", got.RetryAt)
			}
			if tc.delay != 0 && (got.RetryAt == nil || !got.RetryAt.Equal(now.Add(tc.delay))) {
				t.Errorf("want retry in %v, got %v", tc.delay, got.RetryAt)
			}
		})
	}
}
