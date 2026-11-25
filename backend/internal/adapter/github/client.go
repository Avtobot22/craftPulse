package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_apiOrigin     = "https://api.github.com"
	_baseURL       = "https://github.com"
	_apiVersion    = "2022-11-28"
	_edition       = "GitHub.com"
	_maxSourcePage = 4 << 20
)

type source struct {
	client *adaptersdk.SourceClient
	token  string
}

func (s source) request(ctx context.Context, operation, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, _apiOrigin+path, http.NoBody)
	if err != nil {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cannot construct GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", _apiVersion)
	start := time.Now()
	resp, err := s.client.DoGitHub(req, _maxSourcePage)
	log := adaptersdk.Call{}.Log(ctx).Debug().Str("source_operation", operation).Dur("elapsed", time.Since(start))
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		log.Str("reason", string(e.Code)).Int("source_status", e.SourceStatusCode())
	} else if resp != nil {
		log.Int("source_status", resp.StatusCode)
	}
	log.Msg("GitHub request")
	return resp, err
}

func (s source) get(ctx context.Context, operation, path string, dst any) error {
	resp, err := s.request(ctx, operation, path)
	if err != nil {
		return err
	}
	return adaptersdk.DecodeJSON(resp, dst)
}

func nextPage(links []string, current int) (int, error) {
	for _, header := range links {
		for _, link := range strings.Split(header, ",") {
			target, params, ok := strings.Cut(strings.TrimSpace(link), ">")
			if !ok || !strings.Contains(params, `; rel="next"`) {
				continue
			}
			u, err := url.Parse(strings.TrimPrefix(target, "<"))
			if err == nil && strings.HasPrefix(target, "<") && u.Scheme == "https" && u.Host == "api.github.com" &&
				u.User == nil && u.Fragment == "" {
				n, parseErr := strconv.Atoi(u.Query().Get("page"))
				if parseErr == nil && n > current && n-current == 1 {
					// Keep the original endpoint. GitHub can use numeric repository aliases in Link.
					return n, nil
				}
			}
			return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "invalid GitHub next page")
		}
	}
	return 0, nil
}

func walk[T any](ctx context.Context, s source, operation, path string, query url.Values, firstOnly bool, visit func(T) error) error {
	raw := query.Encode()
	for current := 1; ; {
		resp, err := s.request(ctx, operation, path+"?"+raw)
		if err != nil {
			if !firstOnly && current > 1 && hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub next page is unavailable")
			}
			return err
		}
		links := resp.Header.Values("Link")
		if err := adaptersdk.DecodeJSONArray(resp, visit); err != nil {
			// A truncated transfer may look like incomplete JSON to the SDK.
			if current > 1 && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub next page was cut short")
			}
			return err
		}
		if firstOnly {
			return nil
		}
		next, err := nextPage(links, current)
		if err != nil || next == 0 {
			return err
		}
		query.Set("page", strconv.Itoa(next))
		raw = query.Encode()
		current = next
	}
}

func hasCode(err error, code adaptersdk.AdapterErrorCode) bool {
	e, ok := errors.AsType[*adaptersdk.Error](err)
	return ok && e.Code == code
}

func unavailableScope(err error) bool {
	return hasCode(err, adaptersdk.AdapterErrorCodeAuth) || hasCode(err, adaptersdk.AdapterErrorCodeNotFound)
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func numeric(raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n > 0 && id(n) == raw
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
