package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	ingestrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/ingest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// _linkMerged is the merge of the MR of PA-4: in the week of 2 March 2026, before as_of of the series.
var _linkMerged = time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

// importMRs imports the repository 42 with the MR 9001 of the branch feature/PA-4-retry merged on 2 March; the
// repository is an area of the project of the team before the MR comes, so the key in the branch gives a candidate.
func (e *seriesEnv) importMRs(t *testing.T, admin entity.Subject) {
	t.Helper()
	caller, err := adapterapi.NewCaller(adapterapi.Config{Resolver: mapResolver{}})
	require.NoError(t, err)
	conn, err := newConnect(e.app, e.keyring, caller, e.clock)
	require.NoError(t, err)
	orgs, _, err := newOrgAccess(e.app, e.clock, conn, nil)
	require.NoError(t, err)
	intake, err := newIngest(e.app, sharedStore(t), conn, e.connectionsEnv.rights, orgs, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	submit := func(recs ...*envelope.Record) entity.Receipt {
		rc, err := intake.SubmitImport(t.Context(), admin, entity.ID(e.project), bytes.NewReader(pack(t, mrManifest("42"), recs...)))
		require.NoError(t, err)
		if rc.Status == entity.ReceiptQuarantined {
			require.NoError(t, intake.ProcessPackage(t.Context(), e.orgID, rc.ID))
		}
		require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
			rc, err = ingestrepo.NewRepo(e.app).Receipt(ctx, e.orgID, rc.ID)
			return err
		}))
		require.Equal(t, entity.ReceiptAccepted, rc.Status)
		e.settle(t)
		return rc
	}
	rc := submit(mrRepository(t, "42", "team/payments", "r1"))
	area, err := e.runner.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, "42")
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
 VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, uuid.UUID(e.orgID), e.project, uuid.UUID(area), e.clock.Now())
	require.NoError(t, err)
	iid, title, author, sha := 7, "Payment retry", "1", strings.Repeat("a", 40)
	opened := _linkMerged.AddDate(0, 0, -3)
	submit(mrRecord(t, "merge_request", "9001", "", "v1", envelope.MergeRequestV1{
		ProjectNativeId: "42", Iid: &iid, Title: &title, State: envelope.MergeRequestV1StateMerged, AuthorAccountNativeId: &author,
		SourceBranch: "feature/PA-4-retry", TargetBranch: "main", HeadSha: sha, OpenedAt: opened, MergedAt: &_linkMerged, UpdatedAt: &_linkMerged,
	}))
}

func (e *seriesEnv) linkPoint(t *testing.T, revision entity.ID, metric string, at time.Time) entity.SeriesPoint {
	t.Helper()
	pts := e.points(t, revision)
	for i := range pts {
		p := pts[i]
		if p.MetricID != metric || at.Before(p.Start) || !at.Before(p.End) {
			continue
		}
		page, err := e.calc.PointContributions(t.Context(), e.orgID, revision, entity.ContributionQuery{MetricID: metric, PointStart: p.Start, Limit: 500})
		require.NoError(t, err)
		require.Nil(t, page.Next)
		p.Contributions = page.Items
		return p
	}
	t.Fatalf("no point %s at %s", metric, at)
	return entity.SeriesPoint{}
}

func contributionOf(p entity.SeriesPoint, object entity.ID, role entity.ContributionRole) *entity.Contribution {
	for i := range p.Contributions {
		if p.Contributions[i].ObjectID == object && p.Contributions[i].Role == role {
			return &p.Contributions[i]
		}
	}
	return nil
}

func (e *seriesEnv) objectID(t *testing.T, kind, native string) entity.ID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind = $1 AND native_id = $2", kind, native).Scan(&id))
	return entity.ID(id)
}

// TestCalcLinks_TeamSeriesFromCandidateToConfirmedLink builds the weekly coverage of links of the team from the
// tasks of the test adapter and an imported MR: without code_required_type rules A20 and T28 are NOT_EVALUATED,
// with them the candidate of the branch counts nowhere, and its confirmation through decideLink makes a new
// revision with the MR in A18 and PA-4 in A20 and T28, while the former revision keeps its values.
func TestCalcLinks_TeamSeriesFromCandidateToConfirmedLink(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	e.importMRs(t, admin)
	pa4, pa3, mr := e.objectID(t, "task", "20004"), e.objectID(t, "task", "20003"), e.objectID(t, "merge_request", "9001")
	var candidate uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM link.edge WHERE from_object_id = $1 AND to_object_id = $2 AND status = 'candidate'",
		uuid.UUID(pa4), uuid.UUID(mr)).Scan(&candidate))

	_, err := e.calc.ReviewSeries(t.Context(), e.orgID, entity.ReviewScope{RepositoryID: e.objectArea(t, mr)}, entity.MetricSetLinks, entity.StepWeek)
	require.Error(t, err, "the coverage of links reads the mapping of a team")
	_, err = e.calc.ReviewSeries(t.Context(), e.orgID, entity.ReviewScope{TeamID: entity.ID(e.team)}, entity.MetricSetLinks, entity.StepDay)
	require.Error(t, err, "the coverage of links takes weeks or months")
	series, err := e.calc.ReviewSeries(t.Context(), e.orgID, entity.ReviewScope{TeamID: entity.ID(e.team)}, entity.MetricSetLinks, entity.StepWeek)
	require.NoError(t, err)
	build := func() entity.CalcRevision {
		require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, series.ID, _seriesWindow, _seriesAsOf))
		e.recount(t)
		rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
		require.NoError(t, err)
		require.Equal(t, entity.RevisionPublished, rev.Status)
		return rev
	}
	last := _seriesAsOf.Add(-time.Hour)

	unmapped := build()
	for _, metric := range []string{"A20", "T28"} {
		p := e.linkPoint(t, unmapped.ID, metric, last)
		assert.Equal(t, entity.StateNotEvaluated, p.State, metric)
		assert.Equal(t, entity.ReasonCodePolicyMissing, p.ReasonCode, metric)
	}
	a18 := e.linkPoint(t, unmapped.ID, "A18", _linkMerged)
	require.Equal(t, entity.StateAssessed, a18.State)
	assert.InDelta(t, 0, *a18.Value, 0, "a candidate by key is not a link")
	assert.InDelta(t, 1, *a18.Denominator, 0)
	assert.Equal(t, "candidate", contributionOf(a18, mr, entity.RoleDenominator).ReasonCode)

	rule := func(kind entity.RuleKind, ref, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(e.instance), NativeRef: ref, Meaning: meaning}
	}
	_, _, err = e.process.CreateMappingVersion(t.Context(), admin, entity.ID(e.team), 1, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", entity.MeaningIntake), rule(entity.RuleStatus, "3", entity.MeaningActive),
			rule(entity.RuleStatus, "5", entity.MeaningDone), rule(entity.RuleResolution, "10000", entity.MeaningAccepted),
			rule(entity.RuleCodeRequiredType, "10001", entity.MeaningCodeRequired),
		},
	})
	require.NoError(t, err)
	before := build()
	require.Greater(t, before.RevisionNo, unmapped.RevisionNo)
	t28 := e.linkPoint(t, before.ID, "T28", last)
	require.NotNil(t, t28.Value, "state %s, reason %s", t28.State, t28.ReasonCode)
	assert.InDelta(t, 0, *t28.Value, 0, "the candidate of PA-4 is not traceability")
	require.NotNil(t, contributionOf(t28, pa4, entity.RoleDenominator), "PA-4 is a story in work")
	assert.Equal(t, entity.ReasonNotCodeRequired, contributionOf(t28, pa3, entity.RoleExcluded).ReasonCode, "PA-3 is of type 10002")

	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.roles(t, e.admin, e.memberID, rec.Header().Get("ETag"), []roleInput{{Role: "project_manager", ProjectID: &e.project}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	e.link(t, _otherSub, "demo-person-02")
	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	raw, err := json.Marshal(map[string]string{"decision": "confirm"})
	require.NoError(t, err)
	h := _changing.Clone()
	h.Set("Idempotency-Key", uuid.NewString())
	rec = e.send(t, e.member, http.MethodPost, "/api/v1/links/"+candidate.String()+"/decisions", h, bytes.NewReader(raw))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var recounts int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind = $1 AND state IN ('available', 'scheduled')",
		entity.KindCalcRecount).Scan(&recounts))
	assert.Positive(t, recounts, "the decision recounts the areas of its ends")

	e.recount(t)
	after, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.Greater(t, after.RevisionNo, before.RevisionNo, "the confirmation makes a new revision")
	a18 = e.linkPoint(t, after.ID, "A18", _linkMerged)
	assert.InDelta(t, 1, *a18.Value, 0)
	assert.Equal(t, string(entity.LinkUserConfirmed), contributionOf(a18, mr, entity.RoleNumerator).ReasonCode)
	for _, metric := range []string{"A20", "T28"} {
		p := e.linkPoint(t, after.ID, metric, last)
		assert.NotNil(t, contributionOf(p, pa4, entity.RoleNumerator), metric+": PA-4 has a confirmed merged MR")
	}
	old := e.linkPoint(t, before.ID, "T28", last)
	assert.InDelta(t, 0, *old.Value, 0, "the former revision keeps its values")
	assert.Nil(t, contributionOf(old, pa4, entity.RoleNumerator))
}

func (e *seriesEnv) objectArea(t *testing.T, id entity.ID) entity.ID {
	t.Helper()
	var area uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT current_area_id FROM normalize.source_object WHERE id = $1", uuid.UUID(id)).Scan(&area))
	return entity.ID(area)
}
