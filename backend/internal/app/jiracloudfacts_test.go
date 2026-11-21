package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestJiraCloudAdapterPagesAndReplay(t *testing.T) {
	t.Parallel()
	e := newJiraTestEnv(t)
	e.credentials(t)
	e.probe(t)

	in := e.target.Fetch
	p, res := e.fetch(t, in)
	first := jiraRecord(t, p, res, "task", "10002")
	require.NotNil(t, p.Manifest.Selection.NextCursor)
	in.Cursor = jiraCursor(p.Manifest.Selection.NextCursor)
	p, res = e.fetch(t, in)
	long := jiraRecord(t, p, res, "task", "10010")
	require.NotNil(t, long.SourceRevision)
	hash, err := envelope.ContentHash(long)
	require.NoError(t, err)
	assert.Equal(t, hash, *long.SourceRevision)
	var task envelope.TaskV1
	require.NoError(t, json.Unmarshal(long.Payload, &task))
	assert.Equal(t, "2026-11-21T03:00:00.123Z", task.UpdatedAt.UTC().Format(time.RFC3339Nano))
	assert.Equal(t, "10100", *task.Restriction)
	assert.Equal(t, "account-fixture-1", *task.AssigneeAccountNativeId)
	assert.Equal(t, "outward/10000/10002", *(*task.RawFields)["issuelink/10011"])
	assert.Equal(t, "10021", *(*task.RawFields)["customfield_10020"])
	previous := jiraCursor(p.Manifest.Selection.NextCursor)
	in.Cursor = previous
	p, res = e.fetch(t, in)
	assert.True(t, p.Manifest.Selection.End)
	assert.Empty(t, res.Records)

	items := make(map[string]time.Time)
	manifest := e.target.Manifest
	for range 3 {
		status, _, body := e.call(t, http.MethodPost, "manifest", manifest)
		require.Equal(t, http.StatusOK, status, string(body))
		var out adaptersdk.ManifestPage
		require.NoError(t, json.Unmarshal(body, &out))
		for _, item := range out.Items {
			require.NotNil(t, item.UpdatedAt)
			items[item.NativeId] = *item.UpdatedAt
		}
		manifest.Cursor = out.NextCursor
	}
	require.Len(t, items, 2)
	assert.True(t, task.UpdatedAt.Equal(items[long.NativeId]))
	period := manifest
	period.Period.To = task.UpdatedAt
	status, _, body := e.call(t, http.MethodPost, "manifest", period)
	require.Equal(t, http.StatusOK, status, string(body))
	var out adaptersdk.ManifestPage
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Empty(t, out.Items)

	p, res = e.fetch(t, e.target.Fetch)
	second := jiraRecord(t, p, res, "task", first.NativeId)
	firstHash, err := envelope.ContentHash(first)
	require.NoError(t, err)
	secondHash, err := envelope.ContentHash(second)
	require.NoError(t, err)
	assert.Equal(t, first.SourceRevision, second.SourceRevision)
	assert.Equal(t, firstHash, secondHash)
	for _, kind := range []string{"task_status", "task_type", "task_resolution", "task_field"} {
		require.NotEmpty(t, jiraKinds(res, kind))
	}

	e.fixture.updated.Store(true)
	in.Limit = 2
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: previous.ModifiedAt.Add(-10 * time.Minute)}
	p, res = e.fetch(t, in)
	assert.Equal(t, []string{"10010", "10002"}, jiraKinds(res, "task"))
	assert.Equal(t, "10002", p.Manifest.Selection.NextCursor.NativeId)
	changed := jiraRecord(t, p, res, "task", "10002")
	assert.NotEqual(t, first.SourceRevision, changed.SourceRevision)
}

func TestJiraCloudAdapterLongHistory(t *testing.T) {
	t.Parallel()
	e := newJiraTestEnv(t)
	e.credentials(t)
	e.probe(t)

	in := e.target.Fetch
	in.SchemaId, in.Limit = adaptersdk.FetchInputSchemaIdTaskEvents, 2
	p, res := e.fetch(t, in)
	require.Len(t, res.Records, 102)
	require.Len(t, p.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "10010", p.Manifest.Selection.Truncated[0].NativeId)
	assert.Equal(t, envelope.ManifestCompletenessPartial, p.Manifest.Completeness)
	step := jiraRecord(t, p, res, "task_event", "30000/status")
	assert.Equal(t, "10010", step.NativeNs)
	assert.Equal(t, "30000", *step.SourceRevision)
	_, offset := step.EventTime.Zone()
	assert.Equal(t, 7*60*60, offset)
	var event envelope.TaskEventV1
	require.NoError(t, json.Unmarshal(step.Payload, &event))
	assert.Equal(t, "3", *event.AfterValue)
	assert.Equal(t, "account-fixture-1", *event.ActorAccountNativeId)

	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindTask, NativeId: "10010"}}
	p, res = e.fetch(t, in)
	require.Len(t, res.Records, 101)
	require.NotNil(t, p.Manifest.Selection.NextCursor)
	assert.Empty(t, p.Manifest.Selection.Truncated)
	in.Cursor = jiraCursor(p.Manifest.Selection.NextCursor)
	p, res = e.fetch(t, in)
	require.Len(t, res.Records, 22)
	assert.True(t, p.Manifest.Selection.End)
	assert.Empty(t, p.Manifest.Selection.Truncated)
	assert.Equal(t, envelope.ManifestCompletenessComplete, p.Manifest.Completeness)
	tail := jiraRecord(t, p, res, "task_event", "30121/customfield_10020")
	require.NoError(t, json.Unmarshal(tail.Payload, &event))
	assert.Equal(t, "10022", *event.AfterValue)
}

func TestJiraCloudAdapterAccountIdentity(t *testing.T) {
	t.Parallel()
	e := newJiraTestEnv(t)
	e.credentials(t)
	e.probe(t)
	in := e.target.Fetch
	in.SchemaId = adaptersdk.FetchInputSchemaIdAccounts
	p, res := e.fetch(t, in)
	first := jiraRecord(t, p, res, "account", "account-fixture-1")
	e.fixture.renamed.Store(true)
	p, res = e.fetch(t, in)
	second := jiraRecord(t, p, res, "account", "account-fixture-1")
	assert.JSONEq(t, `{"account_kind":"human","active":true,"display_name":"Renamed user","login":null}`, string(second.Payload))
	assert.Equal(t, first.NativeId, second.NativeId)
	assert.NotEqual(t, first.Payload, second.Payload)
}

func jiraRecord(t *testing.T, p *envelope.Package, res *envelope.Result, kind, id string) *envelope.Record {
	t.Helper()
	for _, r := range res.Records {
		if r.Kind == kind && r.NativeID == id {
			record, ok, err := p.Record(r.No)
			require.NoError(t, err)
			require.True(t, ok)
			return record
		}
	}
	t.Fatalf("record %s/%s missing", kind, id)
	return nil
}

func jiraKinds(res *envelope.Result, kind string) []string {
	out := make([]string, 0, len(res.Records))
	for _, r := range res.Records {
		if r.Kind == kind {
			out = append(out, r.NativeID)
		}
	}
	return out
}

func jiraCursor(cursor *envelope.Cursor) *adaptersdk.Cursor {
	return &adaptersdk.Cursor{ModifiedAt: cursor.ModifiedAt, NativeId: cursor.NativeId}
}
