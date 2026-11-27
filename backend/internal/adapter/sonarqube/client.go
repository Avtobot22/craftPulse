package sonarqube

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
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_pageSize      = 500
	_issueWindow   = 10000
	_maxSourcePage = 8 << 20
)

type source struct {
	client  *adaptersdk.SourceClient
	binding adaptersdk.SourceBinding
	token   string
}

func normalizeBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" ||
		strings.Contains(raw, "#") {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "SonarQube requires an HTTPS base URL without credentials, query or fragment")
	}
	u.Host = strings.ToLower(u.Host)
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	if base := u.String(); len(base) <= 512 {
		return base, nil
	}
	return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "SonarQube base URL is too long")
}

func tokenFrom(secret string) (string, error) {
	var credential struct {
		Token string `json:"token"`
	}
	err := json.Unmarshal([]byte(secret), &credential)
	token := strings.TrimSpace(credential.Token)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "SonarQube credentials require a token")
	}
	return token, nil
}

func (s source) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	address := s.binding.BaseURL + endpoint
	if len(query) != 0 {
		address += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cannot construct SonarQube request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	start := time.Now()
	resp, err := s.client.DoDirect(req, _maxSourcePage) //nolint:bodyclose // DecodeJSON closes the body.
	status := 0
	if err == nil {
		status = resp.StatusCode
		err = adaptersdk.DecodeJSON(resp, out)
	}
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok && e.SourceStatusCode() != 0 {
		status = e.SourceStatusCode()
	}
	adaptersdk.Call{}.Log(ctx).Debug().Str("source_operation", endpoint).
		Int("source_status", status).Dur("elapsed", time.Since(start)).Msg("SonarQube request")
	if status == http.StatusForbidden {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodePermissionMissing, "SonarQube project browse permission is missing")
	}
	return err
}

//nolint:tagliatelle // SonarQube Web API fields.
type paging struct {
	PageIndex int  `json:"pageIndex"`
	PageSize  int  `json:"pageSize"`
	Total     *int `json:"total"`
}

func (p paging) check(index, size, count, total int) error {
	if p.Total == nil || *p.Total < 0 || p.PageIndex != index || p.PageSize != size ||
		total >= 0 && *p.Total != total {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube paging metadata is missing or changed")
	}
	expected := min(size, max(0, *p.Total-(index-1)*size))
	if count != expected {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube page is incomplete")
	}
	return nil
}

func pageQuery(field, project string, index, size int) url.Values {
	return url.Values{field: {project}, "p": {strconv.Itoa(index)}, "ps": {strconv.Itoa(size)}}
}

func pageError(err error, index int) error {
	if index <= 1 {
		return err
	}
	missing := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	if sourceErr, ok := errors.AsType[*adaptersdk.Error](err); ok {
		switch sourceErr.SourceStatusCode() {
		case http.StatusNotFound, http.StatusGone, http.StatusBadRequest, http.StatusRequestedRangeNotSatisfiable:
			missing = true
		}
	}
	if missing {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube next page is unavailable")
	}
	return err
}

func hasCode(err error, code adaptersdk.AdapterErrorCode) bool {
	e, ok := errors.AsType[*adaptersdk.Error](err)
	return ok && e.Code == code
}

func textFits(value string, limit int) bool {
	return utf8.RuneCountInString(value) <= limit
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
