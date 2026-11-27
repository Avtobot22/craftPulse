package adapterapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestManifestPreservesIssueCount(t *testing.T) {
	t.Parallel()

	caller, err := NewCaller(Config{Timeout: time.Second})
	require.NoError(t, err)
	caller.clients = &http.Client{Transport: publicationRoundTripper(func(*http.Request) (*http.Response, error) {
		body := `{"items":[
			{"record_kind":"report_file","native_ns":"project","native_id":"old","updated_at":"2026-11-25T08:00:00Z"},
			{"record_kind":"report_file","native_ns":"project","native_id":"current","updated_at":"2026-11-25T09:00:00Z","issue_count":0}
		],"total_count":2,"next_cursor":null}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	target := entity.AdapterTarget{Kind: entity.AdapterClient, URL: "https://adapter.example"}
	page, err := caller.Manifest(t.Context(), target, entity.ManifestInput{
		SchemaID: "findings", Scope: entity.ScopeRef{Kind: entity.ScopeAnalysisProject, NativeScopeID: "project"}, Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Nil(t, page.Items[0].IssueCount)
	assert.Equal(t, new(0), page.Items[1].IssueCount)
}
