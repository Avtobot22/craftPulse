package app

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_mappingPreviews = "mapping-previews"
	_unknownStatus   = "10010"
	_cancelStatus    = "6"
)

type metricValueBody struct {
	State string   `json:"state"`
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
	N     *int     `json:"n"`
}

type previewTaskBody struct {
	Task struct {
		ID         uuid.UUID `json:"id"`
		Kind       string    `json:"kind"`
		DisplayKey *string   `json:"display_key"`
	} `json:"task"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Outcome        string     `json:"outcome"`
	BlockedSeconds *int64     `json:"blocked_seconds"`
	Unmapped       []string   `json:"unmapped"`
}

type mappingPreviewBody struct {
	AsOf   time.Time `json:"as_of"`
	Period struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"period"`
	Tasks         []previewTaskBody `json:"tasks"`
	ControlValues []struct {
		MetricID string          `json:"metric_id"`
		Value    metricValueBody `json:"value"`
	} `json:"control_values"`
	UnmappedStatuses []struct {
		InstanceID uuid.UUID `json:"instance_id"`
		NativeID   string    `json:"native_id"`
	} `json:"unmapped_statuses"`
}

func (p *mappingPreviewBody) task(t *testing.T, key string) previewTaskBody {
	t.Helper()
	i := slices.IndexFunc(p.Tasks, func(v previewTaskBody) bool { return v.Task.DisplayKey != nil && *v.Task.DisplayKey == key })
	require.GreaterOrEqual(t, i, 0, key)
	return p.Tasks[i]
}

func (p *mappingPreviewBody) keys() []string {
	out := make([]string, len(p.Tasks))
	for i, v := range p.Tasks {
		out[i] = *v.Task.DisplayKey
	}
	return out
}

func (e *processEnv) preview(t *testing.T, b *browser, body mappingDraftBody) (code int, raw []byte) {
	t.Helper()
	rec := e.json(t, b, http.MethodPost, teamPath(e.team, _mappingPreviews), nil, body)
	return rec.Code, rec.Body.Bytes()
}

// previewDraft is the mapping of V-14 for the collected project: Intake i is intake although the source puts it
// in the active category, In Progress a is active, the status x has no rule.
func (e *processEnv) previewDraft() mappingDraftBody {
	return moscowDraft(
		e.rule("status", "1", "", "intake"), e.rule("status", _intakeStatus, "", "intake"), e.rule("status", "3", "", "active"),
		e.rule("status", "5", "", "done"), e.rule("status", _cancelStatus, "", "cancelled"),
		e.rule("resolution", "10000", "", "accepted"), e.rule("blocked_flag", "customfield_10020", "Impediment", "blocked"),
	)
}

// collectV14 writes the tasks of V-14 created at d0 of the current week and the changes of two of them: A is
// created in a and done a day later, B is created in i, X in the unknown status x and D is cancelled.
func (e *processEnv) collectV14(t *testing.T, d0 time.Time) {
	t.Helper()
	task := func(id, key, status, resolution string) *envelope.Record {
		p := envelope.TaskV1{ProjectNativeId: "10000", TaskKey: &key, Title: &key, TypeNativeId: "10001", StatusNativeId: status, CreatedAt: d0}
		if resolution != "" {
			p.ResolutionNativeId = &resolution
		}
		raw, err := json.Marshal(p)
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
	active, done, cancelled, accepted := "3", "5", _cancelStatus, "10000"
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		task("30001", "V14-A", done, accepted), task("30002", "V14-B", _intakeStatus, ""),
		task("30003", "V14-X", _unknownStatus, ""), task("30004", "V14-D", cancelled, ""),
	)
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		change("30001", "50001", "status", d0.Add(26*time.Hour), &active, &done),
		change("30001", "50001", "resolution", d0.Add(26*time.Hour), nil, &accepted),
		change("30004", "50002", "status", d0.Add(2*time.Hour), &active, &cancelled),
	)
}

// rows counts what a preview must not write: versions of settings, series and revisions of calc and the audit.
func (e *processEnv) rows(t *testing.T) []int {
	t.Helper()
	tables := []string{"process.mapping_version", "process.calendar_version", "calc.series", "calc.revision", "audit.event"}
	out := make([]int, 0, len(tables))
	for _, table := range tables {
		out = append(out, count(t, t.Context(), e.owner, "SELECT count(*) FROM "+table))
	}
	return out
}

// TestPreview_V14MatchesConfirmedVersion previews the draft of V-14 on the collected tasks, confirms the same
// draft and computes the definitions by the version at the same as_of: the controls agree and the preview
// writes nothing.
func TestPreview_V14MatchesConfirmedVersion(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	moscow, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	now := e.clock.Now().In(moscow)
	monday := time.Date(now.Year(), now.Month(), now.Day()-(int(now.Weekday())+6)%7, 0, 0, 0, 0, moscow)
	d0 := monday.Add(9 * time.Hour).UTC()
	require.True(t, d0.Add(26*time.Hour).Before(e.clock.Now()), "the test clock is late enough in its week")
	e.collectV14(t, d0)
	before := e.rows(t)

	code, raw := e.preview(t, e.admin, e.previewDraft())
	require.Equal(t, http.StatusOK, code, string(raw))
	code, again := e.preview(t, e.admin, e.previewDraft())
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, string(raw), string(again), "a repeat with the same draft gives the same answer")
	assert.Equal(t, before, e.rows(t), "the preview saves no version, revision or audit record")

	var p mappingPreviewBody
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Len(t, p.Tasks, 11, "the seven tasks of the dataset and the four of V-14, fewer than 20")
	a := p.task(t, "V14-A")
	require.NotNil(t, a.StartedAt)
	assert.Equal(t, d0, a.StartedAt.UTC(), "start(A) = d0")
	assert.Equal(t, "completed", a.Outcome)
	b := p.task(t, "V14-B")
	assert.Nil(t, b.StartedAt, "intake with the active category of the source gives no start")
	assert.Equal(t, "open", b.Outcome)
	x := p.task(t, "V14-X")
	assert.Equal(t, []string{_unknownStatus}, x.Unmapped)
	assert.Equal(t, "unknown", x.Outcome)
	assert.Contains(t, p.UnmappedStatuses, struct {
		InstanceID uuid.UUID `json:"instance_id"`
		NativeID   string    `json:"native_id"`
	}{e.instance, _unknownStatus})
	assert.Equal(t, "cancelled", p.task(t, "V14-D").Outcome)
	assert.Positive(t, *p.task(t, "PA-3").BlockedSeconds, "PA-3 carries the impediment flag")
	assert.Equal(t, "task", a.Task.Kind)

	rec := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), e.previewDraft())
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	uc := calc.New(calc.Deps{Structure: orgs, Facts: e.runner.norm, Settings: e.process, Clock: e.clock})
	r, err := uc.Calculate(t.Context(), entity.CalcRequest{
		OrganizationID: e.orgID, TeamID: entity.ID(e.team), KnowledgeSeq: e.lastSeq(t), AsOf: p.AsOf,
		Window: entity.Window{From: p.Period.From, To: p.Period.To, Step: entity.StepWeek},
	})
	require.NoError(t, err)
	require.Len(t, p.ControlValues, 3)
	for i, c := range []struct{ metric, estimator string }{{"T05", ""}, {"T08", calc.EstimatorP50}, {"T12", ""}} {
		got := p.ControlValues[i]
		assert.Equal(t, c.metric, got.MetricID)
		j := indexSeries(r.Series, c.metric, c.estimator)
		require.GreaterOrEqual(t, j, 0, c.metric)
		require.Len(t, r.Series[j].Points, 1)
		want := r.Series[j].Points[0]
		assert.Equal(t, string(want.State), got.Value.State, c.metric)
		assert.Equal(t, want.Value, got.Value.Value, c.metric)
		assert.Equal(t, want.N, got.Value.N, c.metric)
		assert.Equal(t, r.Series[j].Unit, got.Value.Unit, c.metric)
	}
	assert.Equal(t, 1.0, *p.ControlValues[0].Value.Value, "A is the only task completed this week")
	assert.Equal(t, 26*3600.0, *p.ControlValues[1].Value.Value, "the cycle of A")
}

func TestPreview_ErrorsAndRights(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	code, raw := e.preview(t, e.admin, e.previewDraft())
	assert.Equal(t, http.StatusForbidden, code, "an administrator without process_owner: %s", raw)
	e.grantProcessOwner(t, e.adminID)
	setup := emptyStructure()
	setup.Teams = []named{{Name: "Elsewhere", Status: "active"}}
	rec := e.replace(t, e.loginEnv.admin(t, "other"), `"0"`, setup)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	elsewhere := decode[structureBody](t, rec).team("Elsewhere")
	rec = e.json(t, e.admin, http.MethodPost, teamPath(elsewhere, _mappingPreviews), nil, e.previewDraft())
	assert.Equal(t, http.StatusNotFound, rec.Code, "a team of another organization")

	foreign := e.rule("status", "1", "", "intake")
	foreign.InstanceID = uuid.New()
	for _, tt := range []struct {
		name      string
		give      mappingDraftBody
		wantField string
		wantCode  string
	}{
		{"unknown instance", moscowDraft(foreign), "/rules/0/instance_id", entity.FieldOutsideScope},
		{"meaning not allowed", moscowDraft(e.rule("status", "3", "", "accepted")), "/rules/0/meaning", entity.FieldNotAllowedValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, raw := e.preview(t, e.admin, tt.give)
			require.Equal(t, http.StatusUnprocessableEntity, code, string(raw))
			var p problemBody
			require.NoError(t, json.Unmarshal(raw, &p))
			fields := map[string]string{}
			for _, f := range p.Errors {
				fields[f.Field] = f.Code
			}
			assert.Equal(t, tt.wantCode, fields[tt.wantField], string(raw))
		})
	}

	e.grantProcessOwner(t, e.memberID)
	e.link(t, _otherSub, "demo-person-02")
	_, err := e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET restricted = true WHERE object_kind = 'task' AND native_id = '20002'")
	require.NoError(t, err)
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "object", NativeID: "20002", Decision: "denied", ReasonCode: "security_level"},
		{UserID: e.memberID.String(), Decision: "allowed"},
		{UserID: e.adminID.String(), Decision: "allowed"},
	}})
	keys := func(b *browser) []string {
		code, raw := e.preview(t, b, e.previewDraft())
		require.Equal(t, http.StatusOK, code, string(raw))
		var p mappingPreviewBody
		require.NoError(t, json.Unmarshal(raw, &p))
		return p.keys()
	}
	assert.Contains(t, keys(e.admin), "PA-2")
	member := keys(e.member)
	assert.NotEmpty(t, member)
	assert.NotContains(t, member, "PA-2", "a task the source closes for the user is not in the sample")

	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{{UserID: e.adminID.String(), Decision: "allowed"}}})
	e.clock.Advance(61 * time.Second)
	assert.Empty(t, keys(e.member), "the tasks of a project closed for the user are not given")
	assert.Len(t, keys(e.admin), 7, "the administrator keeps them")
}
