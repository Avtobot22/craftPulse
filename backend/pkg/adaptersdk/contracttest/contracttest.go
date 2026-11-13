// Package contracttest checks an adapter at an address against adapter.yaml (TZ 11): status codes and
// response schemas of every call, the cursor and the end of a selection, a repeated DeleteCredentials
// and that no response carries the credentials it was given. Every built-in adapter runs it in its tests.
package contracttest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _defaultMaxPages = 50

var _maxBody = envelope.DefaultLimits.MaxPackageBytes + 1

// Target is the adapter under test and the inputs it is called with.
type Target struct {
	// BaseURL is the root of the adapter, without /v1.
	BaseURL    string
	ServiceKey string
	// Client reaches the adapter; nil is http.DefaultClient.
	Client       *http.Client
	Organization string
	Connection   string
	// Secret is set as the collect credentials; no response of the adapter may contain it.
	Secret string
	Probe  adaptersdk.ProbeInput
	// Fetch is the first page; the suite follows next_cursor until the end of the selection.
	Fetch    adaptersdk.FetchInput
	Manifest adaptersdk.ManifestInput
	// CheckAccess is skipped when nil.
	CheckAccess *adaptersdk.CheckAccessInput
	// MaxPages bounds the pages followed; zero is 50.
	MaxPages int
}

// Violation is one breach of the contract found by a step of the suite.
type Violation struct {
	Step   string
	Detail string
}

func (v Violation) Error() string { return v.Step + ": " + v.Detail }

// Run reports every violation of the target as an error of t.
func Run(t *testing.T, target Target) {
	t.Helper()
	for _, v := range Check(t.Context(), target) {
		t.Error(v)
	}
}

var _spec = sync.OnceValues(GetSpec)

// Check calls the adapter and returns the violations of the contract; none means it passed.
func Check(ctx context.Context, target Target) []Violation {
	doc, err := _spec()
	if err != nil {
		return []Violation{{Step: "spec", Detail: err.Error()}}
	}
	s := &suite{ctx: ctx, t: target, doc: doc}
	if s.t.Client == nil {
		s.t.Client = http.DefaultClient
	}
	if s.t.MaxPages == 0 {
		s.t.MaxPages = _defaultMaxPages
	}
	s.serviceKey()
	s.contractVersion()
	s.credentials()
	s.probe()
	s.fetch()
	s.manifest()
	s.checkAccess()
	s.deleteCredentials()
	return s.violations
}

type suite struct {
	ctx        context.Context
	t          Target
	doc        *openapi3.T
	violations []Violation
}

func (s *suite) fail(step, format string, args ...any) {
	s.violations = append(s.violations, Violation{Step: step, Detail: fmt.Sprintf(format, args...)})
}

type answer struct {
	status int
	header http.Header
	body   []byte
}

// call sends one operation and checks what every answer must satisfy: a declared status, the schema of
// a JSON body and no credentials in the body or the headers.
func (s *suite) call(step, method, operation, query string, body any, edit func(*http.Request)) (answer, bool) {
	path := "/connections/{connection_id}/" + operation
	var r io.Reader = http.NoBody
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			s.fail(step, "encode request: %v", err)
			return answer{}, false
		}
		r = bytes.NewReader(data)
	}
	url := strings.TrimSuffix(s.t.BaseURL, "/") + "/v1" + strings.Replace(path, "{connection_id}", s.t.Connection, 1)
	if query != "" {
		url += "?" + query
	}
	req, err := http.NewRequestWithContext(s.ctx, method, url, r)
	if err != nil {
		s.fail(step, "build request: %v", err)
		return answer{}, false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.t.ServiceKey)
	req.Header.Set("X-Craftpulse-Organization", s.t.Organization)
	req.Header.Set("X-Craftpulse-Contract-Version", adaptersdk.CurrentContractVersion)
	req.Header.Set("X-Correlation-Id", "contracttest-"+strings.ReplaceAll(step, " ", "-"))
	if edit != nil {
		edit(req)
	}
	resp, err := s.t.Client.Do(req)
	if err != nil {
		s.fail(step, "call failed: %v", err)
		return answer{}, false
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, _maxBody))
	if err != nil {
		s.fail(step, "read answer: %v", err)
		return answer{}, false
	}
	a := answer{status: resp.StatusCode, header: resp.Header, body: data}
	s.noSecret(step, a)
	s.validate(step, method, path, req, a)
	return a, true
}

func (s *suite) noSecret(step string, a answer) {
	if s.t.Secret == "" {
		return
	}
	if bytes.Contains(a.body, []byte(s.t.Secret)) {
		s.fail(step, "the answer contains the credentials")
	}
	for name, values := range a.header {
		if slices.ContainsFunc(values, func(v string) bool { return strings.Contains(v, s.t.Secret) }) {
			s.fail(step, "header %s contains the credentials", name)
		}
	}
	if gz, err := gzip.NewReader(bytes.NewReader(a.body)); err == nil {
		plain, _ := io.ReadAll(io.LimitReader(gz, envelope.DefaultLimits.MaxUnpackedBytes))
		if bytes.Contains(plain, []byte(s.t.Secret)) {
			s.fail(step, "the package contains the credentials")
		}
	}
}

func (s *suite) validate(step, method, path string, req *http.Request, a answer) {
	item := s.doc.Paths.Find(path)
	if item == nil {
		s.fail(step, "path %s is not in adapter.yaml", path)
		return
	}
	op := item.GetOperation(method)
	route := &routers.Route{Spec: s.doc, Path: path, PathItem: item, Method: method, Operation: op}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, Route: route},
		Status:                 a.status,
		Header:                 a.header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	if strings.HasPrefix(a.header.Get("Content-Type"), "application/gzip") {
		in.Options.ExcludeResponseBody = true
	}
	in.SetBodyBytes(a.body)
	if err := openapi3filter.ValidateResponse(s.ctx, in); err != nil {
		s.fail(step, "%s %d does not match adapter.yaml: %s", op.OperationID, a.status, brief(err))
	}
}

// brief keeps the first line of each validation error; kin-openapi appends the whole schema.
func brief(err error) string {
	var parts []string
	if me, ok := errors.AsType[openapi3.MultiError](err); ok {
		for _, e := range me {
			parts = append(parts, brief(e))
		}
		return strings.Join(parts, "; ")
	}
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}

func (s *suite) code(step string, a answer, wantStatus int, want adaptersdk.AdapterErrorCode) {
	var e adaptersdk.AdapterError
	if a.status != wantStatus || json.Unmarshal(a.body, &e) != nil || e.Code != want {
		s.fail(step, "want %d %s, got %d %s", wantStatus, want, a.status, bytes.TrimSpace(a.body))
	}
}

func (s *suite) serviceKey() {
	const step = "service key"
	a, ok := s.call(step, http.MethodPost, "probe", "", s.t.Probe, func(r *http.Request) { r.Header.Del("Authorization") })
	if ok {
		s.code(step, a, http.StatusUnauthorized, adaptersdk.AdapterErrorCodeUnauthorized)
	}
	a, ok = s.call(step, http.MethodPost, "probe", "", s.t.Probe, func(r *http.Request) { r.Header.Set("Authorization", "Bearer x"+s.t.ServiceKey) })
	if ok {
		s.code(step, a, http.StatusUnauthorized, adaptersdk.AdapterErrorCodeUnauthorized)
	}
}

func (s *suite) contractVersion() {
	const step = "contract version"
	a, ok := s.call(step, http.MethodPost, "probe", "", s.t.Probe, func(r *http.Request) { r.Header.Set("X-Craftpulse-Contract-Version", "999") })
	if ok {
		s.code(step, a, http.StatusUnprocessableEntity, adaptersdk.AdapterErrorCodeUnsupportedContractVersion)
	}
}

func (s *suite) credentials() {
	const step = "put credentials"
	a, ok := s.call(step, http.MethodPut, "credentials", "", map[string]any{
		"purpose": "collect", "secret": s.t.Secret, "user_id": nil, "oauth_code": nil, "redirect_uri": nil,
	}, nil)
	if ok && (a.status != http.StatusNoContent || len(a.body) > 0) {
		s.fail(step, "want 204 without a body, got %d", a.status)
	}
}

func (s *suite) probe() {
	const step = "probe"
	a, ok := s.call(step, http.MethodPost, "probe", "", s.t.Probe, nil)
	if !ok {
		return
	}
	var res adaptersdk.ProbeResult
	if a.status != http.StatusOK || json.Unmarshal(a.body, &res) != nil {
		s.fail(step, "want 200 with a result, got %d", a.status)
		return
	}
	if !slices.Contains(res.ContractVersions, adaptersdk.CurrentContractVersion) {
		s.fail(step, "contract_versions %v has no version %s", res.ContractVersions, adaptersdk.CurrentContractVersion)
	}
}

func (s *suite) fetch() {
	in := s.t.Fetch
	in.Cursor = nil
	for page := 1; ; page++ {
		step := fmt.Sprintf("fetch page %d", page)
		if page > s.t.MaxPages {
			s.fail(step, "the selection did not end in %d pages", s.t.MaxPages)
			return
		}
		a, ok := s.call(step, http.MethodPost, "fetch", "", in, nil)
		if !ok {
			return
		}
		if a.status != http.StatusOK || !strings.HasPrefix(a.header.Get("Content-Type"), "application/gzip") {
			s.fail(step, "want 200 application/gzip, got %d %s", a.status, a.header.Get("Content-Type"))
			return
		}
		res := envelope.Check(bytes.NewReader(a.body), envelope.DefaultLimits)
		if res.Envelope != nil {
			s.fail(step, "the package is rejected: %s", res.Envelope.Error())
			return
		}
		for _, r := range res.Records {
			if r.Rejection != nil {
				s.fail(step, "record %d is rejected: %s", r.No, r.Rejection.Error())
			}
		}
		m := res.Manifest
		if string(m.SchemaId) != string(in.SchemaId) {
			s.fail(step, "manifest schema_id %s, want %s", m.SchemaId, in.SchemaId)
		}
		sel := m.Selection
		if !sameCursor(sel.Cursor, in.Cursor) {
			s.fail(step, "selection.cursor is not the cursor of the request")
		}
		if sel.End {
			if sel.NextCursor != nil {
				s.fail(step, "the end of the selection has a next_cursor")
			}
			return
		}
		if sel.NextCursor == nil {
			s.fail(step, "next_cursor is null before the end of the selection")
			return
		}
		if sameCursor(sel.NextCursor, in.Cursor) {
			s.fail(step, "next_cursor does not move")
			return
		}
		in.Cursor = &adaptersdk.Cursor{ModifiedAt: sel.NextCursor.ModifiedAt, NativeId: sel.NextCursor.NativeId}
	}
}

func sameCursor(got *envelope.Cursor, want *adaptersdk.Cursor) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return got.NativeId == want.NativeId && got.ModifiedAt.Equal(want.ModifiedAt)
}

func (s *suite) manifest() {
	in := s.t.Manifest
	in.Cursor = nil
	for page := 1; ; page++ {
		step := fmt.Sprintf("manifest page %d", page)
		if page > s.t.MaxPages {
			s.fail(step, "the manifest did not end in %d pages", s.t.MaxPages)
			return
		}
		a, ok := s.call(step, http.MethodPost, "manifest", "", in, nil)
		if !ok {
			return
		}
		var res adaptersdk.ManifestPage
		if a.status != http.StatusOK || json.Unmarshal(a.body, &res) != nil {
			s.fail(step, "want 200 with a page, got %d", a.status)
			return
		}
		if res.NextCursor == nil {
			return
		}
		if in.Cursor != nil && *res.NextCursor == *in.Cursor {
			s.fail(step, "next_cursor does not move")
			return
		}
		in.Cursor = res.NextCursor
	}
}

func (s *suite) checkAccess() {
	if s.t.CheckAccess == nil {
		return
	}
	const step = "check access"
	a, ok := s.call(step, http.MethodPost, "check-access", "", s.t.CheckAccess, nil)
	if !ok {
		return
	}
	var res adaptersdk.CheckAccessResult
	if a.status != http.StatusOK || json.Unmarshal(a.body, &res) != nil {
		s.fail(step, "want 200 with decisions, got %d", a.status)
		return
	}
	if len(res.Decisions) != len(s.t.CheckAccess.Items) {
		s.fail(step, "%d decisions for %d items", len(res.Decisions), len(s.t.CheckAccess.Items))
	}
}

func (s *suite) deleteCredentials() {
	for _, step := range []string{"delete credentials", "delete credentials again"} {
		a, ok := s.call(step, http.MethodDelete, "credentials", "purpose=collect", nil, nil)
		if ok && a.status != http.StatusNoContent {
			s.fail(step, "want 204, got %d", a.status)
		}
	}
}
