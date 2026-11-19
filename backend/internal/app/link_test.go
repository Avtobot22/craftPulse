package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
)

// linkEnv is the collected Jira area of the project Core with a task and merge requests in it. The member is the
// project manager of Core with a linked account; the administrator has no project manager role.
type linkEnv struct {
	*accessEnv
	links *linkuc.UseCase
	task  uuid.UUID
}

func newLinkEnv(t *testing.T) *linkEnv {
	t.Helper()
	e := &linkEnv{accessEnv: newAccessEnv(t)}
	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.roles(t, e.admin, e.memberID, rec.Header().Get("ETag"), []roleInput{{Role: "project_manager", ProjectID: &e.project}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	e.link(t, _otherSub, "demo-person-02")
	tx := persistent.NewTransactor(e.app)
	e.links = linkuc.New(linkuc.Deps{
		Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Auditor: audit.New(auditrepo.NewRepo(e.app), tx, e.clock),
		Clock: e.clock, NewID: newID,
	})
	e.task = e.object(t, "task", "TASK-1", false)
	return e
}

func (e *linkEnv) object(t *testing.T, kind, native string, restricted bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO normalize.source_object
 (organization_id,id,instance_id,object_kind,native_ns,native_id,identity_quality,first_known_seq,current_area_id,restricted)
 VALUES ($1,$2,$3,$4,'',$5,'stable',1,$6,$7)`, uuid.UUID(e.orgID), id, e.instance, kind, native, e.area, restricted)
	require.NoError(t, err)
	return id
}

func (e *linkEnv) propose(t *testing.T, to uuid.UUID, m entity.LinkMethod, evidence string) entity.LinkEdge {
	t.Helper()
	var out entity.LinkEdge
	require.NoError(t, persistent.NewTransactor(e.app).InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		var err error
		out, err = e.links.Record(ctx, e.orgID, entity.LinkProposal{
			Type: entity.LinkTaskMR, From: entity.ID(e.task), To: entity.ID(to), Method: m, Evidence: json.RawMessage(evidence),
		})
		return err
	}))
	return out
}

func (e *linkEnv) decide(t *testing.T, b *browser, id entity.ID, decision string, key uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"decision": decision})
	require.NoError(t, err)
	h := _changing.Clone()
	h.Set("Idempotency-Key", key.String())
	return e.send(t, b, http.MethodPost, "/api/v1/links/"+id.String()+"/decisions", h, bytes.NewReader(raw))
}

func (e *linkEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.owner.QueryRow(t.Context(), query, append([]any{uuid.UUID(e.orgID)}, args...)...).Scan(&n))
	return n
}

func (e *linkEnv) seq(t *testing.T) int64 {
	t.Helper()
	var seq int64
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT last_seq FROM platform.org_clock WHERE organization_id = $1", uuid.UUID(e.orgID)).Scan(&seq))
	return seq
}

func TestLink_ConfirmIsIdempotentAndKeepsTheSlice(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	mr := e.object(t, "merge_request", "!42", false)
	candidate := e.propose(t, mr, entity.LinkBranchKey, `{"branch":"feature/TASK-1"}`)
	require.Equal(t, entity.LinkCandidate, candidate.Status)
	before := e.seq(t)

	key := uuid.New()
	rec := e.decide(t, e.member, candidate.ID, "confirm", key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	link := decode[openapi.Link](t, rec)
	assert.Equal(t, openapi.LinkMethod("user_confirmed"), link.Method)
	assert.Equal(t, openapi.LinkStatusConfirmed, link.Status)
	assert.Equal(t, openapi.LinkLinkType("task_mr"), link.LinkType)
	assert.Equal(t, e.task, link.FromObjectId)
	assert.Equal(t, openapi.ObjectRefKindMergeRequest, link.To.Kind)
	assert.Equal(t, mr, link.To.Id)
	require.NotNil(t, link.DecidedAt)

	again := e.decide(t, e.member, candidate.ID, "confirm", key)
	require.Equal(t, http.StatusOK, again.Code, again.Body.String())
	assert.Equal(t, link.Id, decode[openapi.Link](t, again).Id, "the same key returns the saved link")
	other := e.decide(t, e.member, candidate.ID, "confirm", uuid.New())
	require.Equal(t, http.StatusOK, other.Code, other.Body.String())
	assert.Equal(t, link.Id, decode[openapi.Link](t, other).Id, "a confirmed pair is not confirmed twice")
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM link.edge WHERE organization_id = $1 AND method = 'user_confirmed'"))
	reused := e.decide(t, e.member, candidate.ID, "reject", key)
	assert.Equal(t, http.StatusUnprocessableEntity, reused.Code, reused.Body.String())

	old, err := e.links.EdgesAt(t.Context(), e.orgID, entity.ID(e.task), entity.LinksFrom, before)
	require.NoError(t, err)
	require.Len(t, old, 1)
	assert.Equal(t, entity.LinkCandidate, old[0].Status, "the slice before the decision keeps the candidate")
	now, err := e.links.EdgesAt(t.Context(), e.orgID, entity.ID(mr), entity.LinksTo, e.seq(t))
	require.NoError(t, err)
	require.Len(t, now, 2)
	assert.Equal(t, entity.LinkMethod("branch_key"), now[0].Method)
	assert.Equal(t, entity.LinkConfirmed, now[1].Status)
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit.event WHERE organization_id = $1 AND operation = 'link.decide'"))
}

func TestLink_RejectedCandidateStaysRejected(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	mr := e.object(t, "merge_request", "!43", false)
	candidate := e.propose(t, mr, entity.LinkTitleKey, `{"title":"TASK-1 fix"}`)

	rec := e.decide(t, e.member, candidate.ID, "reject", uuid.New())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rejected := decode[openapi.Link](t, rec)
	assert.Equal(t, openapi.LinkStatusRejected, rejected.Status)
	assert.Equal(t, openapi.LinkMethod("title_key"), rejected.Method)

	again := e.propose(t, mr, entity.LinkTitleKey, `{"title": "TASK-1 fix"}`)
	assert.Equal(t, entity.LinkRejected, again.Status, "the same basis does not bring the candidate back")
	assert.Equal(t, entity.ID(rejected.Id), again.ID)
	renewed := e.propose(t, mr, entity.LinkTitleKey, `{"title":"TASK-1 second fix"}`)
	assert.Equal(t, entity.LinkCandidate, renewed.Status, "a new basis proposes the candidate again")
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit.event WHERE organization_id = $1 AND operation = 'link.decide' AND details->>'decision' = 'reject'"))
}

func TestLink_SourceLinkIsNotRejected(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	mr := e.object(t, "merge_request", "!44", false)
	explicit := e.propose(t, mr, entity.LinkSourceExplicit, `{"field":"development"}`)
	require.Equal(t, entity.LinkConfirmed, explicit.Status)
	candidate := e.propose(t, mr, entity.LinkBranchKey, `{"branch":"TASK-1"}`)

	rec := e.decide(t, e.member, explicit.ID, "reject", uuid.New())
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "state_conflict")
	rec = e.decide(t, e.member, candidate.ID, "confirm", uuid.New())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, entity.ID(decode[openapi.Link](t, rec).Id), explicit.ID, "the explicit link already confirms the pair")
	assert.Zero(t, e.count(t, "SELECT count(*) FROM link.edge WHERE organization_id = $1 AND method = 'user_confirmed'"))
}

func TestLink_DecisionNeedsBothEndsAndTheRole(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	hidden := e.object(t, "merge_request", "!45", true)
	candidate := e.propose(t, hidden, entity.LinkBranchKey, `{"branch":"TASK-1"}`)
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "object", NativeID: "!45", Decision: "denied"},
		{UserID: e.adminID.String(), Decision: "allowed"},
		{UserID: e.memberID.String(), Decision: "allowed"},
	}})

	rec := e.decide(t, e.member, candidate.ID, "confirm", uuid.New())
	assert.Equal(t, http.StatusNotFound, rec.Code, "an MR the user may not read hides the link: %s", rec.Body.String())
	rec = e.decide(t, e.admin, candidate.ID, "confirm", uuid.New())
	assert.Equal(t, http.StatusForbidden, rec.Code, "only a project manager decides: %s", rec.Body.String())
	rec = e.decide(t, e.member, entity.ID(uuid.New()), "confirm", uuid.New())
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "area", Decision: "denied"}, {UserID: e.adminID.String(), Decision: "allowed"},
	}})
	e.clock.Advance(61 * time.Second)
	open := e.object(t, "merge_request", "!46", false)
	other := e.propose(t, open, entity.LinkBranchKey, `{"branch":"TASK-1"}`)
	rec = e.decide(t, e.member, other.ID, "confirm", uuid.New())
	assert.Equal(t, http.StatusNotFound, rec.Code, "without the area in the source neither end is visible: %s", rec.Body.String())
	assert.Zero(t, e.count(t, "SELECT count(*) FROM link.edge WHERE organization_id = $1 AND status <> 'candidate'"))
	assert.Zero(t, e.count(t, "SELECT count(*) FROM audit.event WHERE organization_id = $1 AND operation = 'link.decide'"))
}
