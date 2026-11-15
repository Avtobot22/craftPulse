package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_targetVersions  = "target-versions"
	_targetBaselines = "target-baselines"
	_day             = 86400
)

var _baselinePeriod = struct{ from, to time.Time }{
	from: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
}

type periodBody struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type targetDraftBody struct {
	TaskType        string      `json:"task_type"`
	SizeLabel       string      `json:"size_label"`
	DurationSeconds int64       `json:"duration_seconds"`
	Basis           string      `json:"basis"`
	BasisRevisionID *uuid.UUID  `json:"basis_revision_id"`
	BasisPeriod     *periodBody `json:"basis_period"`
}

type targetVersionBody struct {
	TaskType          string      `json:"task_type"`
	SizeLabel         string      `json:"size_label"`
	VersionNo         int         `json:"version_no"`
	BaseVersionNo     *int        `json:"base_version_no"`
	DurationSeconds   int64       `json:"duration_seconds"`
	Basis             string      `json:"basis"`
	BasisRevisionID   *uuid.UUID  `json:"basis_revision_id"`
	BasisValueSeconds *int64      `json:"basis_value_seconds"`
	BasisN            *int        `json:"basis_n"`
	BasisPeriod       *periodBody `json:"basis_period"`
	ConfirmedBy       uuid.UUID   `json:"confirmed_by"`
}

type baselineListBody struct {
	RevisionID uuid.UUID `json:"revision_id"`
	Data       []struct {
		TaskType  string  `json:"task_type"`
		SizeLabel *string `json:"size_label"`
		P85       struct {
			State      string   `json:"state"`
			Value      *float64 `json:"value"`
			N          *int     `json:"n"`
			LowSample  bool     `json:"low_sample"`
			ReasonCode *string  `json:"reason_code"`
		} `json:"p85"`
	} `json:"data"`
}

// targetEnv is the team of processEnv with types and sizes in its mapping, two sized tasks of V-16 and the
// weekly series of its flow.
type targetEnv struct {
	*seriesEnv
}

func newTargetEnv(t *testing.T) *targetEnv {
	t.Helper()
	p := newProcessEnv(t)
	p.grantProcessOwner(t, p.adminID)
	rule := func(kind entity.RuleKind, ref, value, meaning string, rank *int) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(p.instance), NativeRef: ref, NativeValue: value, Meaning: meaning, Rank: rank}
	}
	admin := entity.Subject{OrganizationID: p.orgID, UserID: entity.ID(p.adminID)}
	_, _, err := p.process.CreateMappingVersion(t.Context(), admin, entity.ID(p.team), 0, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "UTC", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", "", entity.MeaningIntake, nil), rule(entity.RuleStatus, "3", "", entity.MeaningActive, nil),
			rule(entity.RuleStatus, "5", "", entity.MeaningDone, nil), rule(entity.RuleResolution, "10000", "", entity.MeaningAccepted, nil),
			rule(entity.RuleIssueType, "10001", "", "Story", nil), rule(entity.RuleSizeField, "customfield_10016", "", "", nil),
			rule(entity.RuleSizeValue, "customfield_10016", "S", "S", new(1)), rule(entity.RuleSizeValue, "customfield_10016", "M", "M", new(2)),
		},
	})
	require.NoError(t, err)
	p.collectSized(t)
	orgs, _, err := newOrgAccess(p.app, p.clock, nil, nil)
	require.NoError(t, err)
	c, err := newCalc(p.app, calcModules{structure: orgs, facts: p.runner.norm, settings: p.process}, p.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	series, err := c.TeamSeries(t.Context(), p.orgID, entity.ID(p.team), entity.StepWeek)
	require.NoError(t, err)
	return &targetEnv{seriesEnv: &seriesEnv{processEnv: p, calc: c, series: series}}
}

// collectSized writes TG-1, created at work in size S on February 3 and resized to M the next day, done after
// three days, and TG-2 of size S done after two days (V-16).
func (e *processEnv) collectSized(t *testing.T) {
	t.Helper()
	at := func(day, hour int) time.Time { return time.Date(2026, 2, day, hour, 0, 0, 0, time.UTC) }
	task := func(id, key, size string, created time.Time) *envelope.Record {
		status, resolution := "5", "10000"
		raw, err := json.Marshal(envelope.TaskV1{
			ProjectNativeId: "10000", TaskKey: &key, Title: &key, TypeNativeId: "10001", StatusNativeId: status,
			ResolutionNativeId: &resolution, Size: &size, CreatedAt: created,
		})
		require.NoError(t, err)
		return &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/1", NativeId: id, Payload: raw}
	}
	change := func(task, changelog, field string, at time.Time, before, after *string) *envelope.Record {
		raw, err := json.Marshal(envelope.TaskEventV1{
			TaskNativeId: task, EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: before, AfterValue: after,
		})
		require.NoError(t, err)
		return &envelope.Record{
			RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: task,
			NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &at, Payload: raw,
		}
	}
	active, done, accepted, small, medium := "3", "5", "10000", "S", "M"
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		task("30101", "TG-1", medium, at(3, 9)), task("30102", "TG-2", small, at(10, 9)))
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		change("30101", "60001", "customfield_10016", at(4, 9), &small, &medium),
		change("30101", "60002", "status", at(6, 9), &active, &done), change("30101", "60002", "resolution", at(6, 9), nil, &accepted),
		change("30102", "60003", "status", at(12, 9), &active, &done), change("30102", "60003", "resolution", at(12, 9), nil, &accepted),
	)
}

func (e *targetEnv) baselines(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{"from": {_baselinePeriod.from.Format(time.RFC3339)}, "to": {_baselinePeriod.to.Format(time.RFC3339)}}
	return e.do(t, e.admin, http.MethodGet, teamPath(e.team, _targetBaselines)+"?"+q.Encode(), nil)
}

func (e *targetEnv) targets(t *testing.T, b *browser, group bool) (page versionPageBody[targetVersionBody], tag string) {
	t.Helper()
	path := teamPath(e.team, _targetVersions)
	if group {
		path += "?" + url.Values{"task_type": {"Story"}, "size_label": {"S"}}.Encode()
	}
	rec := e.do(t, b, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[versionPageBody[targetVersionBody]](t, rec), rec.Header().Get("ETag")
}

func (e *targetEnv) queued(t *testing.T) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = '"+entity.KindCalcRecount+"' AND state = 'available'")
}

// targetInputs are the versions of targets a revision was computed with, as ID@number.
func (e *targetEnv) targetInputs(t *testing.T, revision entity.ID) []string {
	t.Helper()
	inputs, err := e.calc.RevisionInputs(t.Context(), e.orgID, revision)
	require.NoError(t, err)
	var out []string
	for _, in := range inputs {
		if in.Kind == entity.InputKindTarget {
			out = append(out, in.ID+"@"+in.Version)
		}
	}
	return out
}

func manualTarget(seconds int64) targetDraftBody {
	return targetDraftBody{TaskType: "Story", SizeLabel: "S", DurationSeconds: seconds, Basis: entity.TargetManual}
}

// TestTarget_V16BaselineAndVersions reads the historical p85 of the groups, confirms v1 and v2 of Story S by
// V-07, recounts the series after each of them and keeps the basis of a p85 after its revision is deleted.
func TestTarget_V16BaselineAndVersions(t *testing.T) {
	t.Parallel()
	e := newTargetEnv(t)
	assert.Equal(t, http.StatusNotFound, e.baselines(t).Code, "no revision of the series yet")
	r0 := e.build(t)

	rec := e.baselines(t)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	b := decode[baselineListBody](t, rec)
	assert.Equal(t, uuid.UUID(r0.ID), b.RevisionID)
	require.Len(t, b.Data, 2, "Story S and Story without a size; tasks without a type rule are in no group")
	sized, unsized := b.Data[0], b.Data[1]
	assert.Equal(t, "Story", sized.TaskType)
	require.NotNil(t, sized.SizeLabel)
	assert.Equal(t, "S", *sized.SizeLabel, "TG-1 counts by its size at start although it became M")
	assert.Equal(t, "ASSESSED", sized.P85.State)
	assert.Equal(t, 3.0*_day, *sized.P85.Value, "p85 of 2 and 3 days is 3 days")
	assert.Equal(t, 2, *sized.P85.N)
	assert.True(t, sized.P85.LowSample)
	assert.Nil(t, unsized.SizeLabel)
	assert.Equal(t, "size_unknown", *unsized.P85.ReasonCode)
	assert.Equal(t, 1, *unsized.P85.N, "PA-5 was done on March 3")

	_, tag := e.targets(t, e.admin, true)
	require.Equal(t, `"0"`, tag, "the group has no version yet")
	before := e.queued(t)
	v1 := e.confirm(t, e.admin, _targetVersions, tag, uuid.New(), manualTarget(2*_day))
	require.Equal(t, http.StatusCreated, v1.Code, v1.Body.String())
	assert.Equal(t, `"1"`, v1.Header().Get("ETag"))
	assert.Equal(t, before+1, e.queued(t), "the confirmation queues the recount of the series with it")
	e.recount(t)
	r1 := e.current(t)
	require.Len(t, e.targetInputs(t, r1.ID), 1)
	assert.Empty(t, e.targetInputs(t, r0.ID), "the earlier revision keeps the settings it was computed with")

	_, adminTag := e.targets(t, e.admin, true)
	e.grantProcessOwner(t, e.memberID)
	_, memberTag := e.targets(t, e.member, true)
	require.Equal(t, `"1"`, adminTag)
	require.Equal(t, `"1"`, memberTag, "both authors read v1")
	page, _ := e.targets(t, e.admin, false)
	require.Len(t, page.Data, 1, "a cancelled form leaves v1 = 2 days")
	assert.Equal(t, int64(2*_day), page.Data[0].DurationSeconds)

	key := uuid.New()
	first := e.confirm(t, e.admin, _targetVersions, adminTag, key, manualTarget(4*_day))
	second := e.confirm(t, e.admin, _targetVersions, adminTag, key, manualTarget(4*_day))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	require.Equal(t, http.StatusCreated, second.Code, "a repeat of the action returns the saved version")
	v2 := decode[targetVersionBody](t, first)
	assert.Equal(t, v2, decode[targetVersionBody](t, second))
	assert.Equal(t, 2, v2.VersionNo)
	assert.Equal(t, 1, *v2.BaseVersionNo)
	assert.Len(t, e.audit(t, "process.target.confirm"), 2, "v1 and one v2 are in the audit")
	stale := e.confirm(t, e.member, _targetVersions, memberTag, uuid.New(), manualTarget(3*_day))
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code, "the second author with the form of v1")
	equal := e.confirm(t, e.member, _targetVersions, `"2"`, uuid.New(), manualTarget(4*_day))
	assert.Equal(t, http.StatusOK, equal.Code, "a form equal to v2 keeps it")
	page, _ = e.targets(t, e.admin, false)
	require.Len(t, page.Data, 2)

	e.recount(t)
	r2 := e.current(t)
	require.NotEqual(t, r1.ID, r2.ID)
	assert.NotEqual(t, e.targetInputs(t, r1.ID), e.targetInputs(t, r2.ID), "v2 has its own revision")
	assert.Len(t, e.targetInputs(t, r1.ID), 1, "the revision of v1 keeps v1")

	r1ID := uuid.UUID(r1.ID)
	p85 := targetDraftBody{
		TaskType: "Story", SizeLabel: "S", DurationSeconds: 3 * _day, Basis: entity.TargetHistoricalP85, BasisRevisionID: &r1ID,
		BasisPeriod: &periodBody{From: _baselinePeriod.from, To: _baselinePeriod.to},
	}
	v3 := e.confirm(t, e.admin, _targetVersions, `"2"`, uuid.New(), p85)
	require.Equal(t, http.StatusCreated, v3.Code, v3.Body.String())
	saved := decode[targetVersionBody](t, v3)
	_, err := e.owner.Exec(t.Context(), "DELETE FROM calc.revision WHERE id = $1", r1ID)
	require.NoError(t, err, "the revision of the basis is deleted by its term")
	page, _ = e.targets(t, e.admin, true)
	require.Len(t, page.Data, 3)
	kept := page.Data[0]
	assert.Equal(t, saved, kept)
	assert.Equal(t, 3, kept.VersionNo)
	assert.Equal(t, r1ID, *kept.BasisRevisionID)
	assert.Equal(t, int64(3*_day), *kept.BasisValueSeconds, "the value of the basis stays")
	assert.Equal(t, 2, *kept.BasisN)
	assert.Equal(t, periodBody{From: _baselinePeriod.from, To: _baselinePeriod.to}, *kept.BasisPeriod)
	versions := []int{page.Data[0].VersionNo, page.Data[1].VersionNo, page.Data[2].VersionNo}
	assert.True(t, slices.IsSortedFunc(versions, func(a, b int) int { return b - a }), "newest first")
}

func TestTarget_DraftErrors(t *testing.T) {
	t.Parallel()
	e := newTargetEnv(t)
	e.build(t)
	unknown := uuid.New()
	period := &periodBody{From: _baselinePeriod.from, To: _baselinePeriod.to}
	tests := []struct {
		name      string
		give      targetDraftBody
		wantField string
		wantCode  string
	}{
		{"zero target", manualTarget(0), "/duration_seconds", entity.FieldOutOfRange},
		{"no size", targetDraftBody{TaskType: "Story", DurationSeconds: _day, Basis: entity.TargetManual}, "/size_label", entity.FieldRequired},
		{
			"p85 without a revision",
			targetDraftBody{TaskType: "Story", SizeLabel: "S", DurationSeconds: _day, Basis: entity.TargetHistoricalP85, BasisPeriod: period},
			"/basis_revision_id", entity.FieldRequired,
		},
		{"p85 of an unknown revision", targetDraftBody{
			TaskType: "Story", SizeLabel: "S", DurationSeconds: _day, Basis: entity.TargetHistoricalP85,
			BasisRevisionID: &unknown, BasisPeriod: period,
		}, "/basis_revision_id", entity.FieldUnknownReference},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.confirm(t, e.admin, _targetVersions, `"0"`, uuid.New(), tt.give)
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			p := decode[problemBody](t, rec)
			fields := make(map[string]string, len(p.Errors))
			for _, f := range p.Errors {
				fields[f.Field] = f.Code
			}
			assert.Equal(t, tt.wantCode, fields[tt.wantField], rec.Body.String())
		})
	}
	noIfMatch := e.json(t, e.admin, http.MethodPost, teamPath(e.team, _targetVersions), http.Header{"Idempotency-Key": {uuid.NewString()}}, manualTarget(_day))
	assert.Equal(t, http.StatusPreconditionRequired, noIfMatch.Code)
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM process.target_version"))
}
