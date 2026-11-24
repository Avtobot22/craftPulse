package normalize

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCurrentSecurityReports_KeepsDistinctStreams(t *testing.T) {
	t.Parallel()
	newer := entity.ReportEvidence{
		Kind: entity.AnalysisSecrets, Format: "gitleaks_json", Producer: "gitleaks",
		Mode: "builtin", ProfileID: "gitleaks-default", ScopeHash: "same",
		SecretScope: entity.SecretScopeDiff, HistoryFromSHA: "from", HistoryToSHA: "to", KnownSeq: 2,
	}
	for _, tt := range []struct {
		name   string
		change func(*entity.ReportEvidence)
		want   int
	}{
		{"same stream with older scanner version", func(r *entity.ReportEvidence) { r.ProducerVersion = "older" }, 1},
		{"same stream with older rejection", func(r *entity.ReportEvidence) { r.RejectedReason = entity.RecordReportInvalid }, 1},
		{"another producer", func(r *entity.ReportEvidence) { r.Producer = "another" }, 2},
		{"another scope", func(r *entity.ReportEvidence) { r.ScopeHash = "another" }, 2},
		{"another profile", func(r *entity.ReportEvidence) { r.ProfileID = "alternative" }, 2},
		{"another diff base", func(r *entity.ReportEvidence) { r.HistoryFromSHA = "earlier" }, 2},
		{"head", func(r *entity.ReportEvidence) { r.SecretScope = entity.SecretScopeHead }, 2},
		{"unidentified stream", func(r *entity.ReportEvidence) { r.ScopeHash = "" }, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			older := newer
			older.ID, older.KnownSeq = entity.ID(uuid.New()), 1
			tt.change(&older)
			got := currentSecurityReports([]entity.ReportEvidence{newer, older})
			assert.Len(t, got, tt.want)
			assert.Equal(t, newer, got[0])
		})
	}
}

func TestCurrentSecurityReports_KeepsLastAcceptedWhenReplacementIsInvalid(t *testing.T) {
	t.Parallel()
	accepted := entity.ReportEvidence{Kind: entity.AnalysisSCA, Producer: "trivy", ScopeHash: "scope", KnownSeq: 2}
	rejected := accepted
	rejected.KnownSeq, rejected.RejectedReason = 3, entity.RecordReportInvalid
	stale := accepted
	stale.KnownSeq = 1
	assert.Equal(t, []entity.ReportEvidence{rejected, accepted}, currentSecurityReports([]entity.ReportEvidence{rejected, accepted, stale}))
}
