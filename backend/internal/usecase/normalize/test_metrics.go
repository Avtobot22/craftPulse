package normalize

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// ReportKnowledge freezes the first page or rejects a cursor outside the accessible knowledge history.
func (u *UseCase) ReportKnowledge(ctx context.Context, org entity.ID, known *int64) (int64, error) {
	var seq int64
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var from int64
		var err error
		seq, from, err = u.d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		if known != nil {
			if *known < from || *known > seq {
				return entity.ErrInvalidCursor
			}
			seq = *known
		}
		return nil
	})
	return seq, err
}

// TestCoverageFacts reads the immutable report facts of a repository/SHA or of the scoped pipeline.
func (u *UseCase) TestCoverageFacts(ctx context.Context, org entity.ID, scope entity.ReportScope) (entity.ReportFacts, error) {
	out := entity.ReportFacts{Scope: scope}
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out.Reports, err = u.d.Repo.ReadReportEvidence(ctx, org, scope)
		if err != nil {
			return err
		}
		if tests := idsByReportKind(out.Reports, entity.AnalysisJUnit); len(tests) > 0 {
			out.Executions, err = u.d.Repo.ReadTestExecutions(ctx, org, tests, entity.TestExecutionQuery{})
			if err != nil {
				return err
			}
		}
		if coverage := idsByReportKind(out.Reports, entity.AnalysisCoverage); len(coverage) > 0 {
			out.Coverage, err = u.d.Repo.ReadReportCoverage(ctx, org, coverage)
		}
		return err
	})
	return out, err
}

// PipelineTestExecutions reads one page from the reports visible at the cursor's knowledge number.
func (u *UseCase) PipelineTestExecutions(ctx context.Context, org entity.ID, scope entity.ReportScope, q entity.TestExecutionQuery) (entity.TestExecutionList, error) {
	out := entity.TestExecutionList{KnownSeq: scope.KnownSeq}
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		reports, err := u.d.Repo.ReadReportEvidence(ctx, org, scope)
		if err != nil {
			return err
		}
		tests := idsByReportKind(reports, entity.AnalysisJUnit)
		if len(tests) == 0 {
			return nil
		}
		limit := q.Limit
		q.Limit++
		out.Items, err = u.d.Repo.ReadTestExecutions(ctx, org, tests, q)
		if err != nil {
			return err
		}
		if len(out.Items) > limit {
			next := out.Items[limit-1].ID
			out.Items, out.Next = out.Items[:limit], &next
		}
		return nil
	})
	return out, err
}

func idsByReportKind(reports []entity.ReportEvidence, kind string) []entity.ID {
	var ids []entity.ID
	for i := range reports {
		if reports[i].Kind == kind && reports[i].RejectedReason == "" {
			ids = append(ids, reports[i].ID)
		}
	}
	return ids
}
