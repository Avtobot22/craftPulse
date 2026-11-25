package change

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestSummaryCheckJobsKeepOnlyVisibleExpectations(t *testing.T) {
	t.Parallel()
	u := New(Deps{Catalog: entity.CheckCatalog{Profiles: []entity.CheckProfile{
		{ID: "go-default"}, {ID: "gitleaks-default"}, {ID: "sarif-import", ReportFormat: "sarif"},
	}}})
	entries := []entity.ProfileEntry{
		{ProfileID: "go-default", Mode: entity.CheckModeBuiltin, AnalysisKind: entity.AnalysisCode},
		{ProfileID: "gitleaks-default", Mode: entity.CheckModeBuiltin, AnalysisKind: entity.AnalysisSecrets},
		{ProfileID: "sarif-import", Mode: entity.CheckModeImport, AnalysisKind: entity.AnalysisSAST},
		{ProfileID: "sarif-import", Mode: entity.CheckModeImport, AnalysisKind: entity.AnalysisCode},
	}
	checks := u.summaryCheckJobs(entries[:3])
	assert.Equal(t, []string{"craftpulse-gitleaks-default", "craftpulse-go-default", "craftpulse-import"}, checks.All)
	assert.Equal(t, []string{"craftpulse-go-default"}, checks.Public)

	shared := u.summaryCheckJobs(entries)
	assert.Equal(t, checks.All, shared.All)
	assert.Equal(t, []string{"craftpulse-go-default", "craftpulse-import"}, shared.Public)
}
