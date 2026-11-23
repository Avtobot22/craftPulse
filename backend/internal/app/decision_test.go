package app

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _exceptionsPath = "/api/v1/exceptions"

func TestExceptions_ApprovalRenewalExpiryAndPermissions(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	project := uuid.UUID(e.project)
	author := e.exceptionActor(t, "exception-author", []roleInput{
		{Role: "developer", ProjectID: &project},
		{Role: "tech_lead", ProjectID: &project},
		{Role: "security_access", ProjectID: &project},
	})
	reviewer := e.lead
	e.setExceptionRoles(t, _otherSub, []roleInput{{Role: "tech_lead", ProjectID: &project}, {Role: "security_access", ProjectID: &project}})
	repoID := e.areas["42"]
	first := e.submit(t, reportPackageOf(t, _checksInstance, _headA, reportSpec{
		name: "sample.sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1",
		ci: gitlabCI("57001", "7001"), component: &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"},
		content: sarifOf(hit{rule: "configured-rule", path: "services/billing/src/main.go", line: 12}), exit: "findings", completeness: "complete",
	}))
	findings := e.identities(t, first)
	require.Len(t, findings, 1)
	finding := findings[0]
	require.Equal(t, "security", e.findingKind(t, finding.identity))

	// Exercise imported findings without a live GitLab connection.
	_, err := e.owner.Exec(t.Context(), "DELETE FROM connect.connection_scope WHERE organization_id=$1 AND area_id=$2", uuid.UUID(e.orgID), repoID)
	require.NoError(t, err)

	now := e.clock.Now().UTC()
	expires := now.Add(10 * time.Minute)
	draft := openapi.ExceptionDraft{
		DecisionType: openapi.ExceptionDraftDecisionType(entity.ExceptionAcceptedRisk), Reason: "Reviewed in the test project",
		EffectiveFrom: now, ExpiresAt: expires,
		Scope: openapi.ExceptionScope{AreaId: repoID, ComponentId: &e.goComponent, FindingId: &finding.identity},
	}
	call := func(b *browser, method, path, match string, key uuid.UUID, body any) *httptest.ResponseRecorder {
		t.Helper()
		h := _changing.Clone()
		if match != "" {
			h.Set("If-Match", match)
		}
		if key != uuid.Nil {
			h.Set("Idempotency-Key", key.String())
		}
		var raw []byte
		if body != nil {
			raw = mustMarshal(t, body)
		}
		return e.call(t, b, "", method, path, h, raw)
	}
	post := func(b *browser, path, match string, key uuid.UUID, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		resp := call(b, http.MethodPost, path, match, key, body)
		require.Equal(t, want, resp.Code, resp.Body.String())
		return resp
	}
	get := func(b *browser, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		resp := call(b, http.MethodGet, path, "", uuid.Nil, nil)
		require.Equal(t, want, resp.Code, resp.Body.String())
		return resp
	}

	post(e.adminBrowser, _exceptionsPath, "", uuid.New(), draft, http.StatusForbidden)
	bad := draft
	bad.Reason, bad.ExpiresAt = " ", draft.EffectiveFrom
	post(author, _exceptionsPath, "", uuid.New(), bad, http.StatusUnprocessableEntity)
	bad = draft
	bad.Scope.AreaId = e.areas["43"]
	outside := post(author, _exceptionsPath, "", uuid.New(), bad, http.StatusUnprocessableEntity)
	assert.Contains(t, outside.Body.String(), entity.FieldOutsideScope)

	key := uuid.New()
	draft.Scope.ComponentId = nil
	created := post(author, _exceptionsPath, "", key, draft, http.StatusCreated)
	d := decode[openapi.ExceptionDetail](t, created)
	require.Equal(t, entity.ExceptionPending, string(d.State))
	assert.True(t, d.CurrentVersion.ApprovalRequired)
	path, etag := _exceptionsPath+"/"+d.Id.String(), created.Header().Get("ETag")
	require.Equal(t, path, created.Header().Get("Location"))
	require.NotEmpty(t, etag)
	retry := post(author, _exceptionsPath, "", key, draft, http.StatusCreated)
	assert.Equal(t, d.Id, decode[openapi.ExceptionDetail](t, retry).Id)
	bad = draft
	bad.Reason = "Different request"
	reused := post(author, _exceptionsPath, "", key, bad, http.StatusUnprocessableEntity)
	assert.Contains(t, reused.Body.String(), "idempotency_key_reused")

	before := e.activeExceptions(t, repoID, now)
	assert.Empty(t, before)
	get(e.adminBrowser, path, http.StatusNotFound)
	listHidden := decode[openapi.ExceptionPage](t, get(e.adminBrowser, _exceptionsPath, http.StatusOK))
	assert.Empty(t, listHidden.Data)
	assert.True(t, listHidden.AccessLimited)

	decide := path + "/decisions"
	approve := map[string]string{"decision": "approve"}
	post(author, decide, etag, uuid.New(), approve, http.StatusForbidden)
	post(reviewer, decide, "", uuid.New(), approve, http.StatusPreconditionRequired)
	approvalKey := uuid.New()
	active := decode[openapi.ExceptionDetail](t, post(reviewer, decide, etag, approvalKey, approve, http.StatusOK))
	assert.Equal(t, entity.ExceptionActive, string(active.State))
	assert.Equal(t, entity.ExceptionApproved, string(active.CurrentVersion.Status))
	assert.Equal(t, active, decode[openapi.ExceptionDetail](t, post(reviewer, decide, etag, approvalKey, approve, http.StatusOK)))
	activeVersions := e.activeExceptions(t, repoID, e.clock.Now())
	require.Len(t, activeVersions, 1)
	assert.Equal(t, entity.ID(finding.identity), activeVersions[0].Scope.FindingID)
	coord := entity.FindingCoordinates{
		FindingAccess: entity.FindingAccess{AreaID: entity.ID(repoID), Kind: "security"},
		FindingID:     entity.ID(finding.identity), ComponentID: entity.ID(e.goComponent),
	}
	assert.True(t, activeVersions[0].Matches(coord))
	coord.AreaID = entity.ID(e.areas["43"])
	assert.False(t, activeVersions[0].Matches(coord))
	e.projectExceptionJobs(t, repoID, activeVersions[0].ID)

	draft.Reason, draft.ExpiresAt = "Extend to the next review", now.Add(20*time.Minute)
	draft.Scope.ComponentId = &e.goComponent
	versionPath := path + "/versions"
	extended := post(author, versionPath, etag, uuid.New(), draft, http.StatusCreated)
	v2 := decode[openapi.ExceptionDetail](t, extended)
	assert.Equal(t, entity.ExceptionActive, string(v2.State))
	assert.Equal(t, entity.ExceptionPending, string(v2.CurrentVersion.Status))
	assert.Equal(t, 2, v2.CurrentVersion.VersionNo)
	require.Len(t, v2.Versions, 1)
	assert.Equal(t, entity.ExceptionApproved, string(v2.Versions[0].Status))
	etag2 := extended.Header().Get("ETag")
	post(author, versionPath, etag, uuid.New(), draft, http.StatusPreconditionFailed)
	post(reviewer, decide, etag, uuid.New(), approve, http.StatusPreconditionFailed)

	// Another project now shares the repository, beyond the reviewers' project grants.
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
		VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, uuid.UUID(e.orgID), e.other, repoID, now)
	require.NoError(t, err)
	post(author, versionPath, etag2, uuid.New(), draft, http.StatusForbidden)
	post(reviewer, decide, etag2, uuid.New(), approve, http.StatusForbidden)
	bad = draft
	bad.Scope.ComponentId = nil
	outside = post(author, _exceptionsPath, "", uuid.New(), bad, http.StatusUnprocessableEntity)
	assert.Contains(t, outside.Body.String(), entity.FieldOutsideScope)

	assert.Equal(t, activeVersions[0].ID, e.activeExceptions(t, repoID, expires.Add(-time.Second))[0].ID)
	assert.Empty(t, e.activeExceptions(t, repoID, expires))

	e.expireException(t, d.Id, expires, repoID)
	waiting := decode[openapi.ExceptionDetail](t, get(author, path, http.StatusOK))
	assert.Equal(t, entity.ExceptionPending, string(waiting.State))
	rejected := decode[openapi.ExceptionDetail](t, post(reviewer, decide, etag2, uuid.New(), map[string]string{"decision": "reject"}, http.StatusOK))
	assert.Equal(t, entity.ExceptionRejected, string(rejected.State))
	post(reviewer, decide, etag2, uuid.New(), approve, http.StatusConflict)

	v3 := post(author, versionPath, etag2, uuid.New(), draft, http.StatusCreated)
	withdraw := map[string]string{"decision": "withdraw"}
	post(reviewer, decide, v3.Header().Get("ETag"), uuid.New(), withdraw, http.StatusForbidden)
	w := decode[openapi.ExceptionDetail](t, post(author, decide, v3.Header().Get("ETag"), uuid.New(), withdraw, http.StatusOK))
	assert.Equal(t, entity.ExceptionWithdrawn, string(w.State))
	require.Len(t, w.Transitions, 6)

	ruleDraft := draft
	ruleDraft.DecisionType = openapi.ExceptionDraftDecisionType(entity.ExceptionFalsePositive)
	ruleDraft.Scope.FindingId, ruleDraft.Scope.RuleId = nil, &finding.rule
	byRule := decode[openapi.ExceptionDetail](t, post(author, _exceptionsPath, "", uuid.New(), ruleDraft, http.StatusCreated))
	assert.Equal(t, entity.ExceptionActive, string(byRule.State))
	assert.False(t, byRule.CurrentVersion.ApprovalRequired)
	page1 := decode[openapi.ExceptionPage](t, get(author, _exceptionsPath+"?sort=-created_at&limit=1", http.StatusOK))
	require.Len(t, page1.Data, 1)
	assert.Equal(t, byRule.Id, page1.Data[0].Id)
	require.NotNil(t, page1.NextCursor)
	page2 := decode[openapi.ExceptionPage](t, get(author, _exceptionsPath+"?sort=-created_at&limit=1&cursor="+url.QueryEscape(*page1.NextCursor), http.StatusOK))
	require.Len(t, page2.Data, 1)
	assert.Equal(t, d.Id, page2.Data[0].Id)
	assert.Nil(t, page2.NextCursor)
	filtered := decode[openapi.ExceptionPage](t, get(author, _exceptionsPath+"?state=withdrawn&finding_id="+finding.identity.String(), http.StatusOK))
	require.Len(t, filtered.Data, 1)
	assert.Equal(t, d.Id, filtered.Data[0].Id)
	assert.Equal(t, 6, e.normalized.n(t, "SELECT count(*) FROM audit.event WHERE object_id=$1 AND outcome='succeeded' AND operation IN ('createException','createExceptionVersion','decideException')", d.Id))
}

func (e *identityEnv) exceptionActor(t *testing.T, sub string, roles []roleInput) *browser {
	t.Helper()
	b := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, b, sub).Code)
	e.setExceptionRoles(t, sub, roles)
	return b
}

func (e *identityEnv) setExceptionRoles(t *testing.T, sub string, roles []roleInput) {
	t.Helper()
	id := e.userID(t, e.orgID, sub)
	rec := e.do(t, e.adminBrowser, http.MethodGet, "/api/v1/users/"+id.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.roles(t, e.adminBrowser, id, rec.Header().Get("ETag"), roles)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func (e *identityEnv) findingKind(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var kind string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT finding_kind FROM change.finding_identity WHERE id=$1", id).Scan(&kind))
	return kind
}

func (e *identityEnv) activeExceptions(t *testing.T, area uuid.UUID, at time.Time) []entity.ExceptionVersion {
	t.Helper()
	out, err := e.decisions.ActiveAt(t.Context(), e.orgID, entity.ID(area), math.MaxInt64, at)
	require.NoError(t, err)
	return out
}

func (e *identityEnv) projectExceptionJobs(t *testing.T, area uuid.UUID, want ...entity.ID) {
	t.Helper()
	worker := newIdentifier(e.app, e.norm, e.orgs, e.clock, e.decisions)
	var id int64
	var raw []byte
	require.NoError(t, e.owner.QueryRow(t.Context(),
		"SELECT id, args FROM river_job WHERE kind=$1 AND state='available' ORDER BY id DESC LIMIT 1", entity.KindChangeRecount).Scan(&id, &raw))
	var job entity.ChangeRecountJob
	require.NoError(t, json.Unmarshal(raw, &job))
	require.NoError(t, worker.RunExceptionRecount(t.Context(), job, completeJob(id)))
	var got []uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT active_version_ids FROM change.exception_projection WHERE area_id=$1", area).Scan(&got))
	ids := make([]entity.ID, len(got))
	for i := range got {
		ids[i] = entity.ID(got[i])
	}
	assert.ElementsMatch(t, want, ids)
}

func (e *identityEnv) expireException(t *testing.T, exception uuid.UUID, expiry time.Time, area uuid.UUID) {
	t.Helper()
	_, err := e.owner.Exec(t.Context(), "UPDATE river_job SET state='completed', finalized_at=now() WHERE kind IN ($1,$2) AND state='available'",
		entity.KindCalcRecount, entity.KindChangeRecount)
	require.NoError(t, err)
	var id int64
	var raw []byte
	var scheduled time.Time
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id, args, scheduled_at FROM river_job
 WHERE kind=$1 AND args->>'decision_id'=$2 AND state='scheduled'`, entity.KindDecisionExpire, exception.String()).Scan(&id, &raw, &scheduled))
	assert.True(t, expiry.Equal(scheduled))
	assert.Equal(t, 1, e.normalized.n(t, "SELECT count(*) FROM river_job WHERE kind=$1 AND state='scheduled'", entity.KindDecisionExpire))
	e.clock.Advance(expiry.Sub(e.clock.Now()))
	var job entity.DecisionExpireJob
	require.NoError(t, json.Unmarshal(raw, &job))
	require.NoError(t, e.decisions.RunExpire(t.Context(), job, completeJob(id)))
	for _, kind := range []string{entity.KindCalcRecount, entity.KindChangeRecount} {
		assert.Equal(t, 1, e.normalized.n(t, "SELECT count(*) FROM river_job WHERE kind=$1 AND state='available' AND args->>'area_id'=$2", kind, area.String()))
	}
	require.NoError(t, e.decisions.RunExpire(t.Context(), job, func(context.Context) error { return nil }))
	assert.Equal(t, 1, e.normalized.n(t, "SELECT count(*) FROM river_job WHERE kind=$1 AND state='available'", entity.KindChangeRecount))
	e.projectExceptionJobs(t, area)
}
