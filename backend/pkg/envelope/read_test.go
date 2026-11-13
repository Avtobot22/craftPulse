package envelope_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestReadCleaned_FindsRecordsByTheirNumbers(t *testing.T) {
	t.Parallel()
	task := schemaOf(t, "task")
	data := build(t, manifest(envelope.ManifestSchemaIdTasks, false), func(w *envelope.Writer) {
		good := exampleRecord(t, w, task)
		noID := *good
		noID.NativeId = ""
		third := *good
		third.NativeId = "third"
		for _, rec := range []*envelope.Record{good, &noID, &third} {
			require.NoError(t, w.AddRecord(rec))
		}
	})
	_, cleaned := clean(t, data)

	p, err := envelope.ReadCleaned(bytes.NewReader(cleaned), envelope.DefaultLimits)
	require.NoError(t, err)
	assert.Equal(t, envelope.ManifestSchemaIdTasks, p.Manifest.SchemaId)
	first, ok, err := p.Record(1)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "task/1", first.PayloadSchema)
	_, ok, err = p.Record(2)
	require.NoError(t, err)
	assert.False(t, ok, "a rejected record leaves its line empty")
	last, ok, err := p.Record(3)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "third", last.NativeId)
	_, ok, err = p.Record(4)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestReadCleaned_RefusesWhatIsNotAPackage(t *testing.T) {
	t.Parallel()
	_, err := envelope.ReadCleaned(bytes.NewReader([]byte("plain text")), envelope.DefaultLimits)
	assert.Error(t, err)
}
