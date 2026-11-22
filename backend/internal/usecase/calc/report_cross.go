package calc

import (
	"encoding/json"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type crossKey struct {
	start int64
	task  entity.ID
}

func isCrossMetric(metric string) bool { return metric == "X01" || metric == "X02" }

func (c *cohort) crossBasis(row *entity.CohortRow, v *visibility) (crossEvidence, bool) {
	origin := c.cross[crossKey{start: row.PointStart.UnixNano(), task: row.ObjectID}]
	b := crossEvidence{Age: origin.Age}
	hidden := false
	for i := range origin.MRs {
		m := origin.MRs[i]
		if !c.objectVisible(v, m.ID) {
			hidden = true
			continue
		}
		if m.PipelineID != nil && !c.objectVisible(v, *m.PipelineID) {
			m.PipelineID, m.CIClass, m.Retry, m.RetryKnown = nil, "", false, false
			hidden = true
		}
		b.MRs = append(b.MRs, m)
	}
	return b, hidden
}

func (c *cohort) crossItem(it *entity.CohortItem, b *crossEvidence) {
	it.TaskAgeSeconds = b.Age
	for i := range b.MRs {
		m := &b.MRs[i]
		it.Basis = append(it.Basis, c.reportRef(m.ID, entity.ObjectMergeRequest))
		if m.PipelineID != nil {
			it.Basis = append(it.Basis, c.reportRef(*m.PipelineID, entity.ObjectCIRun))
		}
		for _, signal := range []entity.ObservationKind{m.ReviewSignal, m.ciSignal()} {
			if signal != "" && !slices.Contains(it.Signals, signal) {
				it.Signals = append(it.Signals, signal)
			}
		}
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
