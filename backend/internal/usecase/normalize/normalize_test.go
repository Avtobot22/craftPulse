package normalize

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestPart_DefaultBatchBoundary(t *testing.T) {
	t.Parallel()
	u := New(Deps{})
	records := make([]entity.IngestRecord, 401)
	for i := range records {
		records[i].No = i + 1
	}

	first, more := u.part(records, 0)
	assert.Equal(t, records[:200], first)
	assert.True(t, more)
	second, more := u.part(records, 200)
	assert.Equal(t, records[200:400], second)
	assert.True(t, more)
	last, more := u.part(records, 400)
	assert.Equal(t, records[400:], last)
	assert.False(t, more)
}
