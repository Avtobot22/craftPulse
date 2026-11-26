package calc

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type crossKey struct {
	metric string
	start  int64
	task   entity.ID
}

func isCrossMetric(metric string) bool {
	return metric == "X01" || metric == "X02" || isTechnicalCross(metric, "")
}

func crossBasisKey(row *entity.CohortRow) crossKey {
	metric := ""
	if isTechnicalCross(row.MetricID, row.Estimator) {
		metric = row.MetricID
	}
	return crossKey{metric: metric, start: row.PointStart.UnixNano(), task: row.ObjectID}
}

func (c *cohort) crossBasis(row *entity.CohortRow, v *visibility, f *entity.ReportFilter, g *entity.SeriesGroup) (crossEvidence, bool) {
	origin := c.cross[crossBasisKey(row)]
	b := origin
	b.MRs = nil
	hidden := false
	for i := range origin.MRs {
		m := origin.MRs[i]
		if !c.objectVisible(v, m.ID) {
			hidden = true
			continue
		}
		if len(m.Tasks) > 0 {
			m.Tasks = nil
			for _, id := range origin.MRs[i].Tasks {
				task, ok := c.tasks[id]
				if !ok || !c.visible(v, &task) {
					hidden = true
					continue
				}
				if selected(&task, f, g) {
					m.Tasks = append(m.Tasks, id)
				}
			}
			if len(m.Tasks) == 0 {
				continue
			}
		}
		if m.PipelineID != nil && !c.objectVisible(v, *m.PipelineID) {
			hidden = true
			if row.ObjectKind == entity.ObjectCIRun {
				continue
			}
			m.PipelineID, m.CIClass, m.Retry, m.RetryKnown = nil, "", false, false
			m.FailToPass, m.AllowedFailure = nil, nil
			if m.Technical != nil {
				tech := *m.Technical
				tech.MissingChecks, tech.PublicMissingChecks = false, false
				m.Technical = &tech
			}
		}
		if tech := m.Technical; tech != nil && !v.securityAllowed(c.objects[m.ID].AreaID, tech.ProjectID) {
			public := *tech
			public.Scopes = slices.DeleteFunc(slices.Clone(tech.Scopes), func(s entity.TechnicalScope) bool {
				return entity.SensitiveSummaryAnalysis(s.Kind)
			})
			public.MissingChecks = public.PublicMissingChecks
			hidden = hidden || len(public.Scopes) != len(tech.Scopes) || public.MissingChecks != tech.MissingChecks
			if securityEstimator(row.Estimator) {
				continue
			}
			m.Technical = &public
		}
		if reports := m.PipelineReports; reports != nil && !v.securityAllowed(c.objects[m.ID].AreaID, reports.ProjectID) {
			public := *reports
			public.Scopes = slices.DeleteFunc(slices.Clone(reports.Scopes), func(s entity.TechnicalScope) bool {
				return entity.SensitiveSummaryAnalysis(s.Kind)
			})
			hidden = hidden || len(public.Scopes) != len(reports.Scopes)
			m.PipelineReports = &public
		}
		b.MRs = append(b.MRs, m)
	}
	return b, hidden
}

func (v *visibility) securityAllowed(area, project entity.ID) bool {
	return slices.Contains(v.security, project) && slices.Contains(v.areaProjects[area], project)
}

func (c *cohort) crossItem(it *entity.CohortItem, row *entity.CohortRow, b *crossEvidence) {
	it.TaskAgeSeconds = b.Age
	it.OverTarget = b.OverTarget
	addRef := func(id entity.ID, kind entity.ObjectKind) {
		if id != it.ObjectID && !slices.ContainsFunc(it.Basis, func(ref entity.ObjectRef) bool { return ref.ID == id }) {
			it.Basis = append(it.Basis, c.reportRef(id, kind))
		}
	}
	addSignal := func(s entity.ObservationKind) {
		if s != "" && !slices.Contains(it.Signals, s) {
			it.Signals = append(it.Signals, s)
		}
	}
	for i := range b.MRs {
		m := &b.MRs[i]
		addRef(m.ID, entity.ObjectMergeRequest)
		if m.PipelineID != nil {
			addRef(*m.PipelineID, entity.ObjectCIRun)
		}
		for _, id := range m.Tasks {
			addRef(id, entity.ObjectTask)
		}
		addSignal(m.ReviewSignal)
		addSignal(m.ciSignal())
		if isTechnicalCross(row.MetricID, row.Estimator) {
			estimators := definition(row.MetricID).Estimators
			if row.MetricID == "X02" {
				estimators = []string{_ciMissingReport}
			}
			for _, est := range estimators {
				if n, _, _ := technicalSignal(m, row.MetricID, est); n > 0 {
					addSignal(entity.ObservationKind(est))
				}
			}
		}
	}
}

func securityEstimator(est string) bool {
	kind, _, _ := strings.Cut(est, ":")
	return entity.SensitiveSummaryAnalysis(kind) || kind == "new_security" || kind == "security_exception" || kind == "checked_security"
}

func crossSelected(metric, est string, b *crossEvidence, f *entity.ReportFilter) bool {
	if isTechnicalCross(metric, est) && securityEstimator(est) && len(b.MRs) == 0 {
		return false
	}
	if metric != "X03" {
		return true
	}
	return (f.MinTaskAgeSeconds == nil || b.Age != nil && *b.Age >= *f.MinTaskAgeSeconds) &&
		(f.OverTarget == nil || b.OverTarget != nil && *b.OverTarget == *f.OverTarget)
}

func scoreCrossRow(c *entity.Contribution, row *entity.CohortRow, b *crossEvidence) {
	if isTechnicalCross(row.MetricID, row.Estimator) {
		scoreTechnical(c, row.MetricID, row.Estimator, b)
	} else {
		crossScore(c, row.Estimator, b)
	}
}

func (m *crossMR) ciSignal() entity.ObservationKind {
	switch {
	case m.CIClass == "failure" || m.CIClass == "timeout":
		return entity.ObservationCIFailed
	case m.Retry:
		return entity.ObservationCIRetried
	case m.CIClass == "running":
		return entity.ObservationCIRunning
	}
	return ""
}

func (c *cohort) reportRef(id entity.ID, kind entity.ObjectKind) entity.ObjectRef {
	o := c.objects[id]
	return entity.ObjectRef{Kind: kind, ID: id, DisplayKey: o.DisplayKey, Title: o.Title}
}

func (c *cohort) mrSelected(v *visibility, r *entity.CohortRow, f *entity.ReportFilter, g *entity.SeriesGroup) bool {
	area := c.objects[r.ObjectID].AreaID
	if len(f.Projects) > 0 && !slices.ContainsFunc(v.areaProjects[area], func(id entity.ID) bool {
		return slices.Contains(f.Projects, id) && (v.whole || slices.Contains(v.projects, id))
	}) {
		return false
	}
	if f.TaskType == nil && f.SizeLabel == nil && !f.UnknownSize && g == nil {
		return true
	}
	var tasks []entity.ID
	// MR contributions store the confirmed links from the calculation's knowledge cut.
	if json.Unmarshal(r.Evidence, &tasks) != nil {
		return false
	}
	for _, id := range tasks {
		if task, ok := c.tasks[id]; ok && c.visible(v, &task) && selected(&task, f, g) {
			return true
		}
	}
	return false
}
