package entity_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestID_JSONIsTheUUIDText(t *testing.T) {
	t.Parallel()
	u := uuid.MustParse("0193a5b0-5656-7000-8000-00000000000a")
	b, err := json.Marshal(entity.ConnectProbeJob{ConnectionID: entity.ID(u)})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"connection_id":"0193a5b0-5656-7000-8000-00000000000a"`)

	var job entity.ConnectProbeJob
	require.NoError(t, json.Unmarshal(b, &job))
	assert.Equal(t, entity.ID(u), job.ConnectionID)

	for _, bad := range []string{`"0193a5b0565670008000000000000000"`, `"0193a5b0-5656-7000-8000-00000000000g"`, `"x"`} {
		assert.Error(t, json.Unmarshal([]byte(`{"connection_id":`+bad+`}`), &job), bad)
	}
}
