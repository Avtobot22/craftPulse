package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const (
	_mappingCursor  = "mapping_version"
	_calendarCursor = "calendar_version"
)

var (
	_weekdays = []struct {
		name openapi.Weekday
		day  time.Weekday
	}{
		{openapi.Mon, time.Monday},
		{openapi.Tue, time.Tuesday},
		{openapi.Wed, time.Wednesday},
		{openapi.Thu, time.Thursday},
		{openapi.Fri, time.Friday},
		{openapi.Sat, time.Saturday},
		{openapi.Sun, time.Sunday},
	}
	_valueTypes = map[string]openapi.WorkflowCatalogInstancesFieldsValueType{
		"string": openapi.WorkflowCatalogInstancesFieldsValueTypeString, "number": openapi.WorkflowCatalogInstancesFieldsValueTypeNumber,
		"option": openapi.WorkflowCatalogInstancesFieldsValueTypeOption, "user": openapi.WorkflowCatalogInstancesFieldsValueTypeUser,
		"date": openapi.WorkflowCatalogInstancesFieldsValueTypeDate,
	}
)

type process struct {
	uc          usecase.Process
	previews    usecase.MappingPreviews
	connections usecase.Connections
}

// GetWorkflowCatalog names the provider of every instance from connect, which owns the instances.
func (h *process) GetWorkflowCatalog(ctx context.Context, req openapi.GetWorkflowCatalogRequestObject) (openapi.GetWorkflowCatalogResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	cat, err := h.uc.WorkflowCatalog(ctx, s, entity.ID(req.TeamId))
	if err != nil {
		return nil, err
	}
	out := openapi.WorkflowCatalog{Instances: emptyOf(openapi.WorkflowCatalog{}.Instances, len(cat.Instances))}
	for i := range cat.Instances {
		in := &cat.Instances[i]
		inst, err := h.connections.Instance(ctx, s.OrganizationID, in.InstanceID)
		if err != nil {
			return nil, err
		}
		item := elemOf(out.Instances)
		item.InstanceId, item.Provider = uuid.UUID(in.InstanceID), openapi.Provider(inst.Provider)
		item.Statuses = emptyOf(item.Statuses, len(in.Statuses))
		for _, st := range in.Statuses {
			v := elemOf(item.Statuses)
			v.NativeId, v.Name, v.SourceCategory = st.NativeID, st.Name, optional(st.Category)
			item.Statuses = append(item.Statuses, *v)
		}
		item.Resolutions = emptyOf(item.Resolutions, len(in.Resolutions))
		for _, r := range in.Resolutions {
			v := elemOf(item.Resolutions)
			v.NativeId, v.Name = r.NativeID, r.Name
			item.Resolutions = append(item.Resolutions, *v)
		}
		item.IssueTypes = emptyOf(item.IssueTypes, len(in.Types))
		for _, t := range in.Types {
			v := elemOf(item.IssueTypes)
			v.NativeId, v.Name = t.NativeID, t.Name
			item.IssueTypes = append(item.IssueTypes, *v)
		}
		item.Fields = emptyOf(item.Fields, len(in.Fields))
		for _, f := range in.Fields {
			v := elemOf(item.Fields)
			v.NativeId, v.Name, v.ValueType = f.NativeID, f.Name, valueType(f.ValueType)
			item.Fields = append(item.Fields, *v)
		}
		out.Instances = append(out.Instances, *item)
	}
	out.UnmappedStatuses = make([]openapi.NativeStatusRef, len(cat.Unmapped))
	for i, ref := range cat.Unmapped {
		out.UnmappedStatuses[i] = openapi.NativeStatusRef{InstanceId: uuid.UUID(ref.InstanceID), NativeId: ref.NativeID}
	}
	return openapi.GetWorkflowCatalog200JSONResponse(out), nil
}

func (h *process) ListMappingVersions(ctx context.Context, req openapi.ListMappingVersionsRequestObject) (openapi.ListMappingVersionsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	team := entity.ID(req.TeamId)
	p, err := versionPage(req.Params.Limit, req.Params.Cursor, _mappingCursor, team)
	if err != nil {
		return nil, err
	}
	page, err := h.uc.ListMappingVersions(ctx, s, team, p)
	if err != nil {
		return nil, err
	}
	body := openapi.MappingVersionPage{Data: make([]openapi.MappingVersion, len(page.Items)), NextCursor: nextVersionCursor(page.Next, _mappingCursor, team)}
	for i := range page.Items {
		body.Data[i] = mappingVersionBody(&page.Items[i])
	}
	tag := etag(int64(page.Latest))
	return openapi.ListMappingVersions200JSONResponse{Body: body, Headers: openapi.ListMappingVersions200ResponseHeaders{ETag: &tag}}, nil
}

func (h *process) CreateMappingVersion(ctx context.Context, req openapi.CreateMappingVersionRequestObject) (openapi.CreateMappingVersionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil || req.Body.Rules == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/rules", Code: entity.FieldRequired}}}
	}
	// A version that is not ours cannot match the current one; the usecase still answers a repeated key first.
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		base = -1
	}
	team := entity.ID(req.TeamId)
	v, created, err := h.uc.CreateMappingVersion(ctx, s, team, int(base), entity.ID(req.Params.IdempotencyKey), mappingDraft(req.Body))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.VersionNo))
	if !created {
		return openapi.CreateMappingVersion200JSONResponse{Body: mappingVersionBody(&v), Headers: openapi.CreateMappingVersion200ResponseHeaders{ETag: &tag}}, nil
	}
	location := "/api/v1/teams/" + team.String() + "/mapping-versions"
	return openapi.CreateMappingVersion201JSONResponse{
		Body: mappingVersionBody(&v), Headers: openapi.CreateMappingVersion201ResponseHeaders{ETag: &tag, Location: &location},
	}, nil
}

// PreviewMapping applies the draft to the tasks the scope of the request allows; nothing is saved.
func (h *process) PreviewMapping(ctx context.Context, req openapi.PreviewMappingRequestObject) (openapi.PreviewMappingResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || req.Body.Rules == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/rules", Code: entity.FieldRequired}}}
	}
	p, err := h.previews.PreviewMapping(ctx, s, scope, entity.ID(req.TeamId), mappingDraft(req.Body))
	if err != nil {
		return nil, err
	}
	out := openapi.MappingPreview{
		AsOf: p.AsOf, Period: openapi.Period{From: p.Period.From, To: p.Period.To},
		Tasks: emptyOf(openapi.MappingPreview{}.Tasks, len(p.Tasks)), ControlValues: emptyOf(openapi.MappingPreview{}.ControlValues, len(p.Controls)),
		UnmappedStatuses: make([]openapi.NativeStatusRef, len(p.Unmapped)),
	}
	for i := range p.Tasks {
		t := &p.Tasks[i]
		item := elemOf(out.Tasks)
		item.Task = openapi.ObjectRef{Kind: openapi.ObjectRefKindTask, Id: uuid.UUID(t.TaskID), DisplayKey: optional(t.DisplayKey), Title: optional(t.Title)}
		item.StartedAt, item.CompletedAt, item.Outcome = t.StartedAt, t.CompletedAt, openapi.MappingPreviewTasksOutcome(t.Outcome)
		item.TaskType, item.SizeLabel, item.Unmapped = t.TaskType, t.SizeLabel, append([]string{}, t.Unmapped...)
		if t.BlockedSeconds != nil {
			seconds := int(*t.BlockedSeconds)
			item.BlockedSeconds = &seconds
		}
		out.Tasks = append(out.Tasks, *item)
	}
	for i := range p.Controls {
		c := &p.Controls[i]
		v := elemOf(out.ControlValues)
		v.MetricId, v.Value = c.MetricID, metricValue(c.Unit, &c.Point)
		out.ControlValues = append(out.ControlValues, *v)
	}
	for i, ref := range p.Unmapped {
		out.UnmappedStatuses[i] = openapi.NativeStatusRef{InstanceId: uuid.UUID(ref.InstanceID), NativeId: ref.NativeID}
	}
	return openapi.PreviewMapping200JSONResponse(out), nil
}

func (h *process) ListCalendarVersions(ctx context.Context, req openapi.ListCalendarVersionsRequestObject) (openapi.ListCalendarVersionsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	team := entity.ID(req.TeamId)
	p, err := versionPage(req.Params.Limit, req.Params.Cursor, _calendarCursor, team)
	if err != nil {
		return nil, err
	}
	page, err := h.uc.ListCalendarVersions(ctx, s, team, p)
	if err != nil {
		return nil, err
	}
	body := openapi.CalendarVersionPage{Data: make([]openapi.CalendarVersion, len(page.Items)), NextCursor: nextVersionCursor(page.Next, _calendarCursor, team)}
	for i := range page.Items {
		body.Data[i] = calendarVersionBody(&page.Items[i])
	}
	tag := etag(int64(page.Latest))
	return openapi.ListCalendarVersions200JSONResponse{Body: body, Headers: openapi.ListCalendarVersions200ResponseHeaders{ETag: &tag}}, nil
}

func (h *process) CreateCalendarVersion(ctx context.Context, req openapi.CreateCalendarVersionRequestObject) (openapi.CreateCalendarVersionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/timezone", Code: entity.FieldRequired}}}
	}
	d, err := calendarDraft(req.Body)
	if err != nil {
		return nil, err
	}
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		base = -1
	}
	team := entity.ID(req.TeamId)
	v, created, err := h.uc.CreateCalendarVersion(ctx, s, team, int(base), entity.ID(req.Params.IdempotencyKey), d)
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.VersionNo))
	if !created {
		return openapi.CreateCalendarVersion200JSONResponse{Body: calendarVersionBody(&v), Headers: openapi.CreateCalendarVersion200ResponseHeaders{ETag: &tag}}, nil
	}
	location := "/api/v1/teams/" + team.String() + "/calendar-versions"
	return openapi.CreateCalendarVersion201JSONResponse{
		Body: calendarVersionBody(&v), Headers: openapi.CreateCalendarVersion201ResponseHeaders{ETag: &tag, Location: &location},
	}, nil
}

func mappingDraft(b *openapi.MappingDraft) entity.MappingDraft {
	d := entity.MappingDraft{Timezone: b.Timezone, CalendarVersionNo: b.CalendarVersionNo, Rules: make([]entity.MappingRule, len(b.Rules))}
	for i, r := range b.Rules {
		d.Rules[i] = entity.MappingRule{
			Kind: entity.RuleKind(r.RuleKind), InstanceID: entity.ID(r.InstanceId), NativeRef: r.NativeRef, NativeValue: r.NativeValue,
			Meaning: r.Meaning, Rank: r.Rank,
		}
	}
	return d
}

func mappingVersionBody(v *entity.MappingVersion) openapi.MappingVersion {
	out := openapi.MappingVersion{
		VersionNo: v.VersionNo, BaseVersionNo: v.BaseVersionNo, Timezone: v.Timezone, CalendarVersionNo: v.CalendarVersionNo,
		Rules: make([]openapi.MappingRule, len(v.Rules)), ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt,
	}
	for i, r := range v.Rules {
		out.Rules[i] = openapi.MappingRule{
			RuleKind: openapi.MappingRuleRuleKind(r.Kind), InstanceId: uuid.UUID(r.InstanceID), NativeRef: r.NativeRef,
			NativeValue: r.NativeValue, Meaning: r.Meaning, Rank: r.Rank,
		}
	}
	return out
}

// calendarDraft maps the form; a repeated or unknown day and a malformed time are errors of their fields.
func calendarDraft(b *openapi.CalendarDraft) (entity.CalendarDraft, error) {
	verr := &entity.ValidationError{}
	d := entity.CalendarDraft{Timezone: b.Timezone, Holidays: make([]time.Time, len(b.Holidays))}
	for i, name := range b.Workdays {
		at := "/workdays/" + strconv.Itoa(i)
		day, ok := weekday(name)
		switch {
		case !ok:
			verr.Add(at, entity.FieldNotAllowedValue, "")
		case d.Workdays.Has(day):
			verr.Add(at, entity.FieldDuplicate, "")
		default:
			d.Workdays = d.Workdays.With(day)
		}
	}
	d.WorkStart = clockTime(verr, "/work_start", b.WorkStart)
	d.WorkEnd = clockTime(verr, "/work_end", b.WorkEnd)
	for i, h := range b.Holidays {
		d.Holidays[i] = time.Date(h.Year(), h.Month(), h.Day(), 0, 0, 0, 0, time.UTC)
	}
	if len(verr.Fields) > 0 {
		return entity.CalendarDraft{}, verr
	}
	return d, nil
}

func calendarVersionBody(v *entity.CalendarVersion) openapi.CalendarVersion {
	out := openapi.CalendarVersion{
		VersionNo: v.VersionNo, BaseVersionNo: v.BaseVersionNo, Timezone: v.Timezone, Workdays: []openapi.Weekday{},
		WorkStart: clockText(v.WorkStart), WorkEnd: clockText(v.WorkEnd), Holidays: make([]openapi_types.Date, len(v.Holidays)),
		ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt,
	}
	for _, w := range _weekdays {
		if v.Workdays.Has(w.day) {
			out.Workdays = append(out.Workdays, w.name)
		}
	}
	for i, h := range v.Holidays {
		out.Holidays[i] = openapi_types.Date{Time: h}
	}
	return out
}

func weekday(name openapi.Weekday) (time.Weekday, bool) {
	for _, w := range _weekdays {
		if w.name == name {
			return w.day, true
		}
	}
	return 0, false
}

// clockTime reads HH:MM of the contract.
func clockTime(verr *entity.ValidationError, field string, raw *string) *entity.TimeOfDay {
	if raw == nil {
		return nil
	}
	s := *raw
	if len(s) != 5 || s[2] != ':' {
		verr.Add(field, entity.FieldInvalidFormat, "HH:MM")
		return nil
	}
	h, herr := strconv.Atoi(s[:2])
	m, merr := strconv.Atoi(s[3:])
	if herr != nil || merr != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		verr.Add(field, entity.FieldInvalidFormat, "HH:MM")
		return nil
	}
	t := entity.TimeOfDay(h*60 + m)
	return &t
}

func clockText(t *entity.TimeOfDay) *string {
	if t == nil {
		return nil
	}
	s := time.Date(0, 1, 1, 0, int(*t), 0, 0, time.UTC).Format("15:04")
	return &s
}

func valueType(raw string) openapi.WorkflowCatalogInstancesFieldsValueType {
	if v, ok := _valueTypes[raw]; ok {
		return v
	}
	return openapi.WorkflowCatalogInstancesFieldsValueTypeOther
}

// versionPage reads the cursor of a list of versions; it is bound to the list and the team it came from.
func versionPage(limit *int, cursor *string, kind string, teamID entity.ID) (entity.VersionPageRequest, error) {
	n, err := pageLimit(limit)
	if err != nil {
		return entity.VersionPageRequest{}, err
	}
	p := entity.VersionPageRequest{Limit: n}
	if cursor == nil {
		return p, nil
	}
	c, ok := decodeCursor(*cursor)
	if !ok || c.Kind != kind || c.Sort != teamID.String() {
		return p, errMalformedPage
	}
	before, err := strconv.Atoi(c.Name)
	if err != nil || before < 1 {
		return p, errMalformedPage
	}
	p.Before = &before
	return p, nil
}

func nextVersionCursor(next *int, kind string, teamID entity.ID) *string {
	if next == nil {
		return nil
	}
	c := encodeCursor(cursorData{Kind: kind, Sort: teamID.String(), Name: strconv.Itoa(*next)})
	return &c
}

// metricValue is a point as the contract shows it: a value only in ASSESSED and PROVISIONAL, a bound only in
// BOUNDED, which the formula already ensures.
func metricValue(unit string, p *entity.Point) openapi.MetricValue {
	out := openapi.MetricValue{
		State: openapi.DataState(p.State), Unit: unit, Value: float32Of(p.Value), Numerator: float32Of(p.Numerator),
		Denominator: float32Of(p.Denominator), N: p.N, LowSample: p.LowSample, ExcludedCount: p.ExcludedCount,
		MissingCount: p.MissingCount, ReasonCode: optional(p.ReasonCode),
	}
	if p.Bound != nil {
		out.Bound = &struct {
			Kind  openapi.MetricValueBoundKind `json:"kind"`
			Value float32                      `json:"value"`
		}{Kind: openapi.MetricValueBoundKind(p.Bound.Kind), Value: float32(p.Bound.Value)}
	}
	return out
}

func float32Of(v *float64) *float32 {
	if v == nil {
		return nil
	}
	f := float32(*v)
	return &f
}
