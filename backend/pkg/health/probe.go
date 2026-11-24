package health

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"
)

// CheckLocalLiveness is the exec probe for the api and worker containers.
func CheckLocalLiveness(addr string) bool {
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if number, err := strconv.ParseUint(port, 10, 16); err != nil || number == 0 {
		return false
	}
	client := http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://"+net.JoinHostPort("127.0.0.1", port)+"/livez", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
