package change

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
)

type taskObservationTx struct{ repo.Transactor }

func (taskObservationTx) InOrgTx(ctx context.Context, _ entity.ID, fn func(context.Context) error) error {
	return fn(ctx)
}

type taskSummaryReader func([]entity.MRVersion, int64, time.Time) []entity.TaskSummaryRow

func (r taskSummaryReader) ReadTaskSummaryRows(_ context.Context, _ entity.ID, mrs []entity.MRVersion, seq int64, at time.Time) ([]entity.TaskSummaryRow, error) {
	return r(mrs, seq, at), nil
}

func TestTaskObservations_CurrentHead(t *testing.T) {
	t.Parallel()
	at := time.Unix(100, 0).UTC()
	const seq = 7
	task, mr, other, component := entity.ID{1}, entity.ID{2}, entity.ID{3}, entity.ID{4}
	ref := func(id entity.ID) entity.ObjectRef { return entity.ObjectRef{Kind: entity.ObjectMergeRequest, ID: id} }
	missing := entity.TaskSummaryRow{
		MRID: mr, HeadSHA: "head", BuiltAt: at, PipelineState: "reports_missing",
		ComponentPath: new("billing"),
		SummaryItem: entity.SummaryItem{
			Kind: "not_evaluated", ComponentID: &component, AnalysisKind: "code", ReasonCode: "scan_missing",
		},
	}
	newFinding := func(id byte, rank int) entity.TaskSummaryRow {
		return entity.TaskSummaryRow{
			MRID: mr, HeadSHA: "head", BuiltAt: at, PipelineState: "complete", HasBase: true,
			SummaryItem: entity.SummaryItem{
				Kind: entity.FindingNew, AnalysisKind: "code", FindingID: new(entity.ID{id}), OccurrenceID: new(entity.ID{id + 10}), Rank: new(rank),
			},
		}
	}
	first, second := newFinding(5, 1), newFinding(6, 2)
	noBase := first
	noBase.HasBase, noBase.Kind = false, entity.FindingNewlyObserved
	accepted := first
	accepted.Kind = "accepted"
	otherMissing := missing
	otherMissing.MRID = other
	newHead := missing
	newHead.HeadSHA = "pushed"
	running := missing
	running.PipelineState = "running"
	tests := []struct {
		name   string
		status entity.LinkStatus
		head   string
		twoMRs bool
		rows   []entity.TaskSummaryRow
		want   []entity.ObservationKind
	}{
		{name: "missing report", rows: []entity.TaskSummaryRow{missing}, want: []entity.ObservationKind{entity.ObservationMissingReport}},
		{name: "applicable findings with comparable base", rows: []entity.TaskSummaryRow{second, accepted, first, first}, want: []entity.ObservationKind{entity.ObservationNewFinding}},
		{name: "newly observed without comparable base", rows: []entity.TaskSummaryRow{noBase}},
		{name: "candidate alone", status: entity.LinkCandidate, rows: []entity.TaskSummaryRow{missing, first}},
		{name: "two merge requests", twoMRs: true, rows: []entity.TaskSummaryRow{missing, otherMissing}, want: []entity.ObservationKind{entity.ObservationMissingReport, entity.ObservationMissingReport}},
		{name: "new head without reports", head: "pushed", rows: []entity.TaskSummaryRow{first, newHead}, want: []entity.ObservationKind{entity.ObservationMissingReport}},
		{name: "running pipeline", rows: []entity.TaskSummaryRow{running}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			head := tt.head
			if head == "" {
				head = "head"
			}
			status := tt.status
			if status == "" {
				status = entity.LinkConfirmed
			}
			f := entity.TaskObservationFacts{
				Task: entity.TaskVersion{TaskID: task}, KnowledgeSeq: seq, AsOf: at,
				Refs:    map[entity.ID]entity.ObjectRef{mr: ref(mr), other: ref(other)},
				Links:   []entity.LinkEdge{{From: task, To: mr, Status: status, Type: entity.LinkTaskMR}},
				Reviews: []entity.ReviewFacts{{MRs: []entity.MRVersion{{MRID: mr, HeadSHA: head}}}},
			}
			if tt.twoMRs {
				f.Links = append(f.Links, entity.LinkEdge{From: task, To: other, Status: entity.LinkConfirmed, Type: entity.LinkTaskMR})
				f.Reviews = append(f.Reviews, entity.ReviewFacts{MRs: []entity.MRVersion{{MRID: other, HeadSHA: "head"}}})
			}
			read := false
			reader := taskSummaryReader(func(mrs []entity.MRVersion, cut int64, date time.Time) []entity.TaskSummaryRow {
				read = true
				require.Len(t, mrs, len(f.Links))
				assert.Equal(t, head, mrs[0].HeadSHA)
				assert.EqualValues(t, seq, cut)
				assert.Equal(t, at, date)
				return tt.rows
			})
			u := New(Deps{Tx: taskObservationTx{}, TaskSummaries: reader})
			got, hidden, err := u.TaskObservations(t.Context(), entity.Subject{}, entity.Scope{}, &f)
			require.NoError(t, err)
			assert.Equal(t, status == entity.LinkConfirmed, read)
			assert.False(t, hidden)
			require.Len(t, got, len(tt.want))
			for i, o := range got {
				assert.Equal(t, tt.want[i], o.Kind)
				assert.Equal(t, &at, o.Since)
				require.NotEmpty(t, o.Basis)
				assert.Equal(t, f.Links[i].To, o.Basis[0].ID)
				if o.Kind == entity.ObservationMissingReport {
					assert.Equal(t, entity.ObservationRestoreReport, o.Action)
					assert.Equal(t, new("CODE · компонент billing · отчёт не получен"), o.Basis[0].Title)
				} else {
					assert.Equal(t, entity.ObservationReviewFinding, o.Action)
					require.Len(t, o.Basis, 2)
					assert.Equal(t, *first.FindingID, o.Basis[1].ID)
					assert.True(t, o.Basis[1].Indirect)
					assert.Equal(t, new("CODE · новых находок в MR: 2"), o.Basis[1].Title)
				}
			}
		})
	}
}

func TestTaskObservations_UnavailableReports(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ reason, label string }{
		{"scan_missing", "отчёт не получен"},
		{"scan_failed", "проверка завершилась с ошибкой"},
		{"import_failed", "отчёт не принят"},
		{"scan_incomplete", "отчёт неполный"},
	} {
		t.Run(tt.reason, func(t *testing.T) {
			t.Parallel()
			mr, project, area, component := entity.ID{1}, entity.ID{2}, entity.ID{3}, entity.ID{4}
			row := entity.TaskSummaryRow{
				MRID: mr, ProjectID: project, HeadSHA: "head", PipelineState: "reports_missing", ComponentPath: new("billing"),
				SummaryItem: entity.SummaryItem{
					Kind: "not_evaluated", AnalysisKind: entity.AnalysisSAST,
					ComponentID: &component, ReasonCode: tt.reason, RequiredRight: "security",
				},
			}
			mrs := map[entity.ID]*entity.MRVersion{mr: {MRID: mr, AreaID: area, HeadSHA: "head"}}
			refs := map[entity.ID]entity.ObjectRef{mr: {Kind: entity.ObjectMergeRequest, ID: mr}}
			rights := map[entity.FindingAccess]bool{{AreaID: area, Kind: "security"}: true}
			got, hidden := technicalObservations(refs, mrs, []entity.TaskSummaryRow{row}, []entity.ID{project}, rights)
			assert.False(t, hidden)
			require.Len(t, got, 1)
			assert.Equal(t, new("SAST · компонент billing · "+tt.label), got[0].Basis[0].Title)
			got, hidden = technicalObservations(refs, mrs, []entity.TaskSummaryRow{row}, []entity.ID{project}, nil)
			assert.True(t, hidden)
			assert.Empty(t, got)
		})
	}
}
