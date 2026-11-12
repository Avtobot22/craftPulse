package envelope_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var _schemaIDs = []envelope.ManifestSchemaId{
	envelope.ManifestSchemaIdTasks, envelope.ManifestSchemaIdTaskEvents, envelope.ManifestSchemaIdRelations,
	envelope.ManifestSchemaIdAccounts, envelope.ManifestSchemaIdMergeRequests, envelope.ManifestSchemaIdCiRuns,
	envelope.ManifestSchemaIdFindings, envelope.ManifestSchemaIdJunit, envelope.ManifestSchemaIdCoverage,
	envelope.ManifestSchemaIdDocuments,
}

func manifest(schemaID envelope.ManifestSchemaId, snapshot bool) *envelope.Manifest {
	readID := "read-1"
	return &envelope.Manifest{
		SchemaId: schemaID, SchemaVersion: "1",
		Source:       envelope.Source{Provider: envelope.SourceProviderUniversal, InstanceKey: "import.example.test"},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindTrackerProject, NativeId: "IMP"},
		Completeness: envelope.ManifestCompletenessComplete,
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		Snapshot:     snapshot, ReadId: &readID,
		Producer:  envelope.Producer{Name: "envelope-test", Version: "1"},
		CreatedAt: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
	}
}

// exampleRecord builds a record of the kind from the schema example in its first allowed class.
func exampleRecord(t *testing.T, w *envelope.Writer, schema envelope.RecordSchema) *envelope.Record {
	t.Helper()
	rec := &envelope.Record{
		RecordClass: envelope.RecordRecordClass(schema.Classes[0]), RecordKind: envelope.RecordRecordKind(schema.Kind),
		PayloadSchema: schema.Kind + "/" + schema.Version, NativeId: "id-" + schema.Kind, Payload: schema.Example,
	}
	switch rec.RecordClass {
	case envelope.RecordRecordClassEvent:
		at := time.Date(2026, 3, 2, 12, 0, 0, 0, time.FixedZone("", 3*3600))
		rec.EventTime = &at
	case envelope.RecordRecordClassFile:
		ref, err := w.AddFile("files/"+schema.Kind+"/report.xml", "application/xml", []byte("<testsuites/>"))
		require.NoError(t, err)
		rec.File = &ref
	case envelope.RecordRecordClassObject, envelope.RecordRecordClassSnapshot:
	}
	return rec
}

func build(t *testing.T, m *envelope.Manifest, add func(w *envelope.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	add(w)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func schemaOf(t *testing.T, kind string) envelope.RecordSchema {
	t.Helper()
	for _, s := range envelope.RecordSchemas() {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no record schema %s", kind)
	return envelope.RecordSchema{}
}

func check(data []byte) *envelope.Result {
	return envelope.Check(bytes.NewReader(data), envelope.DefaultLimits)
}

func TestCheck_EveryDataKindPasses(t *testing.T) {
	t.Parallel()
	for _, id := range _schemaIDs {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			kinds := envelope.PackageKinds(string(id))
			require.NotEmpty(t, kinds)
			data := build(t, manifest(id, false), func(w *envelope.Writer) {
				for _, kind := range kinds {
					require.NoError(t, w.AddRecord(exampleRecord(t, w, schemaOf(t, kind))))
				}
			})
			result := check(data)
			require.Nil(t, result.Envelope)
			require.Len(t, result.Records, len(kinds))
			for _, rec := range result.Records {
				assert.Nil(t, rec.Rejection, "%s: %v", rec.Kind, rec.Rejection)
				assert.Len(t, rec.Hash, 64)
			}
		})
	}
}

func TestCheck_RejectsEnvelope(t *testing.T) {
	t.Parallel()
	goodManifest, err := json.Marshal(manifest(envelope.ManifestSchemaIdTasks, false))
	require.NoError(t, err)
	var otherVersion map[string]any
	require.NoError(t, json.Unmarshal(goodManifest, &otherVersion))
	otherVersion["schema_version"] = "2"
	badVersion, err := json.Marshal(otherVersion)
	require.NoError(t, err)
	tests := []struct {
		name    string
		give    []byte
		limits  envelope.Limits
		wantErr string
	}{
		{name: "not gzip", give: []byte("plain text"), wantErr: envelope.ReasonInvalidArchive},
		{name: "records before manifest", give: archive(t, file(envelope.RecordsName, "{}\n"), file(envelope.ManifestName, string(goodManifest))), wantErr: envelope.ReasonManifestNotFirst},
		{name: "no manifest", give: archive(t, file(envelope.RecordsName, "")), wantErr: envelope.ReasonManifestNotFirst},
		{name: "unsupported schema version", give: archive(t, file(envelope.ManifestName, string(badVersion))), wantErr: envelope.ReasonUnsupportedSchemaVersion},
		{name: "invalid manifest", give: archive(t, file(envelope.ManifestName, `{"schema_id":"tasks"}`)), wantErr: envelope.ReasonInvalidManifest},
		{name: "parent path", give: archive(t, file(envelope.ManifestName, string(goodManifest)), file("files/../../etc/passwd", "x")), wantErr: envelope.ReasonUnsafePath},
		{name: "absolute path", give: archive(t, file(envelope.ManifestName, string(goodManifest)), file("/files/a", "x")), wantErr: envelope.ReasonUnsafePath},
		{name: "symlink", give: archive(t, file(envelope.ManifestName, string(goodManifest)), symlink("files/link", "/etc/passwd")), wantErr: envelope.ReasonUnsafePath},
		{name: "unexpected entry", give: archive(t, file(envelope.ManifestName, string(goodManifest)), file("notes.txt", "x")), wantErr: envelope.ReasonUnexpectedEntry},
		{name: "csv outside snapshot", give: archive(t, file(envelope.ManifestName, string(goodManifest)), file(envelope.TasksCSVName, "native_id\nA\n")), wantErr: envelope.ReasonUnexpectedEntry},
		{name: "too many files", give: archive(t, file(envelope.ManifestName, string(goodManifest)), file("files/a", "1"), file("files/b", "2")), limits: envelope.Limits{MaxPackageBytes: 1 << 20, MaxUnpackedBytes: 1 << 20, MaxFiles: 2}, wantErr: envelope.ReasonLimitExceeded},
		{name: "packed size", give: archive(t, file(envelope.ManifestName, string(goodManifest))), limits: envelope.Limits{MaxPackageBytes: 64, MaxUnpackedBytes: 1 << 20, MaxFiles: 10}, wantErr: envelope.ReasonLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limits := tt.limits
			if limits == (envelope.Limits{}) {
				limits = envelope.DefaultLimits
			}
			result := envelope.Check(bytes.NewReader(tt.give), limits)
			require.NotNil(t, result.Envelope)
			assert.Equal(t, tt.wantErr, result.Envelope.Code, result.Envelope.Detail)
			assert.Empty(t, result.Records, "a rejected package reports no records")
		})
	}
}

func TestCheck_RejectsRecordsSeparately(t *testing.T) {
	t.Parallel()
	event := schemaOf(t, "task_event")
	data := build(t, manifest(envelope.ManifestSchemaIdTaskEvents, false), func(w *envelope.Writer) {
		good := exampleRecord(t, w, event)
		noID := exampleRecord(t, w, event)
		noID.NativeId = ""
		noTime := exampleRecord(t, w, event)
		noTime.EventTime = nil
		wrongKind := exampleRecord(t, w, schemaOf(t, "task"))
		badPayload := exampleRecord(t, w, event)
		badPayload.Payload = json.RawMessage(`{"task_native_id":"1","event_kind":"field_change"}`)
		for _, rec := range []*envelope.Record{good, noID, noTime, wrongKind, badPayload} {
			require.NoError(t, w.AddRecord(rec))
		}
	})
	result := check(data)
	require.Nil(t, result.Envelope)
	require.Len(t, result.Records, 5)
	assert.Nil(t, result.Records[0].Rejection)
	want := []string{envelope.ReasonMissingNativeID, envelope.ReasonMissingEventTime, envelope.ReasonKindNotAllowed, envelope.ReasonSchemaViolation}
	for i, code := range want {
		rec := result.Records[i+1]
		require.NotNil(t, rec.Rejection, "record %d", rec.No)
		assert.Equal(t, code, rec.Rejection.Code, rec.Rejection.Detail)
		assert.Empty(t, rec.Hash)
	}
	assert.Equal(t, 4, result.Rejected())
}

func TestCheck_FileReferences(t *testing.T) {
	t.Parallel()
	report := schemaOf(t, "report_file")
	data := build(t, manifest(envelope.ManifestSchemaIdJunit, false), func(w *envelope.Writer) {
		good := exampleRecord(t, w, report)
		missing := *good
		missing.NativeId = "missing"
		missing.File = &envelope.FileRef{Path: "files/absent.xml", Sha256: good.File.Sha256, SizeBytes: 1, MediaType: "application/xml"}
		changed := *good
		changed.NativeId = "changed"
		changed.File = &envelope.FileRef{Path: good.File.Path, Sha256: good.File.Sha256, SizeBytes: good.File.SizeBytes + 1, MediaType: "application/xml"}
		require.NoError(t, w.AddRecord(good))
		require.NoError(t, w.AddRecord(&missing))
		require.NoError(t, w.AddRecord(&changed))
	})
	result := check(data)
	require.Nil(t, result.Envelope)
	require.Len(t, result.Records, 3)
	assert.Nil(t, result.Records[0].Rejection)
	assert.Equal(t, envelope.ReasonMissingFile, result.Records[1].Rejection.Code)
	assert.Equal(t, envelope.ReasonFileMismatch, result.Records[2].Rejection.Code)
}

func TestCheck_TasksCSVIsSnapshot(t *testing.T) {
	t.Parallel()
	csv := "native_id,project_native_id,task_key,type_native_id,status_native_id,created_at,size\n" +
		"IMP-2,IMP,IMP-2,Story,In Progress,2026-02-03T09:00:00+03:00,S\n" +
		",IMP,IMP-3,Story,Done,2026-02-04T09:00:00+03:00,\n" +
		"IMP-4,IMP,IMP-4,Story,Done,yesterday,M\n"
	data := build(t, manifest(envelope.ManifestSchemaIdTasks, true), func(w *envelope.Writer) {
		require.NoError(t, w.AddTasksCSV([]byte(csv)))
	})
	result := check(data)
	require.Nil(t, result.Envelope)
	require.Len(t, result.Records, 3)
	assert.Nil(t, result.Records[0].Rejection)
	assert.Equal(t, "task", result.Records[0].Kind)
	assert.Equal(t, "IMP-2", result.Records[0].NativeID)
	assert.Equal(t, envelope.ReasonMissingNativeID, result.Records[1].Rejection.Code)
	assert.Equal(t, envelope.ReasonSchemaViolation, result.Records[2].Rejection.Code)

	history := build(t, manifest(envelope.ManifestSchemaIdTasks, true), func(w *envelope.Writer) {
		task := exampleRecord(t, w, schemaOf(t, "task"))
		task.RecordClass = envelope.RecordRecordClassObject
		require.NoError(t, w.AddRecord(task))
	})
	result = check(history)
	require.Len(t, result.Records, 1)
	assert.Equal(t, envelope.ReasonNotSnapshot, result.Records[0].Rejection.Code)

	unknown := build(t, manifest(envelope.ManifestSchemaIdTasks, true), func(w *envelope.Writer) {
		require.NoError(t, w.AddTasksCSV([]byte("native_id,description\nA,text\n")))
	})
	assert.Equal(t, envelope.ReasonInvalidCSV, check(unknown).Envelope.Code)
}

func TestContentHash(t *testing.T) {
	t.Parallel()
	line := func(fields string) string {
		return `{"record_class":"event","record_kind":"task_event","payload_schema":"task_event/1",` + fields + `,"file":null}`
	}
	base := line(`"native_ns":"20001","native_id":"30010/status","source_revision":"30010","event_time":"2026-03-02T12:00:00+03:00","observed_time":null,` +
		`"payload":{"task_native_id":"20001","event_kind":"field_change","field_native_id":"status","before_value":"1","after_value":"3"}`)
	reordered := line(`"payload":{"after_value":"3","before_value":"1","field_native_id":"status","event_kind":"field_change","task_native_id":"20001","actor_account_native_id":null},` +
		`"observed_time":null,"event_time":"2026-03-02T12:00:00.000+03:00","source_revision":"30010","native_id":"30010/status","native_ns":"20001"`)
	service := line(`"native_ns":"20009","native_id":"30099/status","source_revision":"30099","event_time":"2026-03-02T12:00:00+03:00","observed_time":"2026-03-05T00:00:00Z",` +
		`"payload":{"task_native_id":"20001","event_kind":"field_change","field_native_id":"status","before_value":"1","after_value":"3"}`)
	changed := line(`"native_ns":"20001","native_id":"30010/status","source_revision":"30010","event_time":"2026-03-02T12:00:00+03:00","observed_time":null,` +
		`"payload":{"task_native_id":"20001","event_kind":"field_change","field_native_id":"status","before_value":"1","after_value":"4"}`)
	shifted := line(`"native_ns":"20001","native_id":"30010/status","source_revision":"30010","event_time":"2026-03-02T09:00:00Z","observed_time":null,` +
		`"payload":{"task_native_id":"20001","event_kind":"field_change","field_native_id":"status","before_value":"1","after_value":"3"}`)
	data := archive(t, file(envelope.ManifestName, mustJSON(t, manifest(envelope.ManifestSchemaIdTaskEvents, false))),
		file(envelope.RecordsName, base+"\n"+reordered+"\n"+service+"\n"+changed+"\n"+shifted+"\n"))
	result := check(data)
	require.Nil(t, result.Envelope)
	require.Len(t, result.Records, 5)
	for _, rec := range result.Records {
		require.Nil(t, rec.Rejection, "record %d: %v", rec.No, rec.Rejection)
	}
	hash := result.Records[0].Hash
	assert.Equal(t, hash, result.Records[1].Hash, "key order, null fields and time precision do not change the hash")
	assert.Equal(t, hash, result.Records[2].Hash, "envelope service fields do not change the hash")
	assert.NotEqual(t, hash, result.Records[3].Hash, "a changed field changes the hash")
	assert.NotEqual(t, hash, result.Records[4].Hash, "the original offset is part of the event")

	var typed envelope.Record
	require.NoError(t, json.Unmarshal([]byte(base), &typed))
	got, err := envelope.ContentHash(&typed)
	require.NoError(t, err)
	assert.Equal(t, hash, got, "producers compute the same hash as the check")
}

func TestWriter_Deterministic(t *testing.T) {
	t.Parallel()
	pack := func() []byte {
		return build(t, manifest(envelope.ManifestSchemaIdTasks, false), func(w *envelope.Writer) {
			require.NoError(t, w.AddRecord(exampleRecord(t, w, schemaOf(t, "task"))))
		})
	}
	assert.Equal(t, pack(), pack())
}

type entry struct {
	name, content, link string
}

func file(name, content string) entry { return entry{name: name, content: content} }

func symlink(name, target string) entry { return entry{name: name, link: target} }

// archive writes raw tar.gz entries in the given order, including broken layouts the Writer refuses.
func archive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.content)), Typeflag: tar.TypeReg}
		if e.link != "" {
			header = &tar.Header{Name: e.name, Linkname: e.link, Typeflag: tar.TypeSymlink}
		}
		require.NoError(t, tw.WriteHeader(header))
		_, err := tw.Write([]byte(e.content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
