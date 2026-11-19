package handler

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_definitionCursor = "metric_definition"
	_cohortCursor     = "cohort"
	// _unknownSize selects the group of tasks without a size.
	_unknownSize = "unknown"
)

func (h *reports) GetTeamReport(ctx context.Context, req openapi.GetTeamReportRequestObject) (openapi.GetTeamReportResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	q := entity.ReportQuery{TeamID: entity.ID(req.TeamId), Step: entity.StepWeek, From: p.From, To: p.To, RevisionID: (*entity.ID)(p.RevisionId)}
	if p.Step != nil {
		q.Step = entity.Step(*p.Step)
	}
	if p.ProjectId != nil {
		for _, id := range *p.ProjectId {
			q.Filter.Projects = append(q.Filter.Projects, entity.ID(id))
		}
	}
	q.Filter.TaskType = p.TaskType
	switch {
	case p.SizeLabel != nil && *p.SizeLabel == _unknownSize:
		q.Filter.UnknownSize = true
	case p.SizeLabel != nil:
		q.Filter.SizeLabel = p.SizeLabel
	}
	if p.GroupBy != nil {
		q.Filter.GroupBy = entity.GroupBy(*p.GroupBy)
	}
	r, err := h.views.TeamReport(ctx, s, scope, q)
	if err != nil {
		return nil, err
	}
	return openapi.GetTeamReport200JSONResponse(teamReport(&r, &q)), nil
}

func teamReport(r *entity.TeamReport, q *entity.ReportQuery) openapi.TeamReport {
	rev := &r.Revision
	out := openapi.TeamReport{
		TeamId: uuid.UUID(q.TeamID), Period: openapi.Period{From: q.From, To: q.To}, Step: openapi.TeamReportStep(q.Step),
		Revision: openapi.RevisionSummary{
			Id: uuid.UUID(rev.ID), SeriesId: uuid.UUID(rev.SeriesID), RevisionNo: rev.RevisionNo, AsOf: rev.AsOf, PublishedAt: rev.PublishedAt,
			Loading: rev.Loading,
		},
		Grouping: openapi.CurrentStructure, Metrics: make([]openapi.MetricSeries, 0, len(r.Series)),
		AccessLimited: len(r.Notes) > 0, AccessNotes: accessNotes(r.Notes),
	}
	if rev.IrreproducibleReason != "" {
		reason := openapi.RevisionSummaryIrreproducibleReason(rev.IrreproducibleReason)
		out.Revision.IrreproducibleReason = &reason
	}
	for i := range r.Series {
		out.Metrics = append(out.Metrics, metricSeries(&r.Series[i]))
	}
	return out
}

func metricSeries(s *entity.Series) openapi.MetricSeries {
	out := openapi.MetricSeries{
		MetricId: s.MetricID, DefinitionVersion: s.DefinitionVersion, Unit: s.Unit, Estimator: optional(s.Estimator),
		Points: make([]openapi.MetricPoint, len(s.Points)),
	}
	if g := s.Group; g != nil {
		out.Group = &struct {
			ActorKind *openapi.ActorKind `json:"actor_kind,omitempty"`
			EventKind *string            `json:"event_kind,omitempty"`
			SizeLabel *string            `json:"size_label,omitempty"`
			TaskType  *string            `json:"task_type,omitempty"`
		}{SizeLabel: g.SizeLabel, TaskType: g.TaskType}
	}
	for i := range s.Points {
		p := &s.Points[i]
		out.Points[i] = openapi.MetricPoint{PointStart: p.Start, Value: metricValue(s.Unit, p)}
	}
	return out
}

func accessNotes(notes []entity.AccessNote) []openapi.AccessNote {
	out := make([]openapi.AccessNote, len(notes))
	for i, n := range notes {
		out[i] = openapi.AccessNote{Code: openapi.AccessNoteCode(n.Code)}
		if !n.ConnectionID.IsZero() {
			out[i].ConnectionId = uuidPtr(&n.ConnectionID)
		}
	}
	return out
}

func (h *reports) ListMetricDefinitions(ctx context.Context, req openapi.ListMetricDefinitionsRequestObject) (openapi.ListMetricDefinitionsResponseObject, error) {
	if _, ok := middleware.SubjectFrom(ctx); !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	defs := h.views.MetricDefinitions()
	start := 0
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		i := slices.IndexFunc(defs, func(d entity.MetricDefinition) bool { return d.ID == c.Name })
		if !ok || c.Kind != _definitionCursor || i < 0 {
			return nil, errMalformedPage
		}
		start = i + 1
	}
	end := min(start+limit, len(defs))
	out := openapi.MetricDefinitionPage{Data: make([]openapi.MetricDefinition, 0, end-start)}
	for i := start; i < end; i++ {
		out.Data = append(out.Data, metricDefinition(&defs[i]))
	}
	if end < len(defs) {
		c := encodeCursor(cursorData{Kind: _definitionCursor, Name: defs[end-1].ID})
		out.NextCursor = &c
	}
	return openapi.ListMetricDefinitions200JSONResponse(out), nil
}

func metricDefinition(d *entity.MetricDefinition) openapi.MetricDefinition {
	out := openapi.MetricDefinition{MetricId: d.ID, DefinitionVersion: d.Version, Name: d.Name, Unit: d.Unit, CatalogRef: d.CatalogRef}
	if len(d.Parameters) > 0 {
		params := d.Parameters
		out.Parameters = &params
	}
	return out
}

func (h *reports) GetRevision(ctx context.Context, req openapi.GetRevisionRequestObject) (openapi.GetRevisionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	p, err := h.views.Revision(ctx, s, entity.ID(req.RevisionId))
	if err != nil {
		return nil, err
	}
	rev := &p.Revision
	out := openapi.Revision{
		Id: uuid.UUID(rev.ID), SeriesId: uuid.UUID(rev.SeriesID), RevisionNo: rev.RevisionNo, Status: openapi.RevisionStatus(rev.Status),
		Window: openapi.Period{From: rev.Window.From, To: rev.Window.To}, AsOf: rev.AsOf, Timezone: rev.Timezone, KnowledgeSeq: int(rev.KnowledgeSeq),
		PublishedAt: rev.PublishedAt, SupersededAt: rev.SupersededAt, Loading: rev.Loading,
		Definitions: make([]openapi.MetricDefinition, len(p.Definitions)), Inputs: emptyOf(openapi.Revision{}.Inputs, len(p.Inputs)),
		SourceWatermarks: emptyOf(openapi.Revision{}.SourceWatermarks, 0),
	}
	if rev.IrreproducibleReason != "" {
		reason := openapi.RevisionIrreproducibleReason(rev.IrreproducibleReason)
		out.IrreproducibleReason = &reason
	}
	for i := range p.Definitions {
		out.Definitions[i] = metricDefinition(&p.Definitions[i])
	}
	for _, in := range p.Inputs {
		if in.Kind == entity.InputKindWatermark {
			conn, err := uuid.Parse(in.ID)
			if err != nil {
				return nil, fmt.Errorf("handler: watermark of revision %s: %w", rev.ID, err)
			}
			w := elemOf(out.SourceWatermarks)
			w.ConnectionId, w.Watermark = conn, in.Watermark
			w.Completeness = openapi.RevisionSourceWatermarksCompleteness(in.Completeness)
			out.SourceWatermarks = append(out.SourceWatermarks, *w)
			continue
		}
		item := elemOf(out.Inputs)
		item.InputKind, item.InputId, item.Version = openapi.RevisionInputsInputKind(in.Kind), in.ID, optional(in.Version)
		out.Inputs = append(out.Inputs, *item)
	}
	return openapi.GetRevision200JSONResponse(out), nil
}

func (h *reports) ListRevisionCohort(ctx context.Context, req openapi.ListRevisionCohortRequestObject) (openapi.ListRevisionCohortResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	p := req.Params
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return nil, err
	}
	q := entity.CohortQuery{MetricID: p.MetricId, PointStart: p.PointStart, Limit: limit}
	if p.Estimator != nil {
		q.Estimator = *p.Estimator
	}
	if p.Role != nil {
		q.Role = entity.ContributionRole(*p.Role)
	}
	filter := q.MetricID + "/" + q.Estimator + "/" + string(q.Role)
	if p.Cursor != nil {
		c, ok := decodeCursor(*p.Cursor)
		item, err := strconv.Atoi(c.Sort)
		if !ok || err != nil || c.Kind != _cohortCursor || c.Name != filter || !c.At.Equal(q.PointStart) {
			return nil, errMalformedPage
		}
		q.After = &entity.ContributionKey{ObjectID: entity.ID(c.ID), ItemNo: item}
	}
	page, urls, err := h.views.Cohort(ctx, s, entity.ID(req.RevisionId), q)
	if err != nil {
		return nil, err
	}
	out := openapi.CohortPage{Data: make([]openapi.CohortItem, len(page.Items)), AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes)}
	for i := range page.Items {
		out.Data[i] = cohortItem(&page.Items[i], urls[i])
	}
	if n := page.Next; n != nil {
		c := encodeCursor(cursorData{Kind: _cohortCursor, Name: filter, At: q.PointStart, ID: uuid.UUID(n.ObjectID), Sort: strconv.Itoa(n.ItemNo)})
		out.NextCursor = &c
	}
	return openapi.ListRevisionCohort200JSONResponse(out), nil
}

func cohortItem(it *entity.CohortItem, url string) openapi.CohortItem {
	out := openapi.CohortItem{
		Object: openapi.ObjectRef{Kind: openapi.ObjectRefKind(it.Object.Kind), Id: uuid.UUID(it.ObjectID), SourceUrl: optional(url)},
		ItemNo: it.ItemNo, Role: openapi.CohortItemRole(it.Role), Value: float32Of(it.Value), ReasonCode: optional(it.ReasonCode),
		TaskType: it.TaskType, SizeLabel: it.SizeLabel, BasisDeleted: it.Object.Deleted,
	}
	if !it.Object.Deleted {
		out.Object.DisplayKey, out.Object.Title = it.Object.DisplayKey, it.Object.Title
	}
	if iv := it.Interval; iv != nil {
		to := iv.To
		out.Interval = &struct {
			From time.Time  `json:"from"`
			To   *time.Time `json:"to"`
		}{From: iv.From, To: &to}
	}
	return out
}
