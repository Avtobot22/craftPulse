package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestFindingBodyMatchesContract(t *testing.T) {
	t.Parallel()
	contract, err := openapi3.NewLoader().LoadFromFile("../../../../../../docs/technical-specification/openapi.yaml")
	require.NoError(t, err)
	schema := contract.Components.Schemas["FindingDetail"].Value
	for _, kind := range []string{entity.FindingCode, entity.FindingSecret} {
		t.Run(kind, func(t *testing.T) {
			body, err := findingBody(&entity.FindingDetail{
				FindingIdentity: entity.FindingIdentity{ID: entity.ID{1}, AreaID: entity.ID{3}, Kind: kind, FirstSeenAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)},
				Observation: entity.FindingOccurrence{
					RuleID: "example", Path: "example.go", LineStart: new(2), NativeSeverity: "high",
					Evidence: json.RawMessage(`{"rule_name":"Rule name","snippet":"synthetic-marker"}`),
				},
				Report: entity.TechnicalReport{ID: entity.ID{2}, SHA: strings.Repeat("a", 40), Mode: "import", Completeness: "complete"},
				State:  entity.FindingNewlyObserved,
			})
			require.NoError(t, err)
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			var value any
			require.NoError(t, json.Unmarshal(raw, &value))
			require.NoError(t, schema.VisitJSON(value))
			assert.Contains(t, string(raw), `"area_id":"03000000-0000-0000-0000-000000000000"`)
			assert.NotContains(t, string(raw), "synthetic-marker")
			if kind == entity.FindingSecret {
				assert.Empty(t, body.Evidence)
			} else {
				assert.Equal(t, "Rule name", body.Evidence["rule_name"])
			}
		})
	}
}
