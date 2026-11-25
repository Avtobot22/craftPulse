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
	resp, err := s.request(ctx, operation, path) //nolint:bodyclose // DecodeJSON owns the body
	if err != nil {
		return err
	}
	return adaptersdk.DecodeJSON(resp, dst)
}

func nextPage(links []string, current int) (int, error) {
	query, err := nextQuery(links)
	if err != nil || query == nil {
		return 0, err
	}
	n, err := strconv.Atoi(query.Get("page"))
	if err != nil || n <= current || n-current != 1 {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "invalid GitHub next page")
	}
	return n, nil
}

func nextQuery(links []string) (url.Values, error) {
	for _, header := range links {
		for _, link := range strings.Split(header, ",") {
			target, params, ok := strings.Cut(strings.TrimSpace(link), ">")
			if !ok && strings.Contains(target, `; rel="next"`) {
				return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "invalid GitHub next page")
			}
			if !strings.Contains(params, `; rel="next"`) {
				continue
			}
			u, err := url.Parse(strings.TrimPrefix(target, "<"))
			if err == nil && strings.HasPrefix(target, "<") && u.Scheme == "https" && u.Host == "api.github.com" &&
				u.User == nil && u.Fragment == "" {
				// Keep the original endpoint. GitHub can use numeric repository aliases in Link.
				return u.Query(), nil
			}
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "invalid GitHub next page")
		}
	}
	return nil, nil
}

func walk[T any](ctx context.Context, s source, operation, path string, query url.Values, firstOnly bool, visit func(T) error) error {
	for current := 1; ; {
		links, err := arrayPage(ctx, s, operation, path, query, current > 1, visit)
		if err != nil {
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
		current = next
	}
}

func arrayPage[T any](ctx context.Context, s source, operation, path string, query url.Values, continuation bool, visit func(T) error) ([]string, error) {
	resp, err := s.request(ctx, operation, path+"?"+query.Encode()) //nolint:bodyclose // DecodeJSONArray owns the body
	if err != nil {
		if continuation && hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub next page is unavailable")
		}
		return nil, err
	}
	links := resp.Header.Values("Link")
	err = adaptersdk.DecodeJSONArray(resp, visit)
	if continuation && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub next page was cut short")
	}
	return links, err
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
