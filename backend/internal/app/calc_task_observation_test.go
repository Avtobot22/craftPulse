package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestTaskObservations_FromAdapterTaskAndLinkDecision(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	submit := e.mrImporter(t, admin)
	rc := submit(mrRepository(t, "42", "team/payments", "r1"))
	ctx := t.Context()
	area, err := e.runner.norm.AreaOf(ctx, e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, "42")
	require.NoError(t, err)
	_, err = e.owner.Exec(ctx, "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		uuid.UUID(e.orgID), e.project, uuid.UUID(area), e.clock.Now())
	require.NoError(t, err)

	ready := _linkMerged
	head := strings.Repeat("a", 40)
	state := func(revision string, reviewers []string, updated time.Time) *envelope.Record {
		return mrRecord(t, "merge_request", "9001", "", revision, envelope.MergeRequestV1{
			ProjectNativeId: "42", Iid: ptr(7), Title: ptr("Payment retry"), State: "open",
			AuthorAccountNativeId: ptr("author-observation"), SourceBranch: "feature/PA-4-retry", TargetBranch: "main", HeadSha: head,
			OpenedAt: ready.Add(-time.Hour), ReadyAt: &ready, UpdatedAt: &updated,
			ReviewerField: ptr(true), ReviewerAccountNativeIds: &reviewers,
		})
	}
	submit(state("v1", []string{}, ready), reviewEvent(t, "ready-observation", "ready", "author-observation", ready),
		accountRecord(t, "bot-observation", "Review bot"), accountRecord(t, "human-observation", "Reviewer"))

	tx := persistent.NewTransactor(e.app)
	linkService := linkuc.New(linkuc.Deps{
		Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx,
		Auditor: audit.New(auditrepo.NewRepo(e.app), tx, e.clock), Clock: e.clock, NewID: newID,
	})
	task := e.objectID(t, "task", "20004")
	mr := e.objectID(t, "merge_request", "9001")
	sourceURL := _gitlabInstance + "/team/payments/-/merge_requests/7"
	taskArea := e.objectArea(t, task)
	read := func(seq int64) (entity.TaskObservationFacts, entity.ID) {
		t.Helper()
		if seq == 0 {
			require.NoError(t, tx.InOrgTx(ctx, e.orgID, func(ctx context.Context) error {
				latest, _, err := platform.NewRepo(e.app).Clock(ctx, e.orgID)
				seq = latest
				return err
			}))
		}
		workflow, err := e.runner.norm.AreaFacts(ctx, e.orgID, taskArea, seq)
		require.NoError(t, err)
		i := slices.IndexFunc(workflow.Tasks, func(v entity.TaskVersion) bool { return v.TaskID == task })
		require.NotEqual(t, -1, i)
		work, found, err := e.process.MappingAt(ctx, e.orgID, entity.ID(e.team), seq)
		require.NoError(t, err)
		require.True(t, found)
		reviews, err := e.runner.norm.ReviewFacts(ctx, e.orgID, area, seq)
		require.NoError(t, err)
		j := slices.IndexFunc(reviews.MREvents, func(v entity.MREvent) bool { return v.MRID == mr && v.Kind == "ready" })
		require.NotEqual(t, -1, j)
		edges, err := linkService.EdgesAt(ctx, e.orgID, task, entity.LinksFrom, seq)
		require.NoError(t, err)
		return entity.TaskObservationFacts{
			AsOf: _seriesAsOf, KnowledgeSeq: seq, Task: workflow.Tasks[i], TaskEvents: workflow.Events,
			InstanceID: workflow.InstanceID, Mapping: &work, Links: edges, Reviews: []entity.ReviewFacts{reviews},
			Refs: map[entity.ID]entity.ObjectRef{mr: {SourceURL: &sourceURL}},
		}, reviews.MREvents[j].ID
	}
	check := func(f *entity.TaskObservationFacts, kind entity.ObservationKind) entity.TaskObservation {
		t.Helper()
		got := calc.TaskObservations(f)
		i := slices.IndexFunc(got, func(o entity.TaskObservation) bool { return o.Kind == kind })
		require.NotEqual(t, -1, i, "observations: %+v", got)
		return got[i]
	}

	initial, readyID := read(0)
	beforeConfirmation := initial.KnowledgeSeq
	require.Len(t, initial.Links, 1)
	require.Equal(t, entity.LinkCandidate, initial.Links[0].Status)
	missing := check(&initial, entity.ObservationNoMR)
	assert.Equal(t, entity.ObservationLinkMR, missing.Action)
	assert.Equal(t, []entity.ObjectRef{{ID: task, Kind: entity.ObjectTask}}, missing.Basis)
	require.NoError(t, tx.InOrgTx(ctx, e.orgID, func(ctx context.Context) error {
		_, err := linkService.Decide(ctx, e.orgID, admin.UserID, initial.Links[0].ID, entity.LinkConfirm, entity.ID(uuid.New()))
		return err
	}))
	confirmed, _ := read(0)
	reviewer := check(&confirmed, entity.ObservationAwaitingReviewer)
	assert.Equal(t, []entity.ObjectRef{{ID: mr, Kind: entity.ObjectMergeRequest, SourceURL: &sourceURL}}, reviewer.Basis)
	assert.Equal(t, []entity.ID{readyID}, reviewer.SourceEventIDs)
	assert.Equal(t, ready, *reviewer.Since)

	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	for native, kind := range map[string]entity.PersonKind{"bot-observation": entity.PersonBot, "human-observation": entity.PersonHuman} {
		var id uuid.UUID
		require.NoError(t, e.owner.QueryRow(ctx, "SELECT id FROM org.source_account WHERE native_account_id = $1", native).Scan(&id))
		assigned, err := orgs.AssignAccounts(ctx, admin, entity.ID(uuid.New()), []entity.AssignmentRequest{
			{SourceAccountID: entity.ID(id), ActorKind: kind, CreatePerson: true},
		})
		require.NoError(t, err)
		require.Len(t, assigned, 1)
		require.Equal(t, entity.AssignmentSaved, assigned[0].Outcome)
	}
	botAt := ready.Add(24 * time.Hour)
	submit(state("v2", []string{"human-observation"}, botAt), reviewEvent(t, "review-bot", "review", "bot-observation", botAt))
	withBot, _ := read(0)
	waiting := check(&withBot, entity.ObservationAwaitingFirst)
	assert.Equal(t, []entity.ID{readyID}, waiting.SourceEventIDs)
	assert.Equal(t, reviewer.Basis, waiting.Basis)

	submit(reviewEvent(t, "review-human", "review", "human-observation", botAt.Add(time.Hour)))
	withHuman, _ := read(0)
	assert.False(t, slices.ContainsFunc(calc.TaskObservations(&withHuman), func(o entity.TaskObservation) bool {
		return o.Kind == entity.ObservationAwaitingFirst || o.Kind == entity.ObservationAwaitingReviewer
	}))
	unknown := check(&withHuman, entity.ObservationUnknown)
	assert.Equal(t, entity.QuestionDelayReason, unknown.Question)
	require.Len(t, unknown.Basis, 2)
	assert.Equal(t, reviewer.Basis[0], unknown.Basis[1])
	old, _ := read(beforeConfirmation)
	assert.Equal(t, missing, check(&old, entity.ObservationNoMR))
}
