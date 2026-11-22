package calc

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func observationFixture() entity.TaskObservationFacts {
	in := task("J1/P/A", d(0), _intake, status(d(1), _active)).input()
	in.version.TypeNativeID, in.version.TaskKey = "story", "A"
	mapping := m1()
	mapping.TeamID = _team
	mapping.Rules = append(mapping.Rules, entity.MappingRule{
		InstanceID: _instance, Kind: entity.RuleIssueType, NativeRef: "story", Meaning: "Story",
	})
	mr, area := idOf("MR"), idOf("repo")
	author := idOf("author")
	ready := d(2)
	return entity.TaskObservationFacts{
		Task: in.version, TaskEvents: in.events, InstanceID: in.instance, Mapping: mapping,
		AsOf: d(9), KnowledgeSeq: 5,
		Links: []entity.LinkEdge{{Type: entity.LinkTaskMR, From: in.version.TaskID, To: mr, Status: entity.LinkConfirmed}},
		Reviews: []entity.ReviewFacts{{
			AreaID: area, ReviewsGiven: true,
			MRs: []entity.MRVersion{{
				MRID: mr, AreaID: area, HeadSHA: "head", OpenedAt: d(1), ReadyAt: &ready,
				State: "open", ReviewerField: true, AuthorAccountID: &author,
			}},
			MREvents: []entity.MREvent{{ID: idOf("ready"), MRID: mr, Kind: _eventReady, EventTime: ready}},
		}},
		Refs: map[entity.ID]entity.ObjectRef{
			mr: {SourceURL: optional("https://gitlab.example/team/repo/-/merge_requests/1")},
		},
	}
}

func TestTaskObservations_ReviewAndLinkQuestions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		note     func(*entity.TaskObservationFacts)
		kind     entity.ObservationKind
		action   entity.ObservationAction
		question entity.ObservationQuestion
		basis    entity.ObjectKind
	}{
		{"confirmed ready MR", func(*entity.TaskObservationFacts) {}, entity.ObservationAwaitingReviewer, entity.ObservationAssignReviewer, "", entity.ObjectMergeRequest},
		{"candidate only", func(f *entity.TaskObservationFacts) {
			f.Links[0].Status = entity.LinkCandidate
		}, entity.ObservationNoMR, entity.ObservationLinkMR, "", entity.ObjectTask},
		{"rejected candidate", func(f *entity.TaskObservationFacts) {
			f.Links[0].Status = entity.LinkRejected
		}, entity.ObservationNoMR, entity.ObservationClarifyReason, entity.QuestionMRExists, entity.ObjectTask},
		{"same issue key in a second instance", func(f *entity.TaskObservationFacts) {
			jira2 := idOf("J2")
			for _, rule := range slices.Clone(f.Mapping.Rules) {
				rule.InstanceID = jira2
				f.Mapping.Rules = append(f.Mapping.Rules, rule)
			}
			f.Task.TaskID, f.InstanceID = idOf("J2/P/A"), jira2
			f.TaskEvents[0].TaskID = f.Task.TaskID
		}, entity.ObservationNoMR, entity.ObservationClarifyReason, entity.QuestionMRExists, entity.ObjectTask},
		{"reviewer field unavailable", func(f *entity.TaskObservationFacts) {
			f.Reviews[0].MRs[0].ReviewerField = false
		}, entity.ObservationUnknown, entity.ObservationClarifyReason, entity.QuestionReviewerAssigned, entity.ObjectMergeRequest},
		{"review after the cut", func(f *entity.TaskObservationFacts) {
			f.Reviews[0].MRs[0].SourceUpdatedAt = &f.AsOf
			f.Reviews[0].MRs[0].ReviewerAccountIDs = []entity.ID{idOf("reviewer")}
			addObservationReview(f, "human", entity.PersonHuman, d(10), 1)
		}, entity.ObservationAwaitingFirst, entity.ObservationRequestReview, "", entity.ObjectMergeRequest},
		{"future reviewer snapshot without assignment history", func(f *entity.TaskObservationFacts) {
			updated := d(10)
			mr := &f.Reviews[0].MRs[0]
			mr.SourceUpdatedAt, mr.ReviewerAccountIDs = &updated, []entity.ID{idOf("reviewer")}
			f.Reviews[0].MREvents = append(f.Reviews[0].MREvents, entity.MREvent{
				ID: idOf("future assignment"), MRID: mr.MRID, Kind: _eventAssigned, EventTime: updated,
			})
		}, entity.ObservationUnknown, entity.ObservationClarifyReason, entity.QuestionReviewerAssigned, entity.ObjectMergeRequest},
		{"hidden links", func(f *entity.TaskObservationFacts) {
			f.Links = nil
			f.TaskMRsUnavailable = true
		}, entity.ObservationUnknown, entity.ObservationClarifyReason, entity.QuestionDelayReason, entity.ObjectTask},
		{"later confirmation", func(f *entity.TaskObservationFacts) {
			f.Links[0].KnownSeq = f.KnowledgeSeq + 1
		}, entity.ObservationNoMR, entity.ObservationClarifyReason, entity.QuestionMRExists, entity.ObjectTask},
		{"draft", func(f *entity.TaskObservationFacts) {
			f.Reviews[0].MRs[0].Draft = true
			f.Reviews[0].MRs[0].ReadyAt = nil
			f.Reviews[0].MREvents = nil
		}, entity.ObservationUnknown, entity.ObservationClarifyReason, entity.QuestionDelayReason, entity.ObjectTask},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := observationFixture()
			tt.note(&f)
			got := TaskObservations(&f)
			require.Len(t, got, 1)
			assert.Equal(t, tt.kind, got[0].Kind)
			assert.Equal(t, tt.action, got[0].Action)
			assert.Equal(t, tt.question, got[0].Question)
			require.NotEmpty(t, got[0].Basis)
			assert.Equal(t, tt.basis, got[0].Basis[0].Kind)
		})
	}
}

func addObservationReview(f *entity.TaskObservationFacts, actor string, kind entity.PersonKind, at time.Time, seq int64) {
	r := &f.Reviews[0]
	if r.ActorKinds == nil {
		r.ActorKinds = map[entity.ID]entity.PersonKind{}
	}
	id := idOf(actor)
	r.ActorKinds[id] = kind
	r.MREvents = append(r.MREvents, entity.MREvent{
		ID: idOf("review/" + actor), MRID: r.MRs[0].MRID, ActorAccountID: &id,
		Kind: _eventReview, EventTime: at, Knowledge: entity.Knowledge{KnownSeq: seq},
	})
}

func TestTaskObservations_AuthorBotAndHumanResponse(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	mr := &f.Reviews[0].MRs[0]
	mr.ReviewerAccountIDs = []entity.ID{idOf("human")}
	addObservationReview(&f, "author", entity.PersonHuman, d(3), 2)
	addObservationReview(&f, "bot", entity.PersonBot, d(4), 3)
	addObservationReview(&f, "human", entity.PersonHuman, d(5), 6)

	wait := TaskObservations(&f)
	require.Len(t, wait, 1)
	assert.Equal(t, entity.ObservationAwaitingFirst, wait[0].Kind)
	require.Len(t, wait[0].Basis, 1)
	assert.Equal(t, mr.MRID, wait[0].Basis[0].ID)
	assert.Equal(t, f.Refs[mr.MRID].SourceURL, wait[0].Basis[0].SourceURL)
	assert.Equal(t, []entity.ID{idOf("ready")}, wait[0].SourceEventIDs)
	assert.Equal(t, mr.ReadyAt, wait[0].Since)

	f.KnowledgeSeq = 6
	answered := TaskObservations(&f)
	require.Len(t, answered, 1)
	assert.Equal(t, entity.ObservationUnknown, answered[0].Kind)
	assert.Equal(t, entity.QuestionDelayReason, answered[0].Question)
	require.Len(t, answered[0].Basis, 2)
	assert.Equal(t, f.Task.TaskID, answered[0].Basis[0].ID)
	assert.Equal(t, mr.MRID, answered[0].Basis[1].ID)
}

func TestTaskObservations_IndependentOfLinksAndTargetVersion(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	in := task("J1/P/A", d(0), _intake, status(d(1), _active), status(d(4), _blocked), status(d(8), _active)).input()
	f.TaskEvents, f.Task.StatusNativeID = in.events, in.version.StatusNativeID
	f.AsOf = d(7)
	first, latest := entity.TargetVersion{
		ID: idOf("target/1"), TeamID: _team, TargetGroup: entity.TargetGroup{TaskType: "Story", SizeLabel: "S"},
		VersionNo: 1, DurationSeconds: 3 * int64(_day), KnownSeq: 1,
	}, entity.TargetVersion{
		ID: idOf("target/2"), TeamID: _team, TargetGroup: entity.TargetGroup{TaskType: "Story", SizeLabel: "S"},
		VersionNo: 2, DurationSeconds: 10 * int64(_day), KnownSeq: 8,
	}
	f.Targets = []entity.TargetVersion{latest, first}
	before := TaskObservations(&f)
	require.Len(t, before, 3)
	assert.Equal(t, entity.ObservationAwaitingReviewer, before[0].Kind)
	assert.Equal(t, []entity.ID{idOf("ready")}, before[0].SourceEventIDs)
	assert.Equal(t, entity.ObservationBlocked, before[1].Kind)
	assert.Equal(t, []entity.ID{in.events[1].ID}, before[1].SourceEventIDs)
	assert.Equal(t, entity.ObservationOverTarget, before[2].Kind)
	require.NotNil(t, before[2].TargetVersion)
	assert.Equal(t, first.ID, before[2].TargetVersion.ID)
	assert.Equal(t, []entity.ID{in.events[0].ID}, before[2].SourceEventIDs)

	next := f.KnowledgeSeq + 1
	f.Links[0].SupersededSeq = &next
	f.KnowledgeSeq = next
	after := TaskObservations(&f)
	require.Len(t, after, 3)
	assert.Equal(t, entity.ObservationNoMR, after[0].Kind)
	assert.Equal(t, before[1:], after[1:])

	f.KnowledgeSeq, f.AsOf = latest.KnownSeq, d(9)
	require.Len(t, TaskObservations(&f), 1, "both the block and the earlier target are over")
}

func TestTaskObservations_UnknownEpisodeStartAndExtension(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	from := d(3)
	f.HistoryFrom = &from
	f.TaskMRsUnavailable = true
	f.Links = nil
	actual := TaskObservations(&f)
	require.Len(t, actual, 1)
	assert.Nil(t, actual[0].Since)
	assert.Equal(t, entity.QuestionDelayReason, actual[0].Question)

	result := TaskObservations(&f, func(in *entity.TaskObservationFacts) []entity.TaskObservation {
		return []entity.TaskObservation{{
			Kind: entity.ObservationMissingReport, Action: entity.ObservationRestoreReport,
			Basis: []entity.ObjectRef{{Kind: entity.ObjectTask, ID: in.Task.TaskID}},
		}}
	})
	require.Len(t, result, 1)
	assert.Equal(t, entity.ObservationMissingReport, result[0].Kind)
}

func TestTaskObservations_CurrentPipeline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		outcome entity.CIOutcome
		history entity.AttemptHistory
		at      time.Time
		want    entity.ObservationKind
	}{
		{"failed", entity.CIFailed, entity.AttemptHistoryComplete, d(9), entity.ObservationCIFailed},
		{"running", entity.CIRunning, entity.AttemptHistoryComplete, d(9), entity.ObservationCIRunning},
		{"completion after as of", entity.CISuccess, entity.AttemptHistoryComplete, d(6), entity.ObservationCIRunning},
		{"passed after retry", entity.CISuccess, entity.AttemptHistoryComplete, d(9), entity.ObservationCIRetried},
		{"truncated retry history", entity.CISuccess, entity.AttemptHistoryTruncated, d(9), entity.ObservationCIRetried},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := observationFixture()
			f.AsOf = tt.at
			pipeline, oldHead, older, unlinked := idOf("pipeline"), idOf("old head"), idOf("older"), idOf("unlinked")
			mr := &f.Reviews[0].MRs[0]
			start, finish, retryStart := d(4), d(7), d(6).Add(time.Minute)
			firstFinished := d(5)
			duration := int64(90)
			f.CI.Runs = []entity.CIRunVersion{
				{RunID: oldHead, AreaID: mr.AreaID, SHA: "previous", StartedAt: &firstFinished, Outcome: entity.CIFailed},
				{RunID: pipeline, AreaID: mr.AreaID, SHA: mr.HeadSHA, StartedAt: &start, FinishedAt: &finish, Outcome: tt.outcome, AttemptHistory: tt.history},
				{RunID: older, AreaID: mr.AreaID, SHA: mr.HeadSHA, StartedAt: mr.ReadyAt, Outcome: entity.CIFailed},
				{RunID: unlinked, AreaID: mr.AreaID, SHA: mr.HeadSHA, StartedAt: &firstFinished, Outcome: entity.CIFailed},
			}
			if tt.outcome == entity.CIRunning {
				f.CI.Runs[1].FinishedAt = nil
			}
			for _, id := range []entity.ID{oldHead, pipeline, older} {
				f.Links = append(f.Links, entity.LinkEdge{Type: entity.LinkMRRun, From: mr.MRID, To: id, Status: entity.LinkConfirmed})
			}
			f.CI.Attempts = []entity.CIAttemptVersion{
				{RunID: pipeline, AttemptID: idOf("failed attempt"), JobKey: "tests", AttemptNo: 1, Outcome: entity.CIFailed, StartedAt: &start, FinishedAt: &firstFinished},
				{RunID: pipeline, AttemptID: idOf("retry attempt"), JobKey: "tests", AttemptNo: 2, Outcome: entity.CISuccess, StartedAt: &retryStart, FinishedAt: &finish, DurationS: &duration},
			}
			got := TaskObservations(&f)
			require.Len(t, got, 2, "there is one current pipeline")
			assert.Equal(t, entity.ObservationAwaitingReviewer, got[0].Kind)
			assert.Equal(t, tt.want, got[1].Kind)
			assert.Equal(t, entity.ObservationCheckPipeline, got[1].Action)
			require.GreaterOrEqual(t, len(got[1].Basis), 2)
			assert.Equal(t, pipeline, got[1].Basis[0].ID)
			assert.Equal(t, idOf("failed attempt"), got[1].Basis[1].ID)
			if tt.want == entity.ObservationCIRetried {
				assert.Equal(t, &entity.TaskRetry{
					Attempts: 1, Seconds: 90, AttemptsLowerBound: tt.history == entity.AttemptHistoryTruncated,
					SecondsLowerBound: tt.history == entity.AttemptHistoryTruncated, ObservedOutcomeFlip: true,
				}, got[1].Retry)
				require.Len(t, got[1].Basis, 3)
				assert.Equal(t, idOf("retry attempt"), got[1].Basis[2].ID)
			}
		})
	}
}

func TestTaskObservations_OldHeadAndTerminalStateWithoutFinishTime(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	mr := &f.Reviews[0].MRs[0]
	future, start := d(10), d(4)
	mr.SourceUpdatedAt, mr.HeadSHA = &future, "new-head"
	f.MRRevisions = []entity.MRRevision{
		{MRID: mr.MRID, HeadSHA: "new-head", CreatedAt: future},
		{MRID: mr.MRID, HeadSHA: "old-head", CreatedAt: d(2)},
		{MRID: mr.MRID, HeadSHA: "unseen-head", CreatedAt: d(3), Knowledge: entity.Knowledge{KnownSeq: 6}},
	}
	run := entity.CIRunVersion{
		RunID: idOf("historic run"), AreaID: mr.AreaID, SHA: "old-head", StartedAt: &start,
		SourceUpdatedAt: &future, Outcome: entity.CIFailed,
	}
	f.Links = append(f.Links, entity.LinkEdge{Type: entity.LinkMRRun, Status: entity.LinkConfirmed, From: mr.MRID, To: run.RunID})
	f.CI.Runs = []entity.CIRunVersion{run}
	got := TaskObservations(&f)
	require.Len(t, got, 1)
	assert.Equal(t, entity.QuestionReviewerAssigned, got[0].Question)

	finish := d(7)
	f.CI.Runs[0].FinishedAt = &finish
	got = TaskObservations(&f)
	require.Len(t, got, 2)
	assert.Equal(t, entity.ObservationCIFailed, got[1].Kind)
	assert.Equal(t, run.RunID, got[1].Basis[0].ID)
	assert.Equal(t, &finish, got[1].Since)
}

func TestTaskObservations_RetryWithoutKnownDuration(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	mr := &f.Reviews[0].MRs[0]
	start, finish, future := d(4), d(7), d(10)
	run := idOf("pipeline")
	f.Links = append(f.Links, entity.LinkEdge{Type: entity.LinkMRRun, Status: entity.LinkConfirmed, From: mr.MRID, To: run})
	f.CI.Runs = []entity.CIRunVersion{{
		RunID: run, AreaID: mr.AreaID, SHA: mr.HeadSHA, StartedAt: &start,
		FinishedAt: &finish, Outcome: entity.CISuccess, AttemptHistory: entity.AttemptHistoryComplete,
	}}
	duration := int64(45)
	f.CI.Attempts = []entity.CIAttemptVersion{
		{RunID: run, AttemptID: idOf("success"), AttemptNo: 1, JobKey: "tests", Outcome: entity.CISuccess, StartedAt: &start},
		{
			RunID: run, AttemptID: idOf("second try"), AttemptNo: 2, JobKey: "tests", Outcome: entity.CISuccess, StartedAt: &start,
			ObservedAt: future, DurationS: &duration,
		},
	}
	got := TaskObservations(&f)
	require.Len(t, got, 2)
	require.Equal(t, entity.ObservationCIRetried, got[1].Kind)
	assert.Equal(t, &entity.TaskRetry{Attempts: 1, SecondsLowerBound: true}, got[1].Retry)
}
