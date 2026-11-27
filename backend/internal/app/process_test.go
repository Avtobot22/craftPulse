package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/process"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_mappingVersions  = "mapping-versions"
	_calendarVersions = "calendar-versions"
	_intakeStatus     = "10001"
)

type mappingRuleBody struct {
	RuleKind    string    `json:"rule_kind"`
	InstanceID  uuid.UUID `json:"instance_id"`
	NativeRef   string    `json:"native_ref"`
	NativeValue string    `json:"native_value"`
	Meaning     string    `json:"meaning"`
	Rank        *int      `json:"rank"`
}

type mappingDraftBody struct {
	Timezone          string            `json:"timezone"`
	CalendarVersionNo *int              `json:"calendar_version_no"`
	Rules             []mappingRuleBody `json:"rules"`
}

type mappingVersionBody struct {
	VersionNo         int               `json:"version_no"`
	BaseVersionNo     *int              `json:"base_version_no"`
	Timezone          string            `json:"timezone"`
	CalendarVersionNo *int              `json:"calendar_version_no"`
	Rules             []mappingRuleBody `json:"rules"`
	ConfirmedBy       uuid.UUID         `json:"confirmed_by"`
}

type calendarDraftBody struct {
	Timezone  string   `json:"timezone"`
	Workdays  []string `json:"workdays"`
	WorkStart *string  `json:"work_start"`
	WorkEnd   *string  `json:"work_end"`
	Holidays  []string `json:"holidays"`
}

type calendarVersionBody struct {
	calendarDraftBody
	VersionNo     int  `json:"version_no"`
	BaseVersionNo *int `json:"base_version_no"`
}

type versionPageBody[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

type catalogEntryBody struct {
	NativeID       string  `json:"native_id"`
	Name           string  `json:"name"`
	SourceCategory *string `json:"source_category"`
	ValueType      string  `json:"value_type"`
}

type workflowCatalogBody struct {
	Instances []struct {
		InstanceID  uuid.UUID          `json:"instance_id"`
		Provider    string             `json:"provider"`
		Statuses    []catalogEntryBody `json:"statuses"`
		Resolutions []catalogEntryBody `json:"resolutions"`
		IssueTypes  []catalogEntryBody `json:"issue_types"`
		Fields      []catalogEntryBody `json:"fields"`
	} `json:"instances"`
	UnmappedStatuses []struct {
		InstanceID uuid.UUID `json:"instance_id"`
		NativeID   string    `json:"native_id"`
	} `json:"unmapped_statuses"`
}

// processEnv is the collected Jira project 10000 of the project Core with the dictionaries of a page of the
// test adapter, and the team Platform owning Core. Nobody owns its process until a test grants the role.
type processEnv struct {
	*accessEnv
	team    uuid.UUID
	process *process.UseCase
}

func newProcessEnv(t *testing.T) *processEnv {
	t.Helper()
	e := &processEnv{accessEnv: newAccessEnv(t)}
	e.collectDictionaries(t)

	st, tag := e.structure(t, e.admin)
	in := st.input()
	in.Teams = append(in.Teams, named{Name: "Platform", Status: "active"})
	rec := e.replace(t, e.admin, tag, in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	created := decode[structureBody](t, rec)
	e.team = created.team("Platform")
	in = created.input()
	in.TeamProjects = append(in.TeamProjects, link{TeamID: e.team, ProjectID: &e.project})
	rec = e.replace(t, e.admin, rec.Header().Get("ETag"), in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	e.process, err = newProcess(e.app, orgs, e.runner.norm, e.clock)
	require.NoError(t, err)
	return e
}

// collectDictionaries writes a sync page of the connection with statuses, a resolution, a type and a field, as the
// Jira adapter sends them, and normalizes it.
func (e *processEnv) collectDictionaries(t *testing.T) {
	t.Helper()
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		dictionaryRecord(t, "task_status", "1", envelope.TaskStatusV1{Name: "Open", Category: envelope.TaskStatusV1CategoryNew}),
		dictionaryRecord(t, "task_status", "3", envelope.TaskStatusV1{Name: "In Progress", Category: envelope.TaskStatusV1CategoryIndeterminate}),
		dictionaryRecord(t, "task_status", _intakeStatus, envelope.TaskStatusV1{Name: "Intake", Category: envelope.TaskStatusV1CategoryIndeterminate}),
		dictionaryRecord(t, "task_resolution", "10000", envelope.TaskResolutionV1{Name: "Done"}),
		dictionaryRecord(t, "task_type", "10004", envelope.TaskTypeV1{Name: "Bug"}),
		dictionaryRecord(t, "task_field", "customfield_10016", envelope.TaskFieldV1{Name: "Story Points", ValueType: "number", Custom: true}),
	)
}

// collect writes a sync page of the connection of the test adapter and normalizes it; the members sign in again
// after the clock moved to the times of the jobs.
func (e *processEnv) collect(t *testing.T, m *envelope.Manifest, recs ...*envelope.Record) {
	t.Helper()
	require.Equal(t, entity.ReceiptAccepted, e.collectVia(t, "", m, recs...))
}

func (e *processEnv) collectVia(t *testing.T, via entity.ObservedVia, m *envelope.Manifest, recs ...*envelope.Record) entity.ReceiptStatus {
	t.Helper()
	caller, err := adapterapi.NewCaller(adapterapi.Config{Resolver: mapResolver{}})
	require.NoError(t, err)
	conn, err := newConnect(e.app, e.keyring, caller, e.clock)
	require.NoError(t, err)
	orgs, _, err := newOrgAccess(e.app, e.clock, conn, nil)
	require.NoError(t, err)
	intake, err := newIngest(e.app, sharedStore(t), conn, e.connectionsEnv.rights, orgs, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	var scope uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM connect.connection_scope WHERE connection_id = $1", e.conn).Scan(&scope))

	p, err := intake.PreparePage(t.Context(), e.orgID, entity.ID(e.conn), entity.ID(scope), bytes.NewReader(pack(t, m, recs...)))
	require.NoError(t, err)
	p.ObservedVia = via
	var rc entity.Receipt
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		var err error
		rc, err = intake.RecordPage(ctx, p)
		return err
	}))
	e.settle(t)
	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	return rc.Status
}

func dictionaryRecord(t *testing.T, kind, id string, payload any) *envelope.Record {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassObject, RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: kind + "/1",
		NativeId: id, Payload: raw,
	}
}

// grantProcessOwner adds the role of the process owner of the team to the roles the user has.
func (e *processEnv) grantProcessOwner(t *testing.T, user uuid.UUID) {
	t.Helper()
	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+user.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	roles := decode[struct {
		Data []roleInput `json:"data"`
	}](t, rec).Data
	roles = append(roles, roleInput{Role: "process_owner", TeamID: &e.team})
	granted := e.roles(t, e.admin, user, rec.Header().Get("ETag"), roles)
	require.Equal(t, http.StatusOK, granted.Code, granted.Body.String())
}

func teamPath(team uuid.UUID, resource string) string {
	return "/api/v1/teams/" + team.String() + "/" + resource
}

func (e *processEnv) confirm(t *testing.T, b *browser, resource, ifMatch string, key uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	h := http.Header{"Idempotency-Key": {key.String()}}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return e.json(t, b, http.MethodPost, teamPath(e.team, resource), h, body)
}

func (e *processEnv) mappings(t *testing.T, b *browser) (page versionPageBody[mappingVersionBody], etag string) {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, teamPath(e.team, _mappingVersions), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[versionPageBody[mappingVersionBody]](t, rec), rec.Header().Get("ETag")
}

func (e *processEnv) catalog(t *testing.T, b *browser) workflowCatalogBody {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, teamPath(e.team, "workflow-catalog"), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[workflowCatalogBody](t, rec)
}

func (e *processEnv) rule(kind, ref, value, meaning string) mappingRuleBody {
	return mappingRuleBody{RuleKind: kind, InstanceID: e.instance, NativeRef: ref, NativeValue: value, Meaning: meaning}
}

func moscowDraft(rules ...mappingRuleBody) mappingDraftBody {
	return mappingDraftBody{Timezone: "Europe/Moscow", Rules: rules}
}

func (e *processEnv) lastSeq(t *testing.T) int64 {
	t.Helper()
	var seq int64
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT last_seq FROM platform.org_clock WHERE organization_id = $1",
		uuid.UUID(e.orgID)).Scan(&seq))
	return seq
}

func TestProcess_WorkflowCatalogListsUnmappedStatuses(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)

	cat := e.catalog(t, e.admin)
	require.Len(t, cat.Instances, 1)
	inst := cat.Instances[0]
	assert.Equal(t, e.instance, inst.InstanceID)
	assert.Equal(t, "jira_cloud", inst.Provider)
	require.Len(t, inst.Statuses, 3)
	intake := inst.Statuses[1]
	assert.Equal(t, _intakeStatus, intake.NativeID)
	require.NotNil(t, intake.SourceCategory)
	assert.Equal(t, "indeterminate", *intake.SourceCategory, "the category of the source is shown as a hint")
	assert.Equal(t, []catalogEntryBody{{NativeID: "10000", Name: "Done"}}, inst.Resolutions)
	assert.Equal(t, []catalogEntryBody{{NativeID: "10004", Name: "Bug"}}, inst.IssueTypes)
	assert.Equal(t, []catalogEntryBody{{NativeID: "customfield_10016", Name: "Story Points", ValueType: "number"}}, inst.Fields)
	assert.Len(t, cat.UnmappedStatuses, 3, "no version maps a status yet")

	rec := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), moscowDraft(e.rule("status", "1", "", "intake"), e.rule("status", "3", "", "active")))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	cat = e.catalog(t, e.admin)
	require.Len(t, cat.UnmappedStatuses, 1)
	assert.Equal(t, e.instance, cat.UnmappedStatuses[0].InstanceID)
	assert.Equal(t, _intakeStatus, cat.UnmappedStatuses[0].NativeID, "Intake stays unknown until it has a rule (FR-017 A3)")
}

func TestProcess_StaleFormConflicts(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	e.grantProcessOwner(t, e.memberID)
	first := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), moscowDraft(e.rule("status", "1", "", "intake")))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	_, adminTag := e.mappings(t, e.admin)
	_, memberTag := e.mappings(t, e.member)
	require.Equal(t, `"1"`, adminTag)
	require.Equal(t, `"1"`, memberTag, "both authors read version 1")

	second := e.confirm(t, e.member, _mappingVersions, memberTag, uuid.New(), moscowDraft(e.rule("status", "1", "", "intake"), e.rule("status", "3", "", "active")))
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	v2 := decode[mappingVersionBody](t, second)
	assert.Equal(t, 2, v2.VersionNo)
	assert.Equal(t, 1, *v2.BaseVersionNo)
	assert.Equal(t, `"2"`, second.Header().Get("ETag"))

	stale := e.confirm(t, e.admin, _mappingVersions, adminTag, uuid.New(), moscowDraft(e.rule("status", "1", "", "wait")))
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code, stale.Body.String())
	assert.Equal(t, "version_conflict", problemCode(t, stale), "AC-FR-008, V-07")
	page, tag := e.mappings(t, e.admin)
	assert.Equal(t, `"2"`, tag)
	require.Len(t, page.Data, 2)
	assert.Equal(t, v2, page.Data[0], "the stale form changed nothing")

	third := e.confirm(t, e.admin, _mappingVersions, tag, uuid.New(), moscowDraft(e.rule("status", "1", "", "wait"), e.rule("status", "3", "", "active")))
	require.Equal(t, http.StatusCreated, third.Code, third.Body.String())
	v3 := decode[mappingVersionBody](t, third)
	assert.Equal(t, 3, v3.VersionNo)
	assert.Equal(t, 2, *v3.BaseVersionNo)
	assert.Equal(t, e.adminID, v3.ConfirmedBy)
	assert.Len(t, e.audit(t, "process.mapping.confirm"), 3, "every confirmation is in the audit (DR-004)")
}

func TestProcess_ParallelFormsOfOneVersion(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	authors := []entity.Subject{
		{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)},
		{OrganizationID: e.orgID, UserID: entity.ID(e.memberID)},
	}
	meanings := []string{entity.MeaningActive, entity.MeaningWait}
	errs := make([]error, len(authors))
	var wg sync.WaitGroup
	for i := range authors {
		wg.Go(func() {
			d := entity.MappingDraft{Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
				{Kind: entity.RuleStatus, InstanceID: entity.ID(e.instance), NativeRef: "3", Meaning: meanings[i]},
			}}
			_, _, errs[i] = e.process.CreateMappingVersion(t.Context(), authors[i], entity.ID(e.team), 0, entity.ID(uuid.New()), d)
		})
	}
	wg.Wait()

	conflicts := 0
	for _, err := range errs {
		if errors.Is(err, entity.ErrVersionConflict) {
			conflicts++
			continue
		}
		require.NoError(t, err)
	}
	assert.Equal(t, 1, conflicts, "one of two forms of version 0 wins")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM process.mapping_version"))
}

func TestProcess_RepeatAndEqualContentKeepVersion(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	e.grantProcessOwner(t, e.memberID)
	key := uuid.New()
	body := moscowDraft(e.rule("status", "3", "", "active"), e.rule("resolution", "10000", "", "accepted"))

	created := e.confirm(t, e.admin, _mappingVersions, `"0"`, key, body)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	saved := decode[mappingVersionBody](t, created)
	repeated := e.confirm(t, e.admin, _mappingVersions, `"0"`, key, body)
	require.Equal(t, http.StatusCreated, repeated.Code, "a repeat of the key returns the saved version")
	assert.Equal(t, saved, decode[mappingVersionBody](t, repeated))
	late := e.confirm(t, e.admin, _mappingVersions, `"7"`, key, body)
	assert.Equal(t, http.StatusCreated, late.Code, "the repeat is answered before If-Match")

	other := e.confirm(t, e.admin, _mappingVersions, `"1"`, key, moscowDraft(e.rule("status", "3", "", "wait")))
	assert.Equal(t, http.StatusUnprocessableEntity, other.Code)
	assert.Equal(t, "idempotency_key_reused", problemCode(t, other))
	foreign := e.confirm(t, e.member, _mappingVersions, `"1"`, key, body)
	assert.Equal(t, "idempotency_key_reused", problemCode(t, foreign), "the key belongs to its author")

	reordered := moscowDraft(body.Rules[1], body.Rules[0])
	equal := e.confirm(t, e.member, _mappingVersions, `"1"`, uuid.New(), reordered)
	require.Equal(t, http.StatusOK, equal.Code, equal.Body.String())
	assert.Equal(t, saved, decode[mappingVersionBody](t, equal), "the same content in another order is the current version (AC-FR-007)")
	assert.Equal(t, `"1"`, equal.Header().Get("ETag"))

	page, tag := e.mappings(t, e.admin)
	assert.Equal(t, `"1"`, tag)
	require.Len(t, page.Data, 1, "repeats and a form cancelled on the client leave the version in force")
	assert.Len(t, e.audit(t, "process.mapping.confirm"), 1)
}

func TestProcess_DraftErrors(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	foreign := e.rule("status", "1", "", "intake")
	foreign.InstanceID = uuid.New()
	size := e.rule("size_value", "customfield_10016", "5", "M")
	team := e.rule("team_value", "customfield_10020", "core", "Core team")

	tests := []struct {
		name      string
		give      mappingDraftBody
		wantField string
		wantCode  string
	}{
		{"instance outside the team", moscowDraft(foreign), "/rules/0/instance_id", entity.FieldOutsideScope},
		{"meaning of another kind", moscowDraft(e.rule("status", "3", "", "accepted")), "/rules/0/meaning", entity.FieldNotAllowedValue},
		{"resolution meaning", moscowDraft(e.rule("resolution", "10000", "", "active")), "/rules/0/meaning", entity.FieldNotAllowedValue},
		{"code required with a value", moscowDraft(e.rule("code_required_type", "10004", "x", "code_required")), "/rules/0/native_value", entity.FieldNotAllowedValue},
		{"blocked flag without a value", moscowDraft(e.rule("blocked_flag", "customfield_10021", "", "blocked")), "/rules/0/native_value", entity.FieldRequired},
		{"size value without a rank", moscowDraft(size), "/rules/0/rank", entity.FieldRequired},
		{"team value without a team ID", moscowDraft(team), "/rules/0/meaning", entity.FieldInvalidFormat},
		{"rule of M1", moscowDraft(e.rule("acceptance_gate", "3", "", "acceptance")), "/rules/0/rule_kind", entity.FieldNotAllowedValue},
		{"repeated rule", moscowDraft(e.rule("status", "3", "", "active"), e.rule("status", "3", "", "wait")), "/rules/1", entity.FieldDuplicate},
		{"unknown time zone", mappingDraftBody{Timezone: "Mars/Olympus", Rules: []mappingRuleBody{}}, "/timezone", entity.FieldNotAllowedValue},
		{"unknown calendar", mappingDraftBody{Timezone: "UTC", CalendarVersionNo: new(4), Rules: []mappingRuleBody{}}, "/calendar_version_no", entity.FieldUnknownReference},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), tt.give)
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			p := decode[problemBody](t, rec)
			fields := make(map[string]string, len(p.Errors))
			for _, f := range p.Errors {
				fields[f.Field] = f.Code
			}
			assert.Equal(t, tt.wantCode, fields[tt.wantField], rec.Body.String())
		})
	}
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM process.mapping_version"))

	noVersion := e.json(t, e.admin, http.MethodPost, teamPath(e.team, _mappingVersions), http.Header{"Idempotency-Key": {uuid.NewString()}}, moscowDraft())
	assert.Equal(t, http.StatusPreconditionRequired, noVersion.Code, "a confirmation without If-Match")
}

func TestProcess_RolesAndOrganizations(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	other := e.loginEnv.admin(t, "other")
	setup := emptyStructure()
	setup.Teams = []named{{Name: "Elsewhere", Status: "active"}}
	rec := e.replace(t, other, `"0"`, setup)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	elsewhere := decode[structureBody](t, rec).team("Elsewhere")

	for _, resource := range []string{"workflow-catalog", _mappingVersions, _calendarVersions} {
		got := e.do(t, e.admin, http.MethodGet, teamPath(e.team, resource), nil)
		assert.Equal(t, http.StatusForbidden, got.Code, "an administrator without process_owner: %s", resource)
	}
	denied := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), moscowDraft())
	assert.Equal(t, http.StatusForbidden, denied.Code)

	e.grantProcessOwner(t, e.adminID)
	assert.Equal(t, http.StatusOK, e.do(t, e.admin, http.MethodGet, teamPath(e.team, "workflow-catalog"), nil).Code)
	foreign := e.do(t, e.admin, http.MethodGet, teamPath(elsewhere, "workflow-catalog"), nil)
	assert.Equal(t, http.StatusNotFound, foreign.Code, "a team of another organization")
	foreignWrite := e.json(t, e.admin, http.MethodPost, teamPath(elsewhere, _mappingVersions),
		http.Header{"Idempotency-Key": {uuid.NewString()}, "If-Match": {`"0"`}}, moscowDraft())
	assert.Equal(t, http.StatusNotFound, foreignWrite.Code)
}

func TestProcess_VersionAtKnowledgeNumber(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	org, team := e.orgID, entity.ID(e.team)
	before := e.lastSeq(t)

	rec := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), moscowDraft(e.rule("status", "3", "", "active")))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	k1 := e.lastSeq(t)
	rec = e.confirm(t, e.admin, _mappingVersions, `"1"`, uuid.New(), moscowDraft(e.rule("status", "3", "", "wait")))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	k2 := e.lastSeq(t)
	require.Greater(t, k2, k1)

	_, ok, err := e.process.MappingAt(t.Context(), org, team, before)
	require.NoError(t, err)
	assert.False(t, ok, "no version was known before the first confirmation")
	v, ok, err := e.process.MappingAt(t.Context(), org, team, k1)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 1, v.VersionNo, "a later version does not change the answer for an earlier K")
	assert.Equal(t, entity.MeaningActive, v.Rules[0].Meaning)
	v, ok, err = e.process.MappingAt(t.Context(), org, team, k2)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 2, v.VersionNo)

	seq, err := e.process.LastSettingsSeq(t.Context(), org, team)
	require.NoError(t, err)
	assert.Equal(t, v.KnownSeq, seq)
	assert.Equal(t, k2, seq)
}

func TestProcess_CalendarVersions(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	start, end := "10:00", "19:00"
	cal := calendarDraftBody{
		Timezone: "Europe/Moscow", Workdays: []string{"fri", "mon", "tue", "wed", "thu"}, WorkStart: &start, WorkEnd: &end,
		Holidays: []string{"2026-12-31", "2026-11-04"},
	}

	created := e.confirm(t, e.admin, _calendarVersions, `"0"`, uuid.New(), cal)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	v1 := decode[calendarVersionBody](t, created)
	assert.Equal(t, 1, v1.VersionNo)
	assert.Nil(t, v1.BaseVersionNo)
	assert.Equal(t, []string{"mon", "tue", "wed", "thu", "fri"}, v1.Workdays)
	assert.Equal(t, []string{"2026-11-04", "2026-12-31"}, v1.Holidays)
	assert.Equal(t, "10:00", *v1.WorkStart)

	equal := e.confirm(t, e.admin, _calendarVersions, `"1"`, uuid.New(), cal)
	assert.Equal(t, http.StatusOK, equal.Code, "the same calendar is the current version")
	stale := e.confirm(t, e.admin, _calendarVersions, `"0"`, uuid.New(), calendarDraftBody{Timezone: "UTC", Workdays: []string{}, Holidays: []string{}})
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code)
	unknown := e.confirm(t, e.admin, _calendarVersions, `"1"`, uuid.New(), calendarDraftBody{Timezone: "Mars/Olympus", Workdays: []string{}, Holidays: []string{}})
	require.Equal(t, http.StatusUnprocessableEntity, unknown.Code)
	assert.Equal(t, []string{"/timezone"}, fieldsOf(t, unknown), "the name is not in pg_timezone_names")
	late := "09:00"
	malformed := e.confirm(t, e.admin, _calendarVersions, `"1"`, uuid.New(), calendarDraftBody{
		Timezone: "UTC", Workdays: []string{"mon", "mon"}, WorkStart: &start, WorkEnd: &late, Holidays: []string{"2026-11-04", "2026-11-04"},
	})
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	assert.Equal(t, []string{"/workdays/1"}, fieldsOf(t, malformed), "the form of the contract is checked first")
	reversed := e.confirm(t, e.admin, _calendarVersions, `"1"`, uuid.New(), calendarDraftBody{
		Timezone: "UTC", Workdays: []string{"mon"}, WorkStart: &start, WorkEnd: &late, Holidays: []string{"2026-11-04", "2026-11-04"},
	})
	require.Equal(t, http.StatusUnprocessableEntity, reversed.Code)
	assert.ElementsMatch(t, []string{"/work_end", "/holidays"}, fieldsOf(t, reversed))

	rec := e.do(t, e.admin, http.MethodGet, teamPath(e.team, _calendarVersions), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	assert.Len(t, decode[versionPageBody[calendarVersionBody]](t, rec).Data, 1)

	no := 1
	mapping := e.confirm(t, e.admin, _mappingVersions, `"0"`, uuid.New(), mappingDraftBody{Timezone: "Europe/Moscow", CalendarVersionNo: &no, Rules: []mappingRuleBody{}})
	require.Equal(t, http.StatusCreated, mapping.Code, mapping.Body.String())
	assert.Equal(t, &no, decode[mappingVersionBody](t, mapping).CalendarVersionNo)
	at, ok, err := e.process.CalendarAt(t.Context(), e.orgID, entity.ID(e.team), e.lastSeq(t))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Europe/Moscow", at.Timezone)
	assert.Len(t, e.audit(t, "process.calendar.confirm"), 1)
}

func TestProcess_ListPages(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	e.grantProcessOwner(t, e.adminID)
	meanings := []string{"intake", "active", "wait"}
	for i, m := range meanings {
		rec := e.confirm(t, e.admin, _mappingVersions, `"`+strconv.Itoa(i)+`"`, uuid.New(), moscowDraft(e.rule("status", "3", "", m)))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	}
	var got []int
	target := teamPath(e.team, _mappingVersions) + "?limit=2"
	for target != "" {
		rec := e.do(t, e.admin, http.MethodGet, target, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, `"3"`, rec.Header().Get("ETag"), "the ETag is the last version on every page")
		page := decode[versionPageBody[mappingVersionBody]](t, rec)
		for _, v := range page.Data {
			got = append(got, v.VersionNo)
		}
		target = ""
		if page.NextCursor != nil {
			target = teamPath(e.team, _mappingVersions) + "?limit=2&cursor=" + *page.NextCursor
		}
	}
	assert.Equal(t, []int{3, 2, 1}, got, "newest first")
	malformed := e.do(t, e.admin, http.MethodGet, teamPath(e.team, _mappingVersions)+"?cursor=bad", nil)
	assert.Equal(t, http.StatusBadRequest, malformed.Code)
}

func fieldsOf(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	p := decode[problemBody](t, rec)
	out := make([]string, len(p.Errors))
	for i, f := range p.Errors {
		out[i] = f.Field
	}
	return out
}
