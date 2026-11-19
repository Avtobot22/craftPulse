package ci

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

var _secretHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"}

type debugTransport struct {
	next http.RoundTripper
	out  io.Writer
}

// DebugTransport prints every request and answer of the step with their headers; the values of
// Authorization and cookies are never printed.
func DebugTransport(next http.RoundTripper, out io.Writer) http.RoundTripper {
	return &debugTransport{next: next, out: out}
}

func (t *debugTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	fmt.Fprintf(t.out, "debug: > %s %s%s\n", r.Method, r.URL.Redacted(), headers(r.Header))
	start := time.Now()
	resp, err := t.next.RoundTrip(r)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Fprintf(t.out, "debug: < error after %s: %v\n", elapsed, err)
		return nil, err
	}
	fmt.Fprintf(t.out, "debug: < %s after %s%s\n", resp.Status, elapsed, headers(resp.Header))
	return resp, nil
}

func headers(h http.Header) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(h)) {
		value := strings.Join(h[name], ", ")
		if slices.Contains(_secretHeaders, http.CanonicalHeaderKey(name)) {
			value = "[REDACTED]"
		}
		b.WriteString("; " + name + ": " + value)
	}
	return b.String()
}
