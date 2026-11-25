package adaptersdk

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func classifyGitHubStatus(resp *http.Response, now time.Time) *Error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return classifyStatus(resp, now)
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	retry := retryAfter(resp.Header.Get("Retry-After"), now)
	primary := resp.Header.Get("X-RateLimit-Remaining") == "0"
	secondary := strings.Contains(strings.ToLower(body.Message), "secondary rate limit")
	if resp.StatusCode == http.StatusForbidden && !primary && !secondary && retry == nil {
		return classifyStatus(resp, now)
	}
	if retry == nil && primary && !secondary {
		if epoch, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && epoch > now.Unix() {
			at := time.Unix(epoch, 0).UTC()
			retry = &at
		}
	}
	if retry == nil {
		at := now.Add(time.Minute).UTC()
		retry = &at
	}
	return &Error{Code: AdapterErrorCodeRateLimited, Detail: "GitHub request rate limited", RetryAt: retry}
}
