package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
)

func ingestMarker() string {
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:32]
}

func assertRedacted(t *testing.T, data []byte) {
	t.Helper()
	marker := ingestMarker()
	for i := 0; i+8 <= len(marker); i++ {
		require.False(t, bytes.Contains(data, []byte(marker[i:i+8])), "stored secret fragment")
	}
}

func markerCIPackage(t *testing.T) []byte {
	t.Helper()
	marker := ingestMarker()
	var manifest envelope.Manifest
	data, err := os.ReadFile("../../pkg/envelope/testdata/ci/gitlab-builtin/manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &manifest))
	manifest.SchemaId = envelope.ManifestSchemaIdFindings
	manifest.Source.Provider = envelope.SourceProviderGitlab
	manifest.Source.InstanceKey = "gitlab.example"
	var scan envelope.ScanManifestV1
	data, err = os.ReadFile("../../pkg/envelope/testdata/ci/gitlab-builtin/scan-manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &scan))
	template := scan.Tools[0]
	scan.Tools = nil
	var out bytes.Buffer
	w, err := envelope.NewWriter(&out, &manifest)
	require.NoError(t, err)
	var escaped strings.Builder
	for _, c := range marker {
		fmt.Fprintf(&escaped, "&#%d;", c)
	}
	sarif, err := json.Marshal(map[string]any{"version": "2.1.0", "runs": []any{map[string]any{"tool": map[string]any{"driver": map[string]any{"name": "fixture"}}, "results": []any{map[string]any{"ruleId": "fixture-rule", "message": map[string]any{"text": marker}, "properties": map[string]any{"evidence": marker[8:16]}, "locations": []any{map[string]any{"physicalLocation": map[string]any{"artifactLocation": map[string]any{"uri": "src/config.go"}, "region": map[string]any{"startLine": 12}}}}}}}}})
	require.NoError(t, err)
	reports := []struct {
		kind, format, media, version, name string
		content                            []byte
	}{
		{"junit", "junit_xml", "application/xml", "1", "junit.xml", []byte("\ufeff" + `<testsuite xmlns="urn:fixture" name="test"><testcase><failure>` + escaped.String() + ` ` + marker[5:13] + `</failure></testcase></testsuite>`)},
		{"sast", "sarif", "application/sarif+json", "2.1.0", "sast.sarif", sarif},
	}
	for _, report := range reports {
		ref, err := w.AddFile("files/reports/"+report.name, report.media, report.content)
		require.NoError(t, err)
		tool := template
		tool.Id = "fixture-" + report.kind
		tool.Producer = "fixture"
		tool.ProducerVersion = "1"
		tool.Mode = envelope.ScanToolRunModeImport
		tool.ProfileId, tool.ProfileImageDigest, tool.ConfigHash = nil, nil, nil
		tool.ReportKind = envelope.ScanToolRunReportKind(report.kind)
		ci := *template.Ci
		ci.RefName = "demo/" + marker
		tool.Ci = &ci
		tool.Reports = []envelope.ScanReport{{File: ref, ReportKind: envelope.ScanReportReportKind(report.kind), ReportFormat: envelope.ScanReportReportFormat(report.format), FormatVersion: report.version}}
		scan.Tools = append(scan.Tools, tool)
		payload, err := json.Marshal(envelope.ReportFileV1{ReportKind: envelope.ReportFileV1ReportKind(report.kind), ReportFormat: envelope.ReportFileV1ReportFormat(report.format), Sha: tool.Sha, Producer: tool.Producer, ProducerVersion: tool.ProducerVersion, RunNativeId: &ci.RunNativeId, AttemptNativeId: ci.AttemptNativeId})
		require.NoError(t, err)
		revision := ref.Sha256
		require.NoError(t, w.AddRecord(&envelope.Record{RecordClass: envelope.RecordRecordClassFile, RecordKind: envelope.RecordRecordKindReportFile, PayloadSchema: "report_file/1", NativeNs: "7002", NativeId: report.kind + ":" + ref.Path, SourceRevision: &revision, File: &ref, Payload: payload}))
	}
	line, path, message := 12, "src/config.go", marker
	payload, err := json.Marshal(envelope.FindingV1{FindingKind: envelope.FindingV1FindingKindSecret, LineStart: &line, Path: &path, Message: &message, Producer: "fixture", ProducerVersion: "1", RuleId: "fixture-rule", Sha: *template.Sha})
	require.NoError(t, err)
	require.NoError(t, w.AddRecord(&envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: envelope.RecordRecordKindFinding, PayloadSchema: "finding/1", NativeNs: *template.Sha, NativeId: "training-finding", Payload: payload}))
	require.NoError(t, w.AddScanManifest(&scan))
	require.NoError(t, w.Close())
	checked := envelope.Check(bytes.NewReader(out.Bytes()), envelope.DefaultLimits)
	require.Nil(t, checked.Envelope)
	require.Zero(t, checked.Rejected())
	return out.Bytes()
}

func (e *ingestEnv) assertCleanObjects(t *testing.T, rc entity.Receipt) []byte {
	t.Helper()
	require.Positive(t, rc.RedactionCount)
	assert.Equal(t, secrets.RulesHash(), rc.RedactionRulesHash)
	keys, err := e.store.List(t.Context(), "quarantine/"+e.orgID.String()+"/")
	require.NoError(t, err)
	assert.Empty(t, keys)
	keys, err = e.store.List(t.Context(), "packages/"+e.orgID.String()+"/")
	require.NoError(t, err)
	require.NotEmpty(t, keys)
	var stored []byte
	for _, key := range keys {
		body, err := e.store.Get(t.Context(), key)
		require.NoError(t, err)
		data, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		gz, err := gzip.NewReader(bytes.NewReader(data))
		require.NoError(t, err)
		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			entry, err := io.ReadAll(tr)
			require.NoError(t, err)
			assertRedacted(t, []byte(header.Name))
			assertRedacted(t, entry)
			if strings.HasSuffix(header.Name, ".xml") {
				decoder := xml.NewDecoder(bytes.NewReader(entry))
				for {
					token, err := decoder.RawToken()
					if errors.Is(err, io.EOF) {
						break
					}
					require.NoError(t, err)
					switch value := token.(type) {
					case xml.CharData:
						assertRedacted(t, value)
					case xml.StartElement:
						for _, attr := range value.Attr {
							assertRedacted(t, []byte(attr.Value))
						}
					}
				}
			}
		}
		require.NoError(t, gz.Close())
		if key == rc.ObjectKey {
			stored = data
		}
	}
	require.NotEmpty(t, stored)
	checked, err := envelope.Clean(bytes.NewReader(stored), envelope.DefaultLimits, io.Discard)
	require.NoError(t, err)
	require.Nil(t, checked.Envelope)
	assert.Equal(t, checked.ContentHash, rc.PackageHash)
	assertRedacted(t, e.logs.Bytes())
	rows, err := e.owner.Query(t.Context(), `SELECT DISTINCT c.table_schema,c.table_name FROM information_schema.columns c JOIN information_schema.tables t ON t.table_schema=c.table_schema AND t.table_name=c.table_name WHERE c.column_name='organization_id' AND t.table_type='BASE TABLE' ORDER BY 1,2`)
	require.NoError(t, err)
	var tables [][]string
	for rows.Next() {
		var schema, name string
		require.NoError(t, rows.Scan(&schema, &name))
		tables = append(tables, []string{schema, name})
	}
	require.NoError(t, rows.Err())
	rows.Close()
	for _, table := range tables {
		var dump []byte
		query := "SELECT COALESCE(jsonb_agg(to_jsonb(t)), '[]'::jsonb) FROM " + pgx.Identifier(table).Sanitize() + " t WHERE organization_id=$1"
		require.NoError(t, e.owner.QueryRow(t.Context(), query, e.orgID).Scan(&dump))
		assertRedacted(t, dump)
	}
	return stored
}

func TestIngestRedaction_CIArtifactsFindingAndReplay(t *testing.T) {
	e := newCIEnv(t)
	data := markerCIPackage(t)
	response := e.uploadCI(t, e.token.Value, data)
	require.Equal(t, http.StatusAccepted, response.Code)
	first := decode[acceptedBody](t, response)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(first.ReceiptID)))
	rc := e.receipt(t, entity.ID(first.ReceiptID))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	stored := e.assertCleanObjects(t, rc)
	pkg, err := envelope.ReadCleaned(bytes.NewReader(stored), envelope.DefaultLimits)
	require.NoError(t, err)
	finding, ok, err := pkg.Record(3)
	require.NoError(t, err)
	require.True(t, ok)
	var payload envelope.FindingV1
	require.NoError(t, json.Unmarshal(finding.Payload, &payload))
	assert.Equal(t, "fixture-rule", payload.RuleId)
	assert.Equal(t, "src/config.go", *payload.Path)
	assert.Equal(t, 12, *payload.LineStart)
	for range 2 {
		duplicate := e.uploadCI(t, e.token.Value, data)
		require.Equal(t, http.StatusAccepted, duplicate.Code)
		assertRedacted(t, duplicate.Body.Bytes())
		repeat := e.receipt(t, entity.ID(decode[acceptedBody](t, duplicate).ReceiptID))
		require.Equal(t, entity.ReceiptDuplicate, repeat.Status)
		assert.Equal(t, rc.PackageHash, repeat.PackageHash)
	}
	families, err := e.reg.Gather()
	require.NoError(t, err)
	var replacements float64
	for _, family := range families {
		if family.GetName() == "craftpulse_ingest_secret_replacements_total" {
			for _, metric := range family.Metric {
				replacements += metric.GetCounter().GetValue()
			}
		}
	}
	assert.Equal(t, float64(rc.RedactionCount), replacements)
	assertRedacted(t, e.logs.Bytes())
}

func TestIngestRedaction_ImportAndSync(t *testing.T) {
	e := newIngestEnv(t)
	record := task(t, "J-SECRET", ingestMarker(), "1")
	raw, err := json.Marshal([]*envelope.Record{record})
	require.NoError(t, err)
	first, err := e.uc.Import(t.Context(), e.admin, entity.ImportFile{ProjectID: e.project, SchemaID: "tasks", SchemaVersion: "1", Provider: entity.ProviderJiraCloud, InstanceKey: _jiraInstance, Format: entity.ImportJSON, Content: raw})
	require.NoError(t, err)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, first.ID))
	rc := e.receipt(t, first.ID)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.assertCleanObjects(t, rc)
	conn, scope := e.scope(t, "10000")
	page := e.page(t, conn, scope, pack(t, importManifest("1"), task(t, "J-SYNC", ingestMarker(), "1")))
	require.Equal(t, entity.ReceiptAccepted, page.Status)
	e.assertCleanObjects(t, page)
	again := e.page(t, conn, scope, pack(t, importManifest("1"), task(t, "J-SYNC", ingestMarker(), "1")))
	require.Equal(t, entity.ReceiptDuplicate, again.Status)
	assert.Equal(t, page.PackageHash, again.PackageHash)
}

func TestIngestRedaction_RejectedRecordIdentity(t *testing.T) {
	e := newIngestEnv(t)
	bad := task(t, ingestMarker(), "invalid kind", "1")
	bad.RecordKind = "unsupported"
	input := pack(t, importManifest("1"), task(t, "J-CLEAN", "clean", "1"), bad)
	receipt := e.imported(t, e.admin, input)
	require.Equal(t, entity.ReceiptPartial, receipt.Status)
	records := e.records(t, receipt.ID)
	require.Len(t, records, 2)
	for _, record := range records {
		assertRedacted(t, []byte(record.NativeID))
	}
	e.assertCleanObjects(t, receipt)
}

func TestIngestRedaction_RejectedManifestHasNoSecret(t *testing.T) {
	e := newIngestEnv(t)
	manifest := importManifest("1")
	manifest.Producer.Name = ingestMarker()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	for _, entry := range []struct {
		name string
		data []byte
	}{{envelope.ManifestName, encoded}, {"../outside", []byte("unsafe")}} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o600, Size: int64(len(entry.data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	receipt, err := e.uc.SubmitImport(t.Context(), e.admin, e.project, &out)
	require.NoError(t, err)
	require.Equal(t, entity.ReceiptRejected, receipt.Status)
	assert.Positive(t, receipt.RedactionCount)
	assertRedacted(t, receipt.Manifest)
	stored := e.receipt(t, receipt.ID)
	assertRedacted(t, stored.Manifest)
	assert.Empty(t, e.keys(t, "quarantine/"))
	assert.Empty(t, e.keys(t, "packages/"))
	assertRedacted(t, e.logs.Bytes())
}
