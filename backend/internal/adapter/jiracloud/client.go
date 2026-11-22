package jiracloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_api              = "/rest/api/3/"
	_oauthGateway     = "https://api.atlassian.com/ex/jira/"
	_maxResponseBytes = 2 << 20
	_pageSize         = 25
)

type credentials struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type source struct {
	client  *adaptersdk.SourceClient
	binding adaptersdk.SourceBinding
	creds   credentials
}

func newSource(client *adaptersdk.SourceClient, binding adaptersdk.SourceBinding, secret string) (source, error) {
	var creds credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil ||
		strings.TrimSpace(creds.Email) == "" || strings.TrimSpace(creds.Token) == "" {
		return source{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "Jira Cloud collect credentials require email and API token")
	}
	return source{client: client, binding: binding, creds: creds}, nil
}

func normalizeBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return "", invalidRequest("Jira base_url must be an HTTPS address without credentials, query or fragment")
	}
	u.Host = strings.ToLower(u.Host)
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String(), nil
}

func (s source) request(ctx context.Context, operation, path string, query url.Values, authenticated bool) (*http.Response, error) {
	address := s.binding.BaseURL + path
	if len(query) != 0 {
		address += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return nil, invalidRequest("cannot construct Jira request")
	}
	req.Header.Set("Accept", "application/json")
	if authenticated {
		req.SetBasicAuth(s.creds.Email, s.creds.Token)
	}
	return doRequest(ctx, s.client, operation, req)
}

func doRequest(ctx context.Context, client *adaptersdk.SourceClient, operation string, req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := client.DoDirect(req, _maxResponseBytes)
	log := adaptersdk.Call{}.Log(ctx).Debug().Str("source_operation", operation).Dur("elapsed", time.Since(start))
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		log.Str("reason", string(e.Code))
	}
	if resp != nil {
		log.Int("status", resp.StatusCode)
	}
	log.Msg("Jira request")
	return resp, err
}

func (s source) get(ctx context.Context, operation, path string, query url.Values, out any) error {
	resp, err := s.request(ctx, operation, _api+path, query, true) //nolint:bodyclose // DecodeJSON closes the body
	if err != nil {
		return err
	}
	return adaptersdk.DecodeJSON(resp, out)
}

func invalidRequest(detail string) *adaptersdk.Error {
	return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, detail)
}

func invalidResponse(detail string) *adaptersdk.Error {
	return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, detail)
}

func hasCode(err error, code adaptersdk.AdapterErrorCode) bool {
	e, ok := errors.AsType[*adaptersdk.Error](err)
	return ok && e.Code == code
}

func hidden(err error) bool {
	return hasCode(err, adaptersdk.AdapterErrorCodeAuth) || hasCode(err, adaptersdk.AdapterErrorCodeNotFound)
}

func numericID(raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == raw
}

func projectScope(scope adaptersdk.ScopeRef) (string, error) {
	_, numeric := numericID(scope.NativeScopeId)
	if scope.Kind != adaptersdk.ScopeRefKindTrackerProject || !numeric {
		return "", invalidRequest("Jira scope must be a tracker_project with a numeric project ID")
	}
	return scope.NativeScopeId, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
