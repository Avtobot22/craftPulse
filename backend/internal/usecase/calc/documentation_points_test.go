package calc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestDocumentationPoints_V23(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name                  string
		root, anchor, partial bool
		days                  int
		wantState             entity.DataState
		wantValue             *float64
	}{
		{"confirmed root with broken anchor", true, true, false, 0, entity.StateAssessed, num(1)},
		{"changed document date with broken anchor", true, true, false, 2, entity.StateAssessed, num(1)},
		{"confirmed root with fixed anchor", true, false, false, 3, entity.StateAssessed, num(0)},
		{"partially checked root", true, true, true, 3, entity.StateProvisional, num(1)},
		{"service without root", false, false, false, 1, entity.StateUnknown, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := documentationScenario(first, tt.days, tt.root, tt.anchor, tt.partial)
			points := s.points(period{start: s.asOf.Add(-time.Hour), end: s.asOf})
			link := docsPoint(t, points, "D10", "")
			debt := docsPoint(t, points, "D30", "")
			assert.Equal(t, tt.wantState, link.State)
			assert.Equal(t, tt.wantState, debt.State)
			assert.Equal(t, tt.wantValue, link.Value)
			assert.Equal(t, tt.wantValue, debt.Value)
			if !tt.root {
				for _, p := range points {
					assert.Equal(t, entity.StateUnknown, p.State)
					assert.Equal(t, _reasonDocsRootMissing, p.ReasonCode)
					assert.Nil(t, p.Value)
				}
				return
			}
			owner := docsPoint(t, points, "D03", _estRootOwner)
			assert.Equal(t, num(0), owner.Value)
			assert.Equal(t, _reasonDocsOwnerMissing, owner.ReasonCode)
			if tt.partial {
				coverage := docsPoint(t, points, "D01", _estDocCoverage)
				assert.Equal(t, entity.StateUnknown, coverage.State)
				assert.Nil(t, coverage.Value)
				assert.Equal(t, entity.StateProvisional, docsPoint(t, points, "D01", _estDocRead).State)
				assert.Nil(t, docsPoint(t, points, "D03", _estDocOwners).Value)
			}
			if tt.anchor {
				require.Len(t, debt.Contributions, 1)
				var found struct {
					FirstSeenAt time.Time `json:"first_seen_at"`
					AgeSeconds  int64     `json:"age_seconds"`
					VersionRef  string    `json:"version_ref"`
				}
				require.NoError(t, json.Unmarshal(debt.Contributions[0].Evidence, &found))
				assert.Equal(t, first, found.FirstSeenAt)
				assert.Equal(t, int64(tt.days)*86400, found.AgeSeconds)
				assert.Equal(t, s.sha, found.VersionRef)
			} else {
				assert.Empty(t, debt.Contributions)
			}
		})
	}
}

func TestDocumentationPoints_MissingEnabledCheck(t *testing.T) {
	t.Parallel()
	s := documentationScenario(time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC), 1, true, false, false)
	s.reports[0].Checks = []string{entity.AnalysisDocs, entity.AnalysisOpenAPI}
	points := s.points(period{start: s.asOf.Add(-time.Hour), end: s.asOf})
	d24 := docsPoint(t, points, "D24", "")
	assert.Nil(t, d24.Value)
	assert.Equal(t, entity.StateUnknown, d24.State)
	d30 := docsPoint(t, points, "D30", "")
	assert.Equal(t, num(0), d30.Value)
	assert.Equal(t, entity.StateProvisional, d30.State)
	assert.Equal(t, 1, d30.MissingCount)
}

func TestDocumentationPoints_NewProfileDisablesOlderCheck(t *testing.T) {
	t.Parallel()
	s := documentationScenario(time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC), 1, true, false, false)
	s.reports[0].Checks = []string{entity.AnalysisDocs}
	s.reports[0].ObservedAt = s.asOf
	older := entity.DocsReport{ID: entity.ID{6}, Kind: entity.AnalysisOpenAPI, SARIFID: entity.ID{7}, Completeness: "complete"}
	older.Checks = []string{entity.AnalysisDocs, entity.AnalysisOpenAPI}
	older.ObservedAt = s.asOf.Add(-time.Hour)
	s.reports = append(s.reports, older)
	s.findings = []entity.DocsFinding{{ID: entity.ID{8}, ReportID: older.SARIFID, Kind: entity.FindingOpenAPI}}
	points := s.points(period{start: s.asOf.Add(-time.Hour), end: s.asOf})
	assert.Equal(t, entity.StateNotApplicable, docsPoint(t, points, "D24", _estSpectral).State)
	assert.Equal(t, num(0), docsPoint(t, points, "D30", "").Value)
}

func documentationScenario(first time.Time, days int, root, anchor, partial bool) documentationSource {
	s := documentationSource{asOf: first.AddDate(0, 0, days), sha: strings.Repeat(string(rune('a'+days)), 40)}
	if !root {
		return s
	}
	doc := entity.DocumentVersion{ID: entity.ID{1}, Path: "docs/README.md", Format: "markdown", Owner: "front-matter", VersionRef: s.sha}
	s.inventory = &entity.DocumentationInventory{Root: entity.DocumentationRoot{ID: entity.ID{2}, PathPrefix: "docs"}, Documents: []entity.DocumentVersion{doc}}
	r := entity.DocsReport{ID: entity.ID{3}, Kind: entity.AnalysisDocs, SARIFID: entity.ID{4}, Completeness: "complete"}
	r.Links, r.OwnerRule = &entity.DocsLinkCounts{Checked: 1}, "owner"
	r.Documents = []entity.DocsReportDocument{{DocumentVersion: doc}}
	if partial {
		r.Completeness, r.Skipped = "partial", 1
	}
	if anchor {
		r.Links.Broken++
		s.findings = []entity.DocsFinding{
			{ID: entity.ID{5}, ReportID: r.SARIFID, Rule: entity.DocsRuleMissingAnchor, Kind: entity.FindingDocs, Path: doc.Path, FirstSeenAt: first, HasLinkTarget: true},
		}
	}
	s.reports = []entity.DocsReport{r}
	return s
}

func docsPoint(t *testing.T, points []entity.SeriesPoint, metric, estimator string) entity.SeriesPoint {
	t.Helper()
	for i := range points {
		p := &points[i]
		if p.MetricID == metric && p.Estimator == estimator {
			return *p
		}
	}
	t.Fatalf("no %s point for %s", metric, estimator)
	return entity.SeriesPoint{}
}
