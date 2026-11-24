package normalize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestParseTrivy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file, kind, delta string
		want              []row
	}{
		{"trivy-sca.json", entity.AnalysisSCA, entity.DependencyDeltaComputed, []row{
			{rule: "CVE-2026-10001", native: "CVE-2026-10001", path: "web/package-lock.json", start: 12, end: 14, severity: "HIGH"},
			{rule: "GHSA-2345-6789-cfgh", native: "GHSA-2345-6789-cfgh", path: "web/package-lock.json", start: 12, end: 14, severity: "HIGH"},
			{rule: "CVE-2026-10001", native: "CVE-2026-10001", path: "tool/package-lock.json", severity: "MEDIUM"},
			{rule: "CVE-2026-10002", native: "CVE-2026-10002", path: "tool/package-lock.json", severity: "LOW"},
		}},
		{"trivy-iac.json", entity.AnalysisIaC, "", []row{
			{rule: "KSV001", native: "KSV001", path: "deploy/pod.yaml", start: 5, end: 7, severity: "HIGH"},
			{rule: "KSV001", native: "KSV001", path: "deploy/pod.yaml", start: 8, end: 9, severity: "HIGH"},
			{rule: "KSV002", native: "KSV002", path: "deploy/pod.yaml", start: 5, severity: "MEDIUM"},
		}},
	}
	for _, tt := range cases {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			got, err := parse(t, "trivy_json", tt.kind, fixture(t, tt.file))
			require.NoError(t, err)
			require.Equal(t, tt.want, rows(got.findings))
			assert.Equal(t, "2", got.formatVersion)
			assert.Equal(t, tt.delta, got.dependencyDelta)
			for _, f := range got.findings {
				assert.Len(t, f.Fingerprint, 32)
				assert.Equal(t, 1, f.SeverityMappingVersion)
				assert.Empty(t, f.NativeConfidence)
				assert.NotContains(t, string(f.Evidence), "synthetic-iac-marker")
			}
		})
	}
}

func TestParseTrivy_PackageAndResourceEvidence(t *testing.T) {
	t.Parallel()
	got, err := parse(t, "trivy_json", entity.AnalysisSCA, fixture(t, "trivy-sca.json"))
	require.NoError(t, err)
	require.Len(t, got.findings, 4)
	first := evidenceOf(t, got.findings[0])
	assert.Equal(t, "pkg:npm/sample@1.0.0", got.findings[0].PackagePURL)
	assert.Equal(t, "high", got.findings[0].MappedSeverity)
	assert.Equal(t, "direct", first.Package.Relationship)
	assert.Equal(t, "1.0.1", first.Package.FixedVersion)
	assert.Equal(t, entity.DependencyAdded, first.Package.Delta)
	assert.Equal(t, []string{"CVE-2026-10001"}, evidenceOf(t, got.findings[1]).Advisory.Aliases)
	assert.Equal(t, entity.DependencyFixUnknown, evidenceOf(t, got.findings[2]).Package.FixState)
	assert.Equal(t, entity.DependencyNoFix, evidenceOf(t, got.findings[3]).Package.FixState)
	assert.Equal(t, "pkg:npm/other@2.0.0", got.findings[3].PackagePURL)
	assert.NotEqual(t, got.findings[0].Fingerprint, got.findings[2].Fingerprint)

	got, err = parse(t, "trivy_json", entity.AnalysisIaC, fixture(t, "trivy-iac.json"))
	require.NoError(t, err)
	require.Len(t, got.findings, 3)
	assert.Equal(t, "Pod/default/sample", evidenceOf(t, got.findings[0]).Resource)
	assert.True(t, evidenceOf(t, got.findings[2]).Suppressed)
}

func TestParseTrivy_InvalidAndEmpty(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`{}`, `{"SchemaVersion":3}`, `{"SchemaVersion":2,"Results":[`, `{"SchemaVersion":2}{}`} {
		_, err := parse(t, "trivy_json", entity.AnalysisSCA, []byte(input))
		require.Error(t, err)
	}
	out, err := parse(t, "trivy_json", entity.AnalysisSCA, []byte(`{"SchemaVersion":2,"Results":null}`))
	require.NoError(t, err)
	assert.Empty(t, out.findings)
}
