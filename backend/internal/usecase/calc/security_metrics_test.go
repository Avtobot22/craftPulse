package calc

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestSecurity_DependencyInstancesAndAdvisories(t *testing.T) {
	t.Parallel()
	rep := entity.ReportEvidence{
		ID: entity.ID(uuid.New()), Kind: entity.AnalysisSCA,
		ExitClass: "findings", Completeness: "complete", DependencyDelta: entity.DependencyDeltaComputed,
	}
	cve := "CVE-2026-10001"
	finding := func(lockfile, id, fix, delta string, aliases ...string) entity.SecurityFinding {
		t.Helper()
		ev, err := json.Marshal(entity.SecurityEvidence{
			Package:  &entity.DependencyEvidence{Name: "sample", Version: "1", FixState: fix, Delta: delta},
			Advisory: &entity.AdvisoryEvidence{ID: id, Aliases: aliases},
		})
		require.NoError(t, err)
		return entity.SecurityFinding{
			ID: entity.ID(uuid.New()), ReportID: rep.ID,
			FindingOccurrence: entity.FindingOccurrence{
				Kind: entity.FindingDependency, Path: lockfile, RuleID: id,
				Fingerprint: []byte(lockfile + ":" + id), Evidence: ev,
			},
		}
	}
	added := finding("a/package-lock.json", cve, entity.DependencyFixable, entity.DependencyAdded)
	unknown := finding("b/package-lock.json", cve, entity.DependencyFixUnknown, "")
	noFix := finding("c/package-lock.json", "CVE-2026-10002", entity.DependencyNoFix, "")
	alias := finding("a/package-lock.json", "GHSA-2345-6789-cfgh", entity.DependencyFixable, entity.DependencyAdded, cve)
	tests := []struct {
		name                                                     string
		findings                                                 []entity.SecurityFinding
		packages, advisories, added, unknown, noFix, denominator int
	}{
		{"one CVE in two lockfiles", []entity.SecurityFinding{added, unknown, added}, 2, 1, 1, 1, 0, 1},
		{"explicit alias in the same lockfile", []entity.SecurityFinding{added, unknown, alias}, 2, 1, 1, 1, 0, 1},
		{"explicit no fix alongside missing metadata", []entity.SecurityFinding{added, unknown, noFix}, 3, 2, 1, 1, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rev, err := (&UseCase{}).CalculateSecurity(entity.SecurityFacts{Reports: []entity.ReportEvidence{rep}, Findings: tt.findings})
			require.NoError(t, err)
			m := rev.Metrics
			assertSecurityValue(t, m["S13"], float64(tt.packages))
			assertSecurityValue(t, m["S14"], float64(tt.advisories))
			assertSecurityValue(t, m["S16"], float64(tt.added))
			assertSecurityValue(t, m["S15"], 1.0/float64(tt.denominator))
			require.NotNil(t, m["S15"].Denominator)
			assert.Equal(t, tt.denominator, *m["S15"].Denominator)
			fixGroups := make(map[string]int)
			for _, g := range m["S15"].Groups {
				require.NotNil(t, g.Count)
				fixGroups[g.Scope] = *g.Count
			}
			assert.Equal(t, tt.unknown, fixGroups[entity.DependencyFixUnknown])
			assert.Equal(t, tt.noFix, fixGroups[entity.DependencyNoFix])
			assert.Len(t, m["S13"].Contributions, tt.packages)
			assert.Equal(t, cve, m["S14"].Contributions[0].AdvisoryID)
		})
	}
}

func TestSecurity_MissingAndFailedScans(t *testing.T) {
	t.Parallel()
	sca := func(exit, completeness, delta string) []entity.ReportEvidence {
		return []entity.ReportEvidence{{Kind: entity.AnalysisSCA, ExitClass: exit, Completeness: completeness, DependencyDelta: delta}}
	}
	tests := []struct {
		name               string
		reports            []entity.ReportEvidence
		wantSCA, wantDelta entity.DataState
	}{
		{"missing report", nil, entity.StateNotEvaluated, entity.StateNotEvaluated},
		{"scanner failed", sca("failed", "unknown", ""), entity.StateUnknown, entity.StateUnknown},
		{"scanner timed out", sca("timeout", "partial", ""), entity.StateUnknown, entity.StateUnknown},
		{"invalid report", []entity.ReportEvidence{{Kind: entity.AnalysisSCA, RejectedReason: entity.RecordReportInvalid}}, entity.StateUnknown, entity.StateUnknown},
		{"empty successful report with a diff", sca("success", "complete", entity.DependencyDeltaComputed), entity.StateAssessed, entity.StateAssessed},
		{"empty successful report without a base", sca("success", "complete", entity.DependencyDeltaNoBase), entity.StateAssessed, entity.StateNotEvaluated},
		{"unavailable lockfile comparison", sca("success", "complete", entity.DependencyDeltaUnavailable), entity.StateAssessed, entity.StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rev, err := (&UseCase{}).CalculateSecurity(entity.SecurityFacts{Reports: tt.reports})
			require.NoError(t, err)
			m := rev.Metrics
			for _, id := range []string{"S13", "S14"} {
				assert.Equal(t, tt.wantSCA, m[id].State)
				if tt.wantSCA == entity.StateAssessed {
					assertSecurityValue(t, m[id], 0)
				} else {
					assert.Nil(t, m[id].Value)
				}
			}
			assert.Equal(t, tt.wantDelta, m["S16"].State)
			if tt.wantDelta == entity.StateAssessed {
				assertSecurityValue(t, m["S16"], 0)
			} else {
				assert.Nil(t, m["S16"].Value)
			}
			assert.Nil(t, m["S15"].Value)
		})
	}
}

func assertSecurityValue(t *testing.T, metric entity.SecurityMetric, want float64) {
	t.Helper()
	require.NotNil(t, metric.Value)
	assert.Equal(t, want, *metric.Value)
}
