// Package github integrates repositories and pull requests from GitHub.com.
package github

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Adapter serves repository-scoped operations on GitHub.com.
type Adapter struct {
	adaptersdk.Unimplemented
	client        *adaptersdk.SourceClient
	publicationMu sync.Mutex
	publications  map[publicationKey]chan struct{}
}

// New constructs the GitHub.com adapter with the SDK source client.
func New(client *adaptersdk.SourceClient) *Adapter { return &Adapter{client: client} }

var _ adaptersdk.Source = (*Adapter)(nil)

var githubBinding = adaptersdk.SourceBinding{BaseURL: _baseURL, InstanceKey: _baseURL, Edition: _edition, Version: _apiVersion}

func readToken(secret string) (string, error) {
	var credential struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(secret), &credential); err != nil || strings.TrimSpace(credential.Token) == "" {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitHub credentials require a token")
	}
	return strings.TrimSpace(credential.Token), nil
}

func (a *Adapter) bound(ctx context.Context, call adaptersdk.Call) (source, error) {
	binding, secret, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, err
	}
	if binding != githubBinding {
		return source{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnsupportedEdition, "GitHub.com binding required")
	}
	token, err := readToken(secret)
	return source{client: a.client, token: token}, err
}

type repository struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	FullName      string     `json:"full_name"`
	DefaultBranch string     `json:"default_branch"`
	Visibility    string     `json:"visibility"`
	Archived      bool       `json:"archived"`
	URL           string     `json:"html_url"`
	UpdatedAt     *time.Time `json:"updated_at"`
	Owner         account    `json:"owner"`
}

func (r repository) path() string {
	owner, name, _ := strings.Cut(r.FullName, "/")
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
}

func (r repository) validIdentity() bool {
	owner, name, found := strings.Cut(r.FullName, "/")
	return r.ID > 0 && found && owner != "" && name != "" && !strings.ContainsAny(r.FullName, "?#\\") &&
		!strings.Contains(name, "/") && owner != "." && owner != ".." && name != "." && name != ".."
}

func (s source) repository(ctx context.Context, scope adaptersdk.ScopeRef) (repository, error) {
	n, ok := numeric(scope.NativeScopeId)
	if !ok || scope.Kind != adaptersdk.ScopeRefKindRepository {
		return repository{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitHub scope requires a repository ID")
	}
	var r repository
	err := s.get(ctx, "repository", "/repositories/"+id(n), &r)
	if err == nil && (r.ID != n || !r.validIdentity()) {
		err = adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub repository identity does not match")
	}
	return r, err
}

func manifest(in adaptersdk.FetchInput) *envelope.Manifest {
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(in.SchemaId), SchemaVersion: "1",
		Source: envelope.Source{Provider: envelope.SourceProviderGithub, InstanceKey: _baseURL, Edition: optional(_edition), Version: optional(_apiVersion)},
		Area:   envelope.AreaRef{Kind: envelope.AreaRefKindRepository, NativeId: in.Scope.NativeScopeId},
		Period: envelope.Period{From: in.Period.From, To: in.Period.To}, CreatedAt: time.Now().UTC(),
		Completeness: envelope.ManifestCompletenessComplete,
		Producer:     envelope.Producer{Name: "adapter-github", Version: "1"},
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
	}
	if in.Cursor != nil {
		m.Selection.Cursor = &envelope.Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeId}
	}
	return m
}

func writeRecord(page *adaptersdk.Page, r envelope.Record, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	r.Payload = data
	// Use the canonical content when GitHub has no immutable version for the record.
	hash, err := envelope.ContentHash(&r)
	if err != nil {
		return err
	}
	r.SourceRevision = &hash
	return page.AddRecord(&r)
}

func object(kind, namespace, nativeID string, observed time.Time) envelope.Record {
	return envelope.Record{
		RecordKind:    envelope.RecordRecordKind(kind),
		PayloadSchema: kind + "/1",
		RecordClass:   envelope.RecordRecordClassObject,
		NativeNs:      namespace,
		NativeId:      nativeID,
		ObservedTime:  &observed,
	}
}
