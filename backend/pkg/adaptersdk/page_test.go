package adaptersdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _adapterMemoryLimit = 48 << 20

func taskManifest() *envelope.Manifest {
	return &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaIdTasks, SchemaVersion: "1",
		Source:       envelope.Source{Provider: envelope.SourceProviderJiraDc, InstanceKey: "jira.example.test"},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindTrackerProject, NativeId: "10000"},
		Completeness: envelope.ManifestCompletenessComplete,
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		Producer:     envelope.Producer{Name: "adaptersdk-test", Version: "1"},
		CreatedAt:    time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
	}
}

// bigTask is a valid task/1 record of about 6 KiB: six source fields of a thousand characters.
func bigTask(t *testing.T, i int) *envelope.Record {
	t.Helper()
	raw := make(map[string]string, 6)
	for f := range 6 {
		raw["customfield_"+strconv.Itoa(10000+f)] = strings.Repeat("r"+strconv.Itoa(i)+"f"+strconv.Itoa(f)+" ", 1000)[:1000]
	}
	payload, err := json.Marshal(map[string]any{
		"project_native_id": "10000", "task_key": "PA-" + strconv.Itoa(i), "title": "Task " + strconv.Itoa(i),
		"type_native_id": "10001", "status_native_id": "3", "created_at": "2026-02-02T09:00:00+03:00", "raw_fields": raw,
	})
	require.NoError(t, err)
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassObject, RecordKind: envelope.RecordRecordKindTask,
		PayloadSchema: "task/1", NativeId: strconv.Itoa(20000 + i), Payload: payload,
	}
}

func heapBytes() uint64 {
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(s)
	return s[0].Value.Uint64()
}

// The test measures the process heap, so it does not run in parallel with others.
func TestFetch_StreamsLargePageWithinAdapterMemory(t *testing.T) {
	const records = 10_000
	var rawBytes int
	src := &fakeSource{fetch: func(_ context.Context, _ adaptersdk.Call, _ adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
		for i := range records {
			rec := bigTask(t, i)
			line, _ := json.Marshal(rec)
			rawBytes += len(line) + 1
			if err := page.AddRecord(rec); err != nil {
				return nil, err
			}
		}
		return taskManifest(), nil
	}}
	h := newHarness(t, adaptersdk.NewStore(nil, nil), src)
	out := filepath.Join(t.TempDir(), "page.tar.gz")

	previous := debug.SetMemoryLimit(_adapterMemoryLimit)
	t.Cleanup(func() { debug.SetMemoryLimit(previous) })
	runtime.GC()
	base := heapBytes()
	stop, peakc := make(chan struct{}), make(chan uint64, 1)
	go func() {
		var peak uint64
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				peakc <- peak
				return
			case <-tick.C:
				peak = max(peak, heapBytes())
			}
		}
	}()

	body, err := json.Marshal(map[string]any{
		"schema_id": "tasks", "scope": map[string]any{"kind": "tracker_project", "native_scope_id": "10000"},
		"cursor": nil, "period": map[string]any{"from": nil, "to": nil}, "limit": 1000, "object_keys": []any{},
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+"/v1/connections/"+h.conn.String()+"/fetch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+_serviceKey)
	req.Header.Set("X-Craftpulse-Organization", h.org.String())
	req.Header.Set("X-Craftpulse-Contract-Version", "1")
	req.Header.Set("X-Correlation-Id", "corr-page")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	f, err := os.Create(out)
	require.NoError(t, err)
	_, err = io.Copy(f, resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.NoError(t, f.Close())
	close(stop)
	peak := <-peakc

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/gzip", resp.Header.Get("Content-Type"))
	assert.Greater(t, rawBytes, _adapterMemoryLimit, "the page is larger than the adapter memory")
	t.Logf("page %d MiB, heap peak %d MiB, base %d MiB", rawBytes>>20, peak>>20, base>>20)
	assert.Less(t, peak, uint64(_adapterMemoryLimit))

	pack, err := os.Open(out)
	require.NoError(t, err)
	defer func() { _ = pack.Close() }()
	result := envelope.Check(pack, envelope.DefaultLimits)
	require.Nil(t, result.Envelope)
	assert.Len(t, result.Records, records)
	assert.Zero(t, result.Rejected())

	entries, err := os.ReadDir(h.spool)
	require.NoError(t, err)
	assert.Empty(t, entries, "the spooled page is removed")
}

func TestFetch_PreparingDefersWithoutAttempt(t *testing.T) {
	t.Parallel()
	retry := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
	src := &fakeSource{fetch: func(context.Context, adaptersdk.Call, adaptersdk.FetchInput, *adaptersdk.Page) (*envelope.Manifest, error) {
		return nil, adaptersdk.Preparing(retry, "partial clone in progress")
	}}
	h := newHarness(t, adaptersdk.NewStore(nil, nil), src)
	resp, body := h.call(http.MethodPost, "/fetch", map[string]any{
		"schema_id": "tasks", "scope": map[string]any{"kind": "tracker_project", "native_scope_id": "10000"},
		"cursor": nil, "period": map[string]any{"from": nil, "to": nil}, "limit": 10, "object_keys": []any{},
	})
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	e := adapterError(t, body)
	assert.Equal(t, adaptersdk.AdapterErrorCodePreparing, e.Code)
	require.NotNil(t, e.RetryAt)
	assert.True(t, retry.Equal(*e.RetryAt))
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
}
