package envelope_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func clean(t *testing.T, data []byte) (res *envelope.Result, cleaned []byte) {
	t.Helper()
	var out bytes.Buffer
	res, err := envelope.Clean(bytes.NewReader(data), envelope.DefaultLimits, &out)
	require.NoError(t, err)
	return res, out.Bytes()
}

func TestClean_KeepsAcceptedRecordsOnTheirNumbers(t *testing.T) {
	t.Parallel()
	report := schemaOf(t, "report_file")
	data := build(t, manifest(envelope.ManifestSchemaIdJunit, false), func(w *envelope.Writer) {
		good := exampleRecord(t, w, report)
		_, err := w.AddFile("files/unused.xml", "application/xml", []byte("<testsuites/>"))
		require.NoError(t, err)
		noID := *good
		noID.NativeId = ""
		second := *good
		second.NativeId = "second"
		for _, rec := range []*envelope.Record{good, &noID, &second} {
			require.NoError(t, w.AddRecord(rec))
		}
	})
	res, cleaned := clean(t, data)
	require.Nil(t, res.Envelope)
	require.Len(t, res.Records, 3)
	assert.Equal(t, "file", res.Records[0].Class)
	assert.Equal(t, "report_file/1", res.Records[0].PayloadSchema)
	assert.Equal(t, envelope.ReasonMissingNativeID, res.Records[1].Rejection.Code)

	again := check(cleaned)
	require.Nil(t, again.Envelope, "the cleaned package is a valid package")
	require.Len(t, again.Records, 2)
	assert.Equal(t, []int{1, 3}, []int{again.Records[0].No, again.Records[1].No}, "record numbers hold")
	assert.Equal(t, res.Records[2].Hash, again.Records[1].Hash)
	assert.NotContains(t, string(gunzip(t, cleaned)), "files/unused.xml", "a file no record references is dropped")

	_, twice := clean(t, data)
	assert.Equal(t, cleaned, twice, "the same input gives the same bytes")
}

func TestClean_TaskCSVBecomesSnapshotRecords(t *testing.T) {
	t.Parallel()
	data := build(t, manifest(envelope.ManifestSchemaIdTasks, true), func(w *envelope.Writer) {
		require.NoError(t, w.AddTasksCSV([]byte("native_id,project_native_id,task_key,type_native_id,status_native_id,created_at\n"+
			",IMP,IMP-3,Story,Done,2026-02-04T09:00:00+03:00\n"+
			"IMP-2,IMP,IMP-2,Story,In Progress,2026-02-03T09:00:00+03:00\n")))
	})
	res, cleaned := clean(t, data)
	require.Nil(t, res.Envelope)
	again := check(cleaned)
	require.Nil(t, again.Envelope)
	require.Len(t, again.Records, 1)
	assert.Equal(t, 2, again.Records[0].No, "the row number of the CSV is the record number")
	assert.Equal(t, "IMP-2", again.Records[0].NativeID)
	assert.Equal(t, "snapshot", again.Records[0].Class)
	assert.Equal(t, res.Records[1].Hash, again.Records[0].Hash)
}

func TestClean_RefusesXMLWithADocumentType(t *testing.T) {
	t.Parallel()
	report := schemaOf(t, "report_file")
	for name, content := range map[string]string{
		"external entity": `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]><testsuites>&x;</testsuites>`,
		"lower case":      "\n  <!doctype html><testsuites/>",
	} {
		data := build(t, manifest(envelope.ManifestSchemaIdJunit, false), func(w *envelope.Writer) {
			ref, err := w.AddFile("files/report.xml", "application/xml", []byte(content))
			require.NoError(t, err)
			rec := exampleRecord(t, w, report)
			rec.File = &ref
			require.NoError(t, w.AddRecord(rec))
		})
		res, cleaned := clean(t, data)
		require.NotNil(t, res.Envelope, name)
		assert.Equal(t, envelope.ReasonUnsafeXML, res.Envelope.Code, name)
		assert.Empty(t, cleaned, "nothing is written for a rejected package")
	}
	json := build(t, manifest(envelope.ManifestSchemaIdJunit, false), func(w *envelope.Writer) {
		ref, err := w.AddFile("files/report.json", "application/json", []byte(`{"note":"<!DOCTYPE in a message"}`))
		require.NoError(t, err)
		rec := exampleRecord(t, w, report)
		rec.File = &ref
		require.NoError(t, w.AddRecord(rec))
	})
	res, _ := clean(t, json)
	assert.Nil(t, res.Envelope, "only a file that starts as XML is checked")
}

func TestClean_BrokenStreamIsRejected(t *testing.T) {
	t.Parallel()
	data := build(t, manifest(envelope.ManifestSchemaIdTasks, true), func(w *envelope.Writer) {
		require.NoError(t, w.AddTasksCSV([]byte("native_id\n10\n")))
	})
	res, _ := clean(t, data[:len(data)/2])
	require.NotNil(t, res.Envelope)
	assert.Equal(t, envelope.ReasonInvalidArchive, res.Envelope.Code)
}

func gunzip(t *testing.T, data []byte) []byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	out, err := io.ReadAll(gz)
	require.NoError(t, err)
	return out
}
