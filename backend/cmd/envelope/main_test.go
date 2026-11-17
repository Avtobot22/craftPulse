package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
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
			raw, err := os.ReadFile("../../pkg/envelope/testdata/ci/gitlab-builtin/scan-manifest.json")
			require.NoError(t, err)
			var scan envelope.ScanManifestV1
			require.NoError(t, json.Unmarshal(raw, &scan))
			scan.Tools[0].Reports[0].File = ref
			var payload envelope.ReportFileV1
			require.NoError(t, json.Unmarshal(rec.Payload, &payload))
			scan.Tools[0].Producer = payload.Producer
			scan.Tools[0].ProducerVersion = payload.ProducerVersion
			scan.Tools[0].ProfileId = payload.ProfileId
			scan.Tools[0].ConfigHash = payload.ConfigHash
			scan.Tools[0].ExitClass = envelope.ScanToolRunExitClass(*payload.ExitClass)
			scan.Tools[0].Completeness = envelope.ScanToolRunCompleteness(*payload.Completeness)
			scan.Tools[0].StartedAt = *payload.StartedAt
			scan.Tools[0].FinishedAt = *payload.FinishedAt
			require.NoError(t, w.AddScanManifest(&scan))
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

func TestRun_CIManifestExamplesAndRejections(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, fixture, missing, version, reason string }{
		{name: "builtin", fixture: "gitlab-builtin"},
		{name: "external", fixture: "external-sarif"},
		{name: "timeout", fixture: "timeout"},
		{name: "jenkins", fixture: "jenkins-retry"},
		{name: "missing scan", fixture: "gitlab-builtin", missing: envelope.ScanManifestName, reason: envelope.ReasonMissingScanManifest},
		{name: "unknown scan", fixture: "gitlab-builtin", version: envelope.ScanManifestName, reason: envelope.ReasonUnsupportedManifestVersion},
		{name: "unknown execution", fixture: "jenkins-retry", version: envelope.ExecutionManifestName, reason: envelope.ReasonUnsupportedManifestVersion},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join("..", "..", "pkg", "envelope", "testdata", "ci", tt.fixture)
			var data bytes.Buffer
			gz := gzip.NewWriter(&data)
			tw := tar.NewWriter(gz)
			add := func(name string, content []byte) {
				require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}))
				_, err := tw.Write(content)
				require.NoError(t, err)
			}
			manifest, err := os.ReadFile(filepath.Join(root, envelope.ManifestName))
			require.NoError(t, err)
			add(envelope.ManifestName, manifest)
			require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				name, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				name = filepath.ToSlash(name)
				if name == envelope.ManifestName || name == tt.missing {
					return nil
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if name == tt.version {
					var value map[string]any
					if err := json.Unmarshal(content, &value); err != nil {
						return err
					}
					value["schema_version"] = "99"
					content, err = json.Marshal(value)
					if err != nil {
						return err
					}
				}
				add(name, content)
				return nil
			}))
			require.NoError(t, tw.Close())
			require.NoError(t, gz.Close())
			path := filepath.Join(t.TempDir(), "package.tar.gz")
			require.NoError(t, os.WriteFile(path, data.Bytes(), 0o600))
			var out bytes.Buffer
			code, err := run([]string{"check", path}, &out)
			require.NoError(t, err)
			if tt.reason == "" {
				assert.Equal(t, 0, code, out.String())
				assert.Contains(t, out.String(), "scan manifest: 1")
			} else {
				assert.Equal(t, _exitRejected, code, out.String())
				assert.Contains(t, out.String(), tt.reason)
			}
		})
	}
}
