package atlassiancheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const _scopes = "offline_access read:jira-work read:jira-user read:space:confluence " +
	"read:page:confluence read:content-details:confluence"

// Errors deliberately contain neither provider response bodies nor URLs with credentials/codes.
type providerError struct {
	Status int
}

func (e providerError) Error() string { return fmt.Sprintf("Atlassian returned HTTP %d", e.Status) }

type provider struct {
	config Config
	http   *http.Client
}

func (p provider) request(ctx context.Context, method, target, bearer string, body any) (data []byte, headers http.Header, status int, resultErr error) {
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, 0, errors.New("cannot encode Atlassian request")
		}
		input = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, target, input)
	if err != nil {
		return nil, nil, 0, errors.New("cannot construct Atlassian request")
	}
	if r.URL.Scheme != "https" || (r.URL.Host != "api.atlassian.com" && r.URL.Host != "auth.atlassian.com") {
		return nil, nil, 0, errors.New("atlassian endpoint is not allowed")
	}
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := p.http.Do(r)
	if err != nil {
		return nil, nil, 0, errors.New("atlassian transport failed")
	}
	defer func() { _ = response.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, response.Header, response.StatusCode, errors.New("invalid Atlassian response size")
	}
	return b, response.Header, response.StatusCode, nil
}

func (p provider) exchange(ctx context.Context, grant, value string) (token, error) {
	body := map[string]string{
		"grant_type": grant, "client_id": p.config.ClientID, "client_secret": p.config.ClientSecret,
	}
	if grant == "authorization_code" {
		body["code"], body["redirect_uri"] = value, p.config.Callback
	} else {
		body["refresh_token"] = value
	}
	b, _, status, err := p.request(ctx, http.MethodPost, "https://auth.atlassian.com/oauth/token", "", body)
	if err != nil {
		return token{}, err
	}
	if status != http.StatusOK {
		return token{}, providerError{Status: status}
	}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
		Type    string `json:"token_type"`
		Scope   string `json:"scope"`
	}
	if err = json.Unmarshal(b, &result); err != nil || result.Access == "" ||
		result.Refresh == "" || result.Expires <= 0 || !strings.EqualFold(result.Type, "Bearer") {
		return token{}, errors.New("invalid Atlassian token response or offline access missing")
	}
	return token{
		Access: result.Access, Refresh: result.Refresh,
		Expires: time.Now().UTC().Add(time.Duration(result.Expires) * time.Second),
	}, nil
}

func (p provider) account(ctx context.Context, access string) (string, error) {
	b, _, status, err := p.request(ctx, http.MethodGet, p.jira("/myself"), access, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", providerError{Status: status}
	}
	var result struct {
		ID     string `json:"accountId"` //nolint:tagliatelle // Atlassian wire format.
		Active bool   `json:"active"`
	}
	if err = json.Unmarshal(b, &result); err != nil || !result.Active ||
		!_accountID.MatchString(result.ID) || result.ID == "unknown" {
		return "", errors.New("invalid Atlassian identity")
	}
	return result.ID, nil
}

func (p provider) jira(path string) string {
	return "https://api.atlassian.com/ex/jira/" + p.config.CloudID + "/rest/api/3" + path
}

func (p provider) confluence(path string) string {
	return "https://api.atlassian.com/ex/confluence/" + p.config.CloudID + path
}
