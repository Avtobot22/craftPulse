package change

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// TaskSummaries supplies the current-head summary rows on the card's knowledge and date cut.
type TaskSummaries interface {
	ReadTaskSummaryRows(context.Context, entity.ID, []entity.MRVersion, int64, time.Time) ([]entity.TaskSummaryRow, error)
}

// TaskObservations evaluates technical rules using only the confirmed, visible MR links from the task card.
func (u *UseCase) TaskObservations(ctx context.Context, s entity.Subject, scope entity.Scope, f *entity.TaskObservationFacts) ([]entity.TaskObservation, bool, error) {
	mrs, byID := taskObservationMRs(f)
	if len(mrs) == 0 {
		return nil, false, nil
	}
	var rows []entity.TaskSummaryRow
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		rows, err = u.d.TaskSummaries.ReadTaskSummaryRows(ctx, s.OrganizationID, mrs, f.KnowledgeSeq, f.AsOf)
		return err
	})
	if err != nil {
		return nil, false, err
	}

	projects := scope.SecurityProjects()
	var targets []entity.FindingAccess
	for i := range rows {
		r := &rows[i]
		if taskSummaryApplicable(r, byID[r.MRID]) && taskSummarySensitive(r) && slices.Contains(projects, r.ProjectID) {
			target := entity.FindingAccess{AreaID: byID[r.MRID].AreaID, Kind: "security"}
			if !slices.Contains(targets, target) {
				targets = append(targets, target)
			}
		}
	}
	var allowed map[entity.FindingAccess]bool
	if len(targets) > 0 {
		allowed, err = u.d.FindingRights.VisibleFindings(ctx, s, scope, "getTask", targets)
		if err != nil {
			return nil, false, err
		}
	}
	out, redacted := technicalObservations(f.Refs, byID, rows, projects, allowed)
	return out, redacted, nil
}

func taskObservationMRs(f *entity.TaskObservationFacts) (mrs []entity.MRVersion, byID map[entity.ID]*entity.MRVersion) {
	confirmed := make(map[entity.ID]bool)
	for i := range f.Links {
		e := &f.Links[i]
		if e.From != f.Task.TaskID || e.Type != entity.LinkTaskMR || e.Status != entity.LinkConfirmed ||
			!taskKnownOnCut(e.Knowledge, f.KnowledgeSeq) || !e.Validity.Contains(f.AsOf) ||
			e.DecidedAt != nil && e.DecidedAt.After(f.AsOf) {
			continue
		}
		confirmed[e.To] = true
	}
	byID = make(map[entity.ID]*entity.MRVersion, len(confirmed))
	mrs = make([]entity.MRVersion, 0, len(confirmed))
	for i := range f.Reviews {
		for j := range f.Reviews[i].MRs {
			mr := &f.Reviews[i].MRs[j]
			if !confirmed[mr.MRID] || byID[mr.MRID] != nil || mr.HeadSHA == "" ||
				mr.OpenedAt.After(f.AsOf) || !taskKnownOnCut(mr.Knowledge, f.KnowledgeSeq) {
				continue
			}
			mrs, byID[mr.MRID] = append(mrs, *mr), mr
		}
	}
	return mrs, byID
}

func taskKnownOnCut(k entity.Knowledge, seq int64) bool {
	return k.KnownSeq <= seq && (k.SupersededSeq == nil || seq < *k.SupersededSeq)
}

func taskSummaryApplicable(r *entity.TaskSummaryRow, mr *entity.MRVersion) bool {
	if mr == nil || r.HeadSHA != mr.HeadSHA {
		return false
	}
	switch r.Kind {
	case "not_evaluated":
		return r.PipelineState != "running" && r.ComponentID != nil && r.ReasonCode != ""
	case entity.FindingNew:
		// The summary uses "new" only for a comparable stream and keeps accepted findings in separate rows.
		return r.HasBase && r.FindingID != nil && r.OccurrenceID != nil
	default:
		return false
	}
}

func taskSummarySensitive(r *entity.TaskSummaryRow) bool {
	return r.RequiredRight == "security" || entity.SensitiveSummaryAnalysis(r.AnalysisKind)
}

type taskFindingGroup struct {
	mr   entity.ID
	kind string
}

type taskNewFindings struct {
	first *entity.TaskSummaryRow
	ids   map[entity.ID]bool
}

func technicalObservations(refs map[entity.ID]entity.ObjectRef, mrs map[entity.ID]*entity.MRVersion, rows []entity.TaskSummaryRow,
	projects []entity.ID, allowed map[entity.FindingAccess]bool,
) ([]entity.TaskObservation, bool) {
	var out []entity.TaskObservation
	groups := make(map[taskFindingGroup]*taskNewFindings)
	var order []taskFindingGroup
	redacted := false
	for i := range rows {
		r := &rows[i]
		mr := mrs[r.MRID]
		if !taskSummaryApplicable(r, mr) {
			continue
		}
		target := entity.FindingAccess{AreaID: mr.AreaID, Kind: "security"}
		if taskSummarySensitive(r) && (!slices.Contains(projects, r.ProjectID) || !allowed[target]) {
			redacted = true
			continue
		}
		if r.Kind == "not_evaluated" {
			out = append(out, missingReportObservation(refs[r.MRID], r))
			continue
		}
		key := taskFindingGroup{mr: r.MRID, kind: r.AnalysisKind}
		g := groups[key]
		if g == nil {
			g = &taskNewFindings{first: r, ids: make(map[entity.ID]bool)}
			groups[key] = g
			order = append(order, key)
		}
		g.ids[*r.FindingID] = true
		if compareTaskFindings(r, g.first) < 0 {
			g.first = r
		}
	}
	for _, key := range order {
		g := groups[key]
		out = append(out, newFindingObservation(refs[key.mr], g.first, len(g.ids)))
	}
	return out, redacted
}

func compareTaskFindings(a, b *entity.TaskSummaryRow) int {
	const unranked = 6
	ar, br := unranked, unranked
	if a.Rank != nil {
		ar = *a.Rank
	}
	if b.Rank != nil {
		br = *b.Rank
	}
	return cmp.Or(cmp.Compare(ar, br), entity.CompareSummarySignals(&a.SummaryItem, &b.SummaryItem))
}

func missingReportObservation(mr entity.ObjectRef, r *entity.TaskSummaryRow) entity.TaskObservation {
	component := r.ComponentID.String()
	if r.ComponentPath != nil {
		component = cmp.Or(*r.ComponentPath, "/")
	}
	reason := r.ReasonCode
	switch reason {
	case "scan_missing":
		reason = "отчёт не получен"
	case "scan_failed":
		reason = "проверка завершилась с ошибкой"
	case "import_failed":
		reason = "отчёт не принят"
	case "scan_incomplete":
		reason = "отчёт неполный"
	}
	mr.Title = new(fmt.Sprintf("%s · компонент %s · %s", strings.ToUpper(r.AnalysisKind), component, reason))
	return entity.TaskObservation{
		Kind: entity.ObservationMissingReport, Action: entity.ObservationRestoreReport, Since: new(r.BuiltAt.UTC()),
		Basis: []entity.ObjectRef{mr},
	}
}

func newFindingObservation(mr entity.ObjectRef, r *entity.TaskSummaryRow, count int) entity.TaskObservation {
	finding := entity.ObjectRef{
		Kind: entity.ObjectFinding, ID: *r.FindingID, Indirect: true, SourceURL: mr.SourceURL,
		Title: new(fmt.Sprintf("%s · новых находок в MR: %d", strings.ToUpper(r.AnalysisKind), count)),
	}
	if r.RuleID != "" {
		finding.DisplayKey = new(r.RuleID)
	}
	return entity.TaskObservation{
		Kind: entity.ObservationNewFinding, Action: entity.ObservationReviewFinding, Since: new(r.BuiltAt.UTC()),
		Basis: []entity.ObjectRef{mr, finding},
	}
}
