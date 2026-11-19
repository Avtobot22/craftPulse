package link

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSameEvidence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "key order and spaces", a: `{"branch":"TASK-1","sha":"a1"}`, b: `{ "sha": "a1", "branch": "TASK-1" }`, want: true},
		{name: "another value", a: `{"branch":"TASK-1"}`, b: `{"branch":"TASK-2"}`},
		{name: "both empty", want: true},
		{name: "one empty", a: `{"branch":"TASK-1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sameEvidence(json.RawMessage(tt.a), json.RawMessage(tt.b))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	_, err := sameEvidence(json.RawMessage(`{}`), json.RawMessage(`{`))
	assert.ErrorIs(t, err, errProposal)
}
