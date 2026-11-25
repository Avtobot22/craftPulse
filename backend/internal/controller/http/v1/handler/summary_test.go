package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestSummaryStructureCountsAcrossStreams(t *testing.T) {
	t.Parallel()
	contract, err := openapi3.NewLoader().LoadFromFile("../../../../../../docs/technical-specification/openapi.yaml")
	require.NoError(t, err)
	schema := contract.Components.Schemas["MergeRequestSummary"].Value
	const first = `{"Complexity":{"State":"ASSESSED","Deltas":[{"Delta":1},{"Delta":-1}]},
		"Clones":{"State":"ASSESSED","Value":2,"Numerator":2,"ExcludedCount":1}}`
	const second = `{"Complexity":{"State":"ASSESSED","Deltas":[{"Delta":3},{"Delta":2}]},
		"Clones":{"State":"ASSESSED","Value":4,"Numerator":4,"ExcludedCount":2}}`
	const missing = `{"Complexity":{"State":"NOT_EVALUATED","ReasonCode":"report_missing"},
		"Clones":{"State":"NOT_EVALUATED","ReasonCode":"report_missing"}}`
	for _, tc := range []struct {
		name, streams      string
		state              openapi.DataState
		complexity, clones *float32
		missing, excluded  int
	}{
		{"complete", first + "," + second, "ASSESSED", new(float32(3)), new(float32(6)), 0, 3},
		{"missing first", missing + "," + first, "PROVISIONAL", new(float32(1)), new(float32(2)), 1, 1},
		{"missing last", first + "," + missing, "PROVISIONAL", new(float32(1)), new(float32(2)), 1, 1},
		{"no comparable streams", missing + "," + missing, "NOT_EVALUATED", nil, nil, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := summaryBody(&entity.SummaryRead{
				No: 1, HeadSHA: strings.Repeat("a", 40), Status: "ready", PipelineState: "complete",
				Publication: entity.Publication{Status: "not_requested"}, Redactions: []string{},
				Items: []entity.SummaryItem{{Kind: "coverage", Value: json.RawMessage(`{"Code":[` + tc.streams + `]}`)}},
			})
			require.NoError(t, err)
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			var response any
			require.NoError(t, json.Unmarshal(raw, &response))
			require.NoError(t, schema.VisitJSON(response))
			c16, c17 := body.Change.ComplexityIncreasedFunctions, body.Change.NewCloneGroups
			assert.Equal(t, tc.state, c16.State)
			assert.Equal(t, tc.state, c17.State)
			assert.Equal(t, tc.complexity, c16.Value)
			assert.Equal(t, tc.clones, c17.Value)
			assert.Equal(t, tc.clones, c17.Numerator)
			assert.Equal(t, tc.missing, c16.MissingCount)
			assert.Equal(t, tc.missing, c17.MissingCount)
			assert.Equal(t, tc.excluded, c17.ExcludedCount)
		})
	}
}

func TestSummaryCheckCountsFollowSecurityRight(t *testing.T) {
	t.Parallel()
	const full = `"metrics":[{"MetricID":"I08","State":"ASSESSED","Value":0.75,"Numerator":3,"Denominator":4,"N":4}]`
	summary := entity.SummaryRead{Items: []entity.SummaryItem{{
		Kind:  "ci_attempts",
		Value: json.RawMessage(`{` + full + `}`),
	}}}
	body, err := summaryBody(&summary)
	require.NoError(t, err)
	assert.Equal(t, openapi.DataState("NOT_EVALUATED"), body.Expected.MissingChecks.State)
	assert.Nil(t, body.Expected.MissingChecks.Value)
	assert.Nil(t, body.Expected.MissingChecks.Numerator)
	assert.Nil(t, body.Expected.MissingChecks.Denominator)
	assert.Nil(t, body.Expected.MissingChecks.N)

	summary.SecurityAllowed = true
	body, err = summaryBody(&summary)
	require.NoError(t, err)
	assert.Equal(t, new(float32(0.75)), body.Expected.MissingChecks.Value)
	assert.Equal(t, new(float32(3)), body.Expected.MissingChecks.Numerator)
	assert.Equal(t, new(float32(4)), body.Expected.MissingChecks.Denominator)
	assert.Equal(t, new(4), body.Expected.MissingChecks.N)

	summary.Items[0].Value = json.RawMessage(`{` + full + `,
		"public_checks":{"MetricID":"I08","State":"ASSESSED","Value":0,"Numerator":0,"Denominator":1,"N":1}}`)
	summary.SecurityAllowed = false
	body, err = summaryBody(&summary)
	require.NoError(t, err)
	assert.Equal(t, new(float32(0)), body.Expected.MissingChecks.Value)
	assert.Equal(t, new(float32(0)), body.Expected.MissingChecks.Numerator)
	assert.Equal(t, new(float32(1)), body.Expected.MissingChecks.Denominator)
	assert.Equal(t, new(1), body.Expected.MissingChecks.N)

	summary.SecurityAllowed = true
	body, err = summaryBody(&summary)
	require.NoError(t, err)
	assert.Equal(t, new(float32(0.75)), body.Expected.MissingChecks.Value)
	assert.Equal(t, new(float32(3)), body.Expected.MissingChecks.Numerator)
	assert.Equal(t, new(float32(4)), body.Expected.MissingChecks.Denominator)
	assert.Equal(t, new(4), body.Expected.MissingChecks.N)
}
