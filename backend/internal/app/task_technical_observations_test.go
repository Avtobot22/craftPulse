package app

import (
	"context"
	"crypto/sha256"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestTaskViews_TechnicalObservationsByHeadAndAccess(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	task, mr, _, _, area := seedTaskCard(t, e)
	e.link(t, _otherSub, "demo-person-02")
	e.grantManager(t)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment
		(organization_id,user_id,role,project_id,assigned_by,assigned_at) VALUES ($1,$2,'security_access',$3,$2,$4)`,
		uuid.UUID(e.orgID), e.adminID, e.project, e.clock.Now())
	require.NoError(t, err)

	profile, component, finding := seedTaskSummaryProfile(t, e, area)
	missing := entity.SummaryItem{
		Kind: "not_evaluated", ComponentID: &component, AnalysisKind: entity.AnalysisSAST, ReasonCode: "scan_missing", RequiredRight: "security",
	}
	added := entity.SummaryItem{
		Kind: entity.FindingNew, ComponentID: &component, AnalysisKind: entity.AnalysisSCA, FindingID: &finding,
		OccurrenceID: new(entity.ID(uuid.New())), Rank: new(1), RequiredRight: "security",
	}
	changes := changerepo.NewRepo(e.app)
	tx := persistent.NewTransactor(e.app)
	save := func(head, base string, items ...entity.SummaryItem) {
		t.Helper()
		seq, err := e.runner.norm.ReportKnowledge(t.Context(), e.orgID, nil)
		require.NoError(t, err)
		hash := sha256.Sum256([]byte(head))
		v := entity.SummaryRevision{
			MRID: entity.ID(mr), HeadSHA: head, BaseSHA: base, ProfileVersionID: profile, PipelineState: "reports_missing",
			KnowledgeSeq: seq, BuiltAt: e.clock.Now(), InputHash: hash[:], Items: items,
		}
		require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
			saved, err := changes.SaveSummary(ctx, e.orgID, &v)
			assert.True(t, saved)
			return err
		}))
	}
	save(_headB, _headA, missing, added)

	path := _tasksPath + "/" + task.String()
	read := func(actor *browser) openapi.TaskDetail {
		t.Helper()
		return decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, actor, path, "TaskDetail"))
	}
	technical := func(v openapi.TaskDetail) []taskObservationBody {
		return slices.DeleteFunc(v.Observations, func(o taskObservationBody) bool {
			return o.Kind != openapi.TaskDetailObservationsKindMissingReport && o.Kind != openapi.TaskDetailObservationsKindNewFinding
		})
	}
	before := read(e.admin)
	require.Len(t, before.Links, 1)
	assert.Equal(t, openapi.LinkStatusCandidate, before.Links[0].Status)
	assert.Empty(t, technical(before))

	res := e.json(t, e.admin, http.MethodPost, "/api/v1/links/"+before.Links[0].Id.String()+"/decisions",
		http.Header{"Idempotency-Key": {uuid.NewString()}}, map[string]string{"decision": "confirm"})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	withRights := technical(read(e.admin))
	require.Len(t, withRights, 2)
	reports, newFinding := withRights[0], withRights[1]
	assert.Equal(t, openapi.TaskDetailObservationsKindMissingReport, reports.Kind)
	assert.Equal(t, openapi.RestoreReport, reports.Action)
	require.Len(t, reports.Basis, 1)
	assert.Equal(t, mr, reports.Basis[0].Id)
	assert.Equal(t, new("!7"), reports.Basis[0].DisplayKey)
	assert.Equal(t, new("SAST · компонент services/billing · отчёт не получен"), reports.Basis[0].Title)
	assert.Equal(t, openapi.TaskDetailObservationsKindNewFinding, newFinding.Kind)
	assert.Equal(t, openapi.ReviewFinding, newFinding.Action)
	require.Len(t, newFinding.Basis, 2)
	assert.Equal(t, mr, newFinding.Basis[0].Id)
	assert.Equal(t, openapi.ObjectRefKindFinding, newFinding.Basis[1].Kind)
	assert.Equal(t, uuid.UUID(finding), newFinding.Basis[1].Id)
	assert.Equal(t, new("private-rule"), newFinding.Basis[1].DisplayKey)
	assert.Equal(t, new("SCA · новых находок в MR: 1"), newFinding.Basis[1].Title)

	assertRestricted := func() {
		t.Helper()
		res := codeRead(t, e.loginEnv, e.member, path, "TaskDetail")
		limited := decode[openapi.TaskDetail](t, res)
		assert.Contains(t, limited.Redactions, "observations")
		assert.True(t, slices.ContainsFunc(limited.Links, func(l openapi.Link) bool {
			return l.To.Id == mr && l.Status == openapi.LinkStatusConfirmed
		}))
		assert.Empty(t, technical(limited))
		assert.NotContains(t, res.Body.String(), finding.String())
		assert.NotContains(t, res.Body.String(), "private-rule")
	}
	assertRestricted()

	oldAt := e.clock.Now()
	oldSeq, err := e.runner.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	e.clock.Advance(time.Second)
	submit := e.mrImporter(t, entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)})
	updated := e.clock.Now()
	pushed := "cccccccccccccccccccccccccccccccccccccccc"
	submit(mrRecord(t, "merge_request", "9001", "", "v2", envelope.MergeRequestV1{
		ProjectNativeId: "42", Iid: new(7), Title: new("Private ready MR"), State: "open",
		SourceBranch: "feature/PA-4-retry", TargetBranch: "main", HeadSha: pushed,
		OpenedAt: _linkMerged.Add(-time.Hour), ReadyAt: &_linkMerged, UpdatedAt: &updated,
	}))
	assert.Empty(t, technical(read(e.admin)), "the previous head is still stored but supplies no observations")
	save(pushed, "", missing)
	current := technical(read(e.admin))
	require.Len(t, current, 1)
	assert.Equal(t, openapi.TaskDetailObservationsKindMissingReport, current[0].Kind)
	assert.Equal(t, openapi.RestoreReport, current[0].Action)
	require.Len(t, current[0].Basis, 1)
	assert.Equal(t, mr, current[0].Basis[0].Id)
	assertRestricted()

	latestSeq, err := e.runner.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	require.Greater(t, latestSeq, oldSeq)
	require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		oldHead := []entity.MRVersion{{MRID: entity.ID(mr), HeadSHA: _headB}}
		byDate, err := changes.ReadTaskSummaryRows(ctx, e.orgID, oldHead, latestSeq, oldAt)
		require.NoError(t, err)
		require.Len(t, byDate, 2)
		byKnowledge, err := changes.ReadTaskSummaryRows(ctx, e.orgID, oldHead, oldSeq, e.clock.Now())
		require.NoError(t, err)
		assert.Equal(t, byDate, byKnowledge)
		rows, err := changes.ReadTaskSummaryRows(ctx, e.orgID, oldHead, latestSeq, e.clock.Now())
		assert.Empty(t, rows, "the newest revision for another head must not fall back to an older match")
		return err
	}))
}

func seedTaskSummaryProfile(t *testing.T, e *reportEnv, area uuid.UUID) (profile, component, finding entity.ID) {
	t.Helper()
	org, service := uuid.UUID(e.orgID), uuid.New()
	at := e.clock.Now()
	profileID, componentID, findingID := uuid.New(), uuid.New(), uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.service (organization_id, id, name, status, created_at)
		VALUES ($1, $2, 'Billing', 'active', $3)`, org, service, at)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.component (organization_id, id, known_seq, service_id, repository_area_id,
		path_prefix, status, origin, confirmed_by, confirmed_at)
		VALUES ($1, $2, 1, $3, $4, 'services/billing', 'confirmed', 'manual', $5, $6)`, org, componentID, service, area, e.adminID, at)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO change.profile_version (organization_id, id, project_id, version_no,
		content_hash, known_seq, confirmed_by, confirmed_at, confirmation_id)
		VALUES ($1, $2, $3, 1, decode(repeat('00',32),'hex'), 1, $4, $5, uuidv7())`, org, profileID, e.project, e.adminID, at)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO change.finding_identity (organization_id, id, area_id, method_version,
		fingerprint, rule_id, finding_kind, first_report_id, first_seen_at)
		VALUES ($1, $2, $3, 1, decode(repeat('00',32),'hex'), 'private-rule', 'dependency', uuidv7(), $4)`, org, findingID, area, at)
	require.NoError(t, err)
	return entity.ID(profileID), entity.ID(componentID), entity.ID(findingID)
}
