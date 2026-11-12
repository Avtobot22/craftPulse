package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// writePackage stores a package of schemaID with one example record of every allowed kind; broken drops
// native_id of the first record.
func writePackage(t *testing.T, schemaID string, broken bool) string {
	t.Helper()
	readID := "read-1"
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(schemaID), SchemaVersion: "1",
		Source:       envelope.Source{Provider: envelope.SourceProviderUniversal, InstanceKey: "import.example.test"},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindRepository, NativeId: "42"},
		Completeness: envelope.ManifestCompletenessComplete,
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		ReadId:       &readID, Producer: envelope.Producer{Name: "cli-test", Version: "1"}, CreatedAt: time.Unix(0, 0).UTC(),
	}
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	schemas := map[string]envelope.RecordSchema{}
	for _, s := range envelope.RecordSchemas() {
		schemas[s.Kind] = s
	}
	for i, kind := range envelope.PackageKinds(schemaID) {
		s := schemas[kind]
		rec := &envelope.Record{RecordClass: envelope.RecordRecordClass(s.Classes[0]), RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: kind + "/" + s.Version, NativeId: "id-" + kind, Payload: s.Example}
		if broken && i == 0 {
			rec.NativeId = ""
		}
		switch rec.RecordClass {
		case envelope.RecordRecordClassEvent:
			at := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
			rec.EventTime = &at
		case envelope.RecordRecordClassFile:
			ref, err := w.AddFile("files/report.xml", "application/xml", []byte("<testsuites/>"))
			require.NoError(t, err)
			rec.File = &ref
		case envelope.RecordRecordClassObject, envelope.RecordRecordClassSnapshot:
		}
		require.NoError(t, w.AddRecord(rec))
	}
	require.NoError(t, w.Close())
	path := filepath.Join(t.TempDir(), schemaID+".tar.gz")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return path
}

func TestRun_EveryDataKindPasses(t *testing.T) {
	t.Parallel()
	for _, schemaID := range []string{"tasks", "task_events", "relations", "accounts", "merge_requests", "ci_runs", "findings", "junit", "coverage", "documents"} {
		t.Run(schemaID, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code, err := run([]string{"check", writePackage(t, schemaID, false)}, &out)
			require.NoError(t, err)
			assert.Equal(t, 0, code, out.String())
			assert.Contains(t, out.String(), "0 rejected")
		})
	}
}

func TestRun_ReportsRecordAndPackageRejections(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code, err := run([]string{"check", writePackage(t, "task_events", true)}, &out)
	require.NoError(t, err)
	assert.Equal(t, _exitRecordsRejected, code)
	assert.Contains(t, out.String(), "record 1 rejected task_event (no native_id) missing_native_id")
	assert.Contains(t, out.String(), "records: 0 accepted, 1 rejected")

	plain := filepath.Join(t.TempDir(), "plain.tar.gz")
	require.NoError(t, os.WriteFile(plain, []byte("not a package"), 0o600))
	out.Reset()
	code, err = run([]string{"check", plain}, &out)
	require.NoError(t, err)
	assert.Equal(t, _exitRejected, code)
	assert.Contains(t, out.String(), "package rejected: invalid_archive")

	_, err = run([]string{"verify"}, &out)
	require.ErrorIs(t, err, errUsage)
}
