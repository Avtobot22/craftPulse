package change

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestTechnicalEvidenceReevaluatesExceptionsAndDeduplicatesFindings(t *testing.T) {
	t.Parallel()
	area, id := entity.ID(uuid.New()), entity.ID(uuid.New())
	f := entity.SummaryFinding{Lifecycle: entity.FindingNew, FindingCoordinates: entity.FindingCoordinates{
		FindingAccess: entity.FindingAccess{AreaID: area, Kind: entity.FindingCode}, FindingID: id, Path: "src/ac.go",
	}}
	revision := entity.TechnicalSummaryRevision{TechnicalSummary: entity.TechnicalSummary{Streams: []entity.TechnicalStream{{
		ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisCode}, Complete: true, Comparable: true,
		Findings: []entity.SummaryFinding{f, f},
	}}}}
	active := entity.ExceptionVersion{FindingKind: entity.FindingCode, Scope: entity.ExceptionScope{AreaID: area, FindingID: id}}
	exempt := technicalEvidence(&revision, []entity.ExceptionVersion{active})
	require.Len(t, exempt.Scopes, 1)
	assert.Equal(t, 1, exempt.Scopes[0].Accepted)
	assert.Zero(t, exempt.Scopes[0].New)
	expired := technicalEvidence(&revision, nil)
	assert.Equal(t, 1, expired.Scopes[0].New)
	assert.Zero(t, expired.Scopes[0].Accepted)
}

func TestTechnicalEvidenceRequiresRepeatOfTheSameDocumentationCheck(t *testing.T) {
	t.Parallel()
	finding := entity.SummaryFinding{
		Lifecycle:          entity.FindingNew,
		FindingCoordinates: entity.FindingCoordinates{FindingID: entity.ID(uuid.New())},
	}
	previous := entity.TechnicalSummaryRevision{HeadSHA: "old", TechnicalSummary: entity.TechnicalSummary{Streams: []entity.TechnicalStream{{
		ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisDocs}, Complete: true,
		DocumentationKey: []byte{1}, Findings: []entity.SummaryFinding{finding},
	}}}}
	for _, tc := range []struct {
		name     string
		complete bool
		key      []byte
		resolved int
	}{
		{"same check", true, []byte{1}, 1},
		{"incomplete repeat", false, []byte{1}, 0},
		{"different scope", true, []byte{2}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := entity.TechnicalSummaryRevision{HeadSHA: "new", TechnicalSummary: entity.TechnicalSummary{Streams: []entity.TechnicalStream{{
				ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisDocs}, Complete: tc.complete, DocumentationKey: tc.key,
			}}}}
			row.Streams = recheckDocumentation(&row, []entity.TechnicalSummaryRevision{previous}, row.BuiltAt)
			got := technicalEvidence(&row, nil)
			require.Len(t, got.Scopes, 1)
			assert.Equal(t, tc.resolved, got.Scopes[0].Resolved)
		})
	}
}
