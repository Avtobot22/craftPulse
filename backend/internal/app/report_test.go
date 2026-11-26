package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var (
	_contractOnce sync.Once
	_contractDoc  *openapi3.T
	_contractErr  error
)

type reportValueBody struct {
	State         string   `json:"state"`
	Value         *float64 `json:"value"`
	Numerator     *float64 `json:"numerator"`
	Denominator   *float64 `json:"denominator"`
	N             *int     `json:"n"`
	ExcludedCount int      `json:"excluded_count"`
	ReasonCode    *string  `json:"reason_code"`
}

type reportBody struct {
	Revision struct {
		ID uuid.UUID `json:"id"`
	} `json:"revision"`
	Metrics []struct {
		MetricID  string  `json:"metric_id"`
		Estimator *string `json:"estimator"`
		Group     *struct {
			TaskType  *string `json:"task_type"`
			SizeLabel *string `json:"size_label"`
		} `json:"group"`
		Points []struct {
			PointStart time.Time       `json:"point_start"`
			Value      reportValueBody `json:"value"`
		} `json:"points"`
	} `json:"metrics"`
	AccessLimited bool `json:"access_limited"`
	AccessNotes   []struct {
		Code string `json:"code"`
	} `json:"access_notes"`
}

type cohortBody struct {
	Data          []cohortItemBody `json:"data"`
	NextCursor    *string          `json:"next_cursor"`
	AccessLimited bool             `json:"access_limited"`
}

type cohortItemBody struct {
	Object struct {
		ID         uuid.UUID `json:"id"`
		DisplayKey *string   `json:"display_key"`
		SourceURL  *string   `json:"source_url"`
	} `json:"object"`
	Role       string  `json:"role"`
	ReasonCode *string `json:"reason_code"`
	TaskType   *string `json:"task_type"`
	SizeLabel  *string `json:"size_label"`
	Interval   *struct {
		From time.Time `json:"from"`
	} `json:"interval"`
}

// value is the point of the ungrouped series of a metric and an estimator starting at a time.
func (r reportBody) value(t *testing.T, metric, estimator string, start time.Time) reportValueBody {
	t.Helper()
	for _, m := range r.Metrics {
		if m.MetricID != metric || (m.Estimator == nil) != (estimator == "") || (m.Estimator != nil && *m.Estimator != estimator) || m.Group != nil {
			continue
		}
		for _, p := range m.Points {
			if p.PointStart.Equal(start) {
				return p.Value
			}
		}
	}
	require.Failf(t, "no point", "%s %s %s", metric, estimator, start)
	return reportValueBody{}
}

func (c *cohortBody) keys() []string {
	out := make([]string, 0, len(c.Data))
	for _, it := range c.Data {
		out = append(out, *it.Object.DisplayKey+":"+it.Role)
	}
	return out
}

// reportEnv is the team of processEnv with types, sizes and cancellation in its mapping and the tasks R-A, R-B
// and R-C of V-02/V-03. week is the Monday in Europe/Moscow six weeks before the clock: A completes in it and B
// is cancelled, C completes the week after; nothing of the test adapter falls there.
type reportEnv struct {
	*seriesEnv
	week, next time.Time
}

// day is a time of the week of the set: Monday plus days, at 10:00 in Moscow.
func (e *reportEnv) day(n int) time.Time { return e.week.AddDate(0, 0, n).Add(10 * time.Hour) }

func newReportEnv(t *testing.T, mapped bool) *reportEnv {
	t.Helper()
	p := newProcessEnv(t)
	p.grantProcessOwner(t, p.adminID)
	if mapped {
		rule := func(kind entity.RuleKind, ref, value, meaning string, rank *int) entity.MappingRule {
			return entity.MappingRule{Kind: kind, InstanceID: entity.ID(p.instance), NativeRef: ref, NativeValue: value, Meaning: meaning, Rank: rank}
		}
		admin := entity.Subject{OrganizationID: p.orgID, UserID: entity.ID(p.adminID)}
		_, _, err := p.process.CreateMappingVersion(t.Context(), admin, entity.ID(p.team), 0, entity.ID(uuid.New()), entity.MappingDraft{
			Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
				rule(entity.RuleStatus, "1", "", entity.MeaningIntake, nil), rule(entity.RuleStatus, "3", "", entity.MeaningActive, nil),
				rule(entity.RuleStatus, "5", "", entity.MeaningDone, nil), rule(entity.RuleStatus, _cancelStatus, "", entity.MeaningCancelled, nil),
				rule(entity.RuleResolution, "10000", "", entity.MeaningAccepted, nil),
				rule(entity.RuleBlockedFlag, "customfield_10020", "Impediment", entity.MeaningBlocked, nil),
				rule(entity.RuleIssueType, "10001", "", "Story", nil), rule(entity.RuleIssueType, "10002", "", "Bug", nil),
				rule(entity.RuleSizeField, "customfield_10016", "", "", nil),
				rule(entity.RuleSizeValue, "customfield_10016", "S", "S", new(1)), rule(entity.RuleSizeValue, "customfield_10016", "M", "M", new(2)),
			},
		})
		require.NoError(t, err)
	}
	moscow, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	now := p.clock.Now().In(moscow)
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, moscow).AddDate(0, 0, -(int(now.Weekday())+6)%7-42)
	e := &reportEnv{seriesEnv: &seriesEnv{processEnv: p}, week: monday.UTC(), next: monday.AddDate(0, 0, 7).UTC()}
	e.collectV03(t)
	orgs, _, err := newOrgAccess(p.app, p.clock, nil, nil)
	require.NoError(t, err)
	e.calc, err = newCalc(p.app, calcModules{structure: orgs, facts: p.runner.norm, settings: p.process}, p.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	return e
}

// collectV03 writes R-A of size S done on Thursday, R-B without a size cancelled on Wednesday and R-C of size M
// done on the next Tuesday, all created on Monday and started on Tuesday of the week.
func (e *reportEnv) collectV03(t *testing.T) {
	t.Helper()
	at := e.day
	task := func(id, key, kind, status string, size *string) *envelope.Record {
		p := envelope.TaskV1{ProjectNativeId: "10000", TaskKey: &key, Title: &key, TypeNativeId: kind, StatusNativeId: status, Size: size, CreatedAt: at(0)}
		if status == "5" {
			resolution := "10000"
			p.ResolutionNativeId = &resolution
		}
		raw, err := json.Marshal(p)
		require.NoError(t, err)
		return &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/1", NativeId: id, Payload: raw}
	}
	change := func(task, changelog, field string, day int, before, after *string) *envelope.Record {
		raw, err := json.Marshal(envelope.TaskEventV1{
			TaskNativeId: task, EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: before, AfterValue: after,
		})
		require.NoError(t, err)
		when := at(day)
		return &envelope.Record{
			RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: task,
			NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &when, Payload: raw,
		}
	}
	open, active, done, cancelled, accepted, small, medium := "1", "3", "5", _cancelStatus, "10000", "S", "M"
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		task("31001", "R-A", "10001", done, &small), task("31002", "R-B", "10002", cancelled, nil), task("31003", "R-C", "10001", done, &medium))
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		change("31001", "70001", "status", 1, &open, &active), change("31001", "70002", "status", 3, &active, &done),
		change("31001", "70002", "resolution", 3, nil, &accepted),
		change("31002", "70003", "status", 1, &open, &active), change("31002", "70004", "status", 2, &active, &cancelled),
		change("31003", "70005", "status", 1, &open, &active), change("31003", "70006", "status", 8, &active, &done),
		change("31003", "70006", "resolution", 8, nil, &accepted),
	)
}

// report asks for the weekly report of the team over the two weeks of the tasks with extra query values.
func (e *reportEnv) report(t *testing.T, b *browser, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{"from": {e.week.Format(time.RFC3339)}, "to": {e.next.AddDate(0, 0, 7).Format(time.RFC3339)}}
	for k, v := range extra {
		q[k] = v
	}
	return e.checked(t, b, teamPath(e.team, "report")+"?"+q.Encode())
}

func (e *reportEnv) readReport(t *testing.T, b *browser, extra url.Values) reportBody {
	t.Helper()
	rec := e.report(t, b, extra)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[reportBody](t, rec)
}

func (e *reportEnv) cohort(t *testing.T, b *browser, revision uuid.UUID, q url.Values) cohortBody {
	t.Helper()
	rec := e.checked(t, b, "/api/v1/revisions/"+revision.String()+"/cohort?"+q.Encode())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[cohortBody](t, rec)
}

// publish asks for the report of a team without a revision, runs the chain it queued and reads the report.
func (e *reportEnv) publish(t *testing.T) reportBody {
	t.Helper()
	rec := e.report(t, e.admin, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "a series is calculated before its first report")
	e.recount(t)
	return e.readReport(t, e.admin, nil)
}

// checked reads a path and checks the answer against the contract.
func (e *reportEnv) checked(t *testing.T, b *browser, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, target, nil)
	_contractOnce.Do(func() {
		_contractDoc, _contractErr = openapi3.NewLoader().LoadFromFile("../../../docs/technical-specification/openapi.yaml")
	})
	require.NoError(t, _contractErr)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, Route: reportRoute(t, req.URL.Path)},
		Status:                 rec.Code, Header: rec.Header(), Body: io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), input), rec.Body.String())
	return rec
}

func reportRoute(t *testing.T, path string) *routers.Route {
	t.Helper()
	pattern := "/teams/{team_id}/report"
	switch {
	case path == "/api/v1/metric-definitions":
		pattern = "/metric-definitions"
	case strings.HasPrefix(path, "/api/v1/revisions/") && strings.HasSuffix(path, "/cohort"):
		pattern = "/revisions/{revision_id}/cohort"
	case strings.HasPrefix(path, "/api/v1/revisions/"):
		pattern = "/revisions/{revision_id}"
	}
	item := _contractDoc.Paths.Value(pattern)
	require.NotNil(t, item, pattern)
	return &routers.Route{Spec: _contractDoc, Path: pattern, PathItem: item, Method: http.MethodGet, Operation: item.Get}
}

func TestReport_V03ValuesCohortFiltersAndPassport(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	r := e.publish(t)

	t05 := r.value(t, "T05", "", e.week)
	assert.Equal(t, "ASSESSED", t05.State)
	assert.Equal(t, 1.0, *t05.Value, "A completes, B is cancelled")
	assert.Equal(t, 1, t05.ExcludedCount)
	assert.Equal(t, 1, *r.value(t, "T08", "p50", e.week).N)
	assert.False(t, r.AccessLimited)

	start := url.Values{"metric_id": {"T05"}, "point_start": {e.week.Format(time.RFC3339)}}
	cohort := e.cohort(t, e.admin, r.Revision.ID, start)
	assert.ElementsMatch(t, []string{"R-A:numerator", "R-B:excluded"}, cohort.keys(), "AC-FR-021")
	numerators := url.Values{"metric_id": {"T05"}, "point_start": start["point_start"], "role": {"numerator"}, "limit": {"1"}}
	page := e.cohort(t, e.admin, r.Revision.ID, numerators)
	assert.Equal(t, []string{"R-A:numerator"}, page.keys())
	assert.Nil(t, page.NextCursor)
	require.NotNil(t, page.Data[0].Object.SourceURL)
	assert.Equal(t, _probeIn.BaseURL+"/browse/R-A", *page.Data[0].Object.SourceURL)
	assert.Equal(t, "S", *page.Data[0].SizeLabel)
	assert.Equal(t, "Story", *page.Data[0].TaskType)

	outcomes := url.Values{"metric_id": {"T07"}, "point_start": start["point_start"], "role": {"numerator"}, "project_id": {e.project.String()}}
	inProject := e.cohort(t, e.admin, r.Revision.ID, outcomes)
	require.Equal(t, []string{"R-B:numerator"}, inProject.keys())
	require.NotNil(t, inProject.Data[0].ReasonCode)
	assert.Equal(t, "cancelled", *inProject.Data[0].ReasonCode)
	outcomes.Set("project_id", uuid.NewString())
	assert.Empty(t, e.cohort(t, e.admin, r.Revision.ID, outcomes).Data)

	wip := e.cohort(t, e.admin, r.Revision.ID, url.Values{"metric_id": {"T12"}, "point_start": start["point_start"]})
	i := slices.IndexFunc(wip.Data, func(it cohortItemBody) bool { return *it.Object.DisplayKey == "R-C" })
	require.GreaterOrEqual(t, i, 0, "C is in work at the end of the week")
	require.NotNil(t, wip.Data[i].Interval)
	assert.Equal(t, e.day(1), wip.Data[i].Interval.From, "the interval open at the end of the point")

	sized := e.readReport(t, e.admin, url.Values{"size_label": {"S"}})
	assert.Equal(t, 1.0, *sized.value(t, "T05", "", e.week).Value)
	assert.Equal(t, 0, sized.value(t, "T05", "", e.week).ExcludedCount, "B has no size")
	unsized := e.readReport(t, e.admin, url.Values{"size_label": {"unknown"}})
	assert.Equal(t, 0.0, *unsized.value(t, "T05", "", e.week).Value)
	assert.Equal(t, 1, unsized.value(t, "T05", "", e.week).ExcludedCount)
	bugs := e.readReport(t, e.admin, url.Values{"task_type": {"Bug"}})
	assert.Equal(t, 0, *bugs.value(t, "T08", "p50", e.week).N)
	assert.Equal(t, 1.0, *e.readReport(t, e.admin, url.Values{"task_type": {"Story"}}).value(t, "T05", "", e.next).Value, "C is a story")

	grouped := e.readReport(t, e.admin, url.Values{"group_by": {"size_label"}})
	var labels []string
	for _, m := range grouped.Metrics {
		if m.MetricID == "T05" {
			require.NotNil(t, m.Group)
			label := "none"
			if m.Group.SizeLabel != nil {
				label = *m.Group.SizeLabel
			}
			labels = append(labels, label)
		}
	}
	assert.Contains(t, labels, "S")
	assert.Contains(t, labels, "M")
	assert.Equal(t, "none", labels[len(labels)-1], "the group without a size goes last")

	passport := e.checked(t, e.admin, "/api/v1/revisions/"+r.Revision.ID.String())
	require.Equal(t, http.StatusOK, passport.Code, passport.Body.String())
	assert.Contains(t, passport.Body.String(), `"metric_id":"T08"`)
	assert.Contains(t, passport.Body.String(), `"input_kind":"mapping"`)
	defs := e.checked(t, e.admin, "/api/v1/metric-definitions?limit=5")
	require.Equal(t, http.StatusOK, defs.Code, defs.Body.String())
	assert.Contains(t, defs.Body.String(), `"next_cursor":"`)

	// A late completion of R-D in the same week gives a new revision; the first one keeps its values.
	e.collectLate(t)
	e.recount(t)
	current := e.readReport(t, e.admin, nil)
	require.NotEqual(t, r.Revision.ID, current.Revision.ID)
	assert.Equal(t, 2.0, *current.value(t, "T05", "", e.week).Value)
	old := e.readReport(t, e.admin, url.Values{"revision_id": {r.Revision.ID.String()}})
	assert.Equal(t, r.Revision.ID, old.Revision.ID)
	assert.Equal(t, 1.0, *old.value(t, "T05", "", e.week).Value)
	var first, second struct {
		KnowledgeSeq int64 `json:"knowledge_seq"`
	}
	require.NoError(t, json.Unmarshal(passport.Body.Bytes(), &first))
	again := e.checked(t, e.admin, "/api/v1/revisions/"+current.Revision.ID.String())
	require.NoError(t, json.Unmarshal(again.Body.Bytes(), &second))
	assert.Less(t, first.KnowledgeSeq, second.KnowledgeSeq, "the passport shows the slice of its own revision")
}

// collectLate writes R-D started on Tuesday and done on Wednesday of the week after the first revision.
func (e *reportEnv) collectLate(t *testing.T) {
	t.Helper()
	day := e.day
	key, done, resolution, open, active := "R-D", "5", "10000", "1", "3"
	raw, err := json.Marshal(envelope.TaskV1{
		ProjectNativeId: "10000", TaskKey: &key, Title: &key, TypeNativeId: "10001", StatusNativeId: done, ResolutionNativeId: &resolution, CreatedAt: day(0),
	})
	require.NoError(t, err)
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		&envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/1", NativeId: "31004", Payload: raw})
	event := func(changelog, field string, d int, before, after *string) *envelope.Record {
		raw, err := json.Marshal(envelope.TaskEventV1{TaskNativeId: "31004", EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: before, AfterValue: after})
		require.NoError(t, err)
		when := day(d)
		return &envelope.Record{
			RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: "31004",
			NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &when, Payload: raw,
		}
	}
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		event("70007", "status", 1, &open, &active), event("70008", "status", 2, &active, &done), event("70008", "resolution", 2, nil, &resolution))
}

func TestReport_AccessNarrowsAggregateAndCohort(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	r := e.publish(t)
	org := uuid.UUID(e.orgID)
	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	roles := []roleInput{{Role: "developer", ProjectID: &e.project}, {Role: "dev_lead", TeamID: &e.team}}
	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, rec.Header().Get("ETag"), roles).Code)
	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	e.link(t, _otherSub, "demo-person-02")

	// V-08: B moves to a project the member has no role in; the admin of the whole organization still sees it.
	closed := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO normalize.source_object
  (organization_id,id,instance_id,object_kind,native_ns,native_id,identity_quality,first_known_seq,restricted)
  VALUES ($1,$2,$3,'tracker_project','','10099','stable',1,false)`, org, closed, e.instance)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.area (organization_id,area_id,area_kind,last_change_seq) VALUES ($1,$2,'tracker_project',1)`, org, closed)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `UPDATE normalize.source_object SET current_area_id = $1 WHERE native_id = '31002' AND object_kind = 'task'`, closed)
	require.NoError(t, err)
	e.clock.Advance(61 * time.Second)

	member := e.readReport(t, e.member, nil)
	assert.Equal(t, 0, member.value(t, "T05", "", e.week).ExcludedCount, "B is not counted for the member")
	assert.Equal(t, 1.0, *member.value(t, "T05", "", e.week).Value)
	assert.True(t, member.AccessLimited)
	require.Len(t, member.AccessNotes, 1)
	assert.Equal(t, "restricted_by_access", member.AccessNotes[0].Code)
	start := url.Values{"metric_id": {"T05"}, "point_start": {e.week.Format(time.RFC3339)}}
	hidden := e.cohort(t, e.member, r.Revision.ID, start)
	assert.Equal(t, []string{"R-A:numerator"}, hidden.keys())
	assert.True(t, hidden.AccessLimited)
	admin := e.cohort(t, e.admin, r.Revision.ID, start)
	assert.ElementsMatch(t, []string{"R-A:numerator", "R-B:excluded"}, admin.keys())
	assert.False(t, admin.AccessLimited)
	assert.Equal(t, 1, e.readReport(t, e.admin, nil).value(t, "T05", "", e.week).ExcludedCount)

	// C has its own restriction: the member sees it only once the source allows it, at most 60 s later.
	_, err = e.owner.Exec(t.Context(), `UPDATE normalize.source_object SET restricted = true WHERE native_id = '31003' AND object_kind = 'task'`)
	require.NoError(t, err)
	e.clock.Advance(61 * time.Second)
	deny := testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "object", NativeID: "31003", Decision: "denied"},
		{UserID: e.adminID.String(), Decision: "allowed"},
		{UserID: e.memberID.String(), Decision: "allowed"},
	}}
	e.rights(t, deny)
	assert.Equal(t, 0.0, *e.readReport(t, e.member, nil).value(t, "T05", "", e.next).Value, "C needs its own permission")
	assert.Equal(t, 1.0, *e.readReport(t, e.admin, nil).value(t, "T05", "", e.next).Value)
	e.rights(t, testadapter.Access{Default: "denied", Rules: deny.Rules[1:]})
	assert.Equal(t, 0.0, *e.readReport(t, e.member, nil).value(t, "T05", "", e.next).Value, "a decision lives 60 s")
	e.clock.Advance(61 * time.Second)
	assert.Equal(t, 1.0, *e.readReport(t, e.member, nil).value(t, "T05", "", e.next).Value)
}

func TestReport_WithoutMappingIsNotEvaluated(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, false)
	r := e.publish(t)
	points := 0
	for _, m := range r.Metrics {
		for _, p := range m.Points {
			assert.Equal(t, "NOT_EVALUATED", p.Value.State, m.MetricID)
			assert.Equal(t, "mapping_missing", *p.Value.ReasonCode, m.MetricID)
			assert.Nil(t, p.Value.Value, "not a zero")
			points++
		}
	}
	assert.Positive(t, points)
}
