package gitlab

import (
	"context"
	"encoding/json"
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
	_sourcePageSize     = 25
	_maxSourcePageBytes = 4 << 20
)

type source struct {
	client  *adaptersdk.SourceClient
	binding adaptersdk.SourceBinding
	token   string
}

func normalizeBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab base_url must be an HTTPS address without credentials, query or fragment")
	}
	u.Host = strings.ToLower(u.Host)
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String(), nil
}

func tokenFrom(secret string) (string, error) {
	var creds struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(secret), &creds); err != nil || strings.TrimSpace(creds.Token) == "" {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab collect credentials require a token")
	}
	return creds.Token, nil
}

func (s source) request(ctx context.Context, operation, path string, query url.Values) (*http.Response, error) {
	return s.requestURL(ctx, operation, "/api/v4/"+path, query, "application/json", _maxSourcePageBytes)
}

func (s source) requestURL(ctx context.Context, operation, path string, query url.Values, accept string, size int64) (*http.Response, error) {
	address := s.binding.BaseURL + path
	if len(query) != 0 {
		address += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cannot construct the GitLab request")
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("PRIVATE-TOKEN", s.token)
	start := time.Now()
	res, err := s.client.DoDirect(req, size)
	log := adaptersdk.Call{}.Log(ctx).Debug().Str("source_operation", operation).Dur("elapsed", time.Since(start))
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		log.Str("reason", string(e.Code))
	} else if res != nil {
		log.Int("status", res.StatusCode)
	}
	log.Msg("GitLab request")
	return res, err
}

func (s source) get(ctx context.Context, operation, path string, out any) error {
	resp, err := s.request(ctx, operation, path, nil) //nolint:bodyclose // DecodeJSON owns the body
	if err != nil {
		return err
	}
	return adaptersdk.DecodeJSON(resp, out)
}

func readPage[T any](ctx context.Context, s source, operation, path string, query url.Values, current, size int, visit func(T) error) (int, error) {
	query.Set("page", strconv.Itoa(current))
	query.Set("per_page", strconv.Itoa(size))
	resp, err := s.request(ctx, operation, path, query) //nolint:bodyclose // DecodeJSONArray owns the body
	if err != nil {
		if current > 1 && hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
			return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "the next GitLab page is unavailable")
		}
		return 0, err
	}
	if err := adaptersdk.DecodeJSONArray(resp, visit); err != nil {
		if current > 1 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "the next GitLab page was interrupted")
		}
		return 0, err
	}
	return nextPage(resp.Header, current)
}

func nextPage(header http.Header, current int) (int, error) {
	value := strings.TrimSpace(header.Get("X-Next-Page"))
	if value == "" {
		for _, link := range strings.Split(header.Get("Link"), ",") {
			addr, params, ok := strings.Cut(strings.TrimSpace(link), ">")
			if !ok || !strings.HasPrefix(addr, "<") || !strings.Contains(params, `rel="next"`) {
				continue
			}
			u, err := url.Parse(addr[1:])
			if err != nil || u.Query().Get("page") == "" {
				return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "the GitLab next-page link is invalid")
			}
			value = u.Query().Get("page")
			break
		}
	}
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n-1 != current {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "the GitLab next page does not follow the current one")
	}
	return n, nil
}

func hasCode(err error, code adaptersdk.AdapterErrorCode) bool {
	e, ok := errors.AsType[*adaptersdk.Error](err)
	return ok && e.Code == code
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func numericID(raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n > 0 && id(n) == raw
}

func projectPath(projectID int64) string { return "projects/" + id(projectID) }

func scopeID(scope adaptersdk.ScopeRef) (int64, error) {
	if scope.Kind != adaptersdk.ScopeRefKindRepository {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab scope must be a repository")
	}
	n, ok := numericID(scope.NativeScopeId)
	if !ok {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab repository scope requires the global numeric project ID")
	}
	return n, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
