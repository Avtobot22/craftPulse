package normalize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) InsertRejectedReport(ctx context.Context, org entity.ID, report entity.ScanReport, reason string) error {
	no, err := recordNo(report.Record.RecordNo)
	if err != nil {
		return err
	}
	err = r.q(ctx).InsertRejectedReport(ctx, InsertRejectedReportParams{
		OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(report.Record.ReceiptID), RecordNo: no,
		KnownSeq: report.KnownSeq, AreaID: uuid.UUID(report.AreaID), ComponentID: nullID(report.ComponentID),
		ReportKind: report.Kind, Language: report.Language, Sha: text(report.SHA), RunID: nullID(report.RunID),
		AttemptID: nullID(report.AttemptID), ReasonCode: reason,
	})
	if err != nil {
		return fmt.Errorf("normalize: insert rejected report evidence: %w", err)
	}
	return nil
}

func (r *Repo) ReadReportEvidence(ctx context.Context, org entity.ID, scope entity.ReportScope) ([]entity.ReportEvidence, error) {
	rows, err := r.q(ctx).ReadReportEvidence(ctx, ReadReportEvidenceParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(scope.AreaID), Sha: scope.SHA, KnownSeq: scope.KnownSeq,
		Pipeline: scope.Pipeline != nil, RunIds: ids(scope.RunIDs), AttemptIds: ids(scope.AttemptIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read report evidence: %w", err)
	}
	out := make([]entity.ReportEvidence, len(rows))
	for i := range rows {
		row := &rows[i]
		e := &out[i]
		*e = entity.ReportEvidence{
			ID: entity.ID(row.ID), RunID: idPtr(row.RunID), ComponentID: idPtr(row.ComponentID), Kind: row.ReportKind,
			Language: row.Language, Format: row.ReportFormat, Producer: row.Producer, ProducerVersion: row.ProducerVersion,
			ScopeHash: row.ScopeHash, ExitClass: row.ExitClass, Completeness: row.Completeness, KnownSeq: row.KnownSeq, RejectedReason: row.RejectedReason,
		}
		if err := json.Unmarshal(row.SkippedTargets, &e.SkippedTargets); err != nil {
			return nil, fmt.Errorf("normalize: decode skipped report targets: %w", err)
		}
	}
	return out, nil
}

func (r *Repo) ReadTestExecutions(ctx context.Context, org entity.ID, reports []entity.ID, q entity.TestExecutionQuery) ([]entity.ReportExecution, error) {
	rows, err := r.q(ctx).ReadTestExecutions(ctx, ReadTestExecutionsParams{
		OrganizationID: uuid.UUID(org), ReportIds: ids(reports), Outcome: q.Outcome, AfterID: nullID(q.After),
		CurrentOnly: q.Limit > 0, PageSize: int32(q.Limit), //nolint:gosec // page size is bounded by the use case
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read test executions: %w", err)
	}
	out := make([]entity.ReportExecution, len(rows))
	for i := range rows {
		row := &rows[i]
		x := &out[i]
		*x = entity.ReportExecution{
			ID: entity.ID(row.ID), ReportID: entity.ID(row.ReportID), CaseID: entity.ID(row.TestCaseID), Superseded: row.Superseded,
			TestExecution: entity.TestExecution{
				CaseKey: row.CaseKey, AttemptID: idPtr(row.AttemptID), ConfigFingerprint: row.ConfigFingerprint,
				Ordinal: int(row.Ordinal), Outcome: row.Outcome, RunStartedAt: row.RunStartedAt,
			},
		}
		if row.Suite.Valid {
			x.Suite = &row.Suite.String
		}
		if row.DurationMs.Valid {
			x.DurationMS = &row.DurationMs.Int64
		}
	}
	return out, nil
}

func (r *Repo) ReadReportCoverage(ctx context.Context, org entity.ID, reports []entity.ID) ([]entity.ReportCoverage, error) {
	rows, err := r.q(ctx).ReadReportCoverage(ctx, ReadReportCoverageParams{OrganizationID: uuid.UUID(org), ReportIds: ids(reports)})
	if err != nil {
		return nil, fmt.Errorf("normalize: read coverage: %w", err)
	}
	out := make([]entity.ReportCoverage, len(rows))
	for i, row := range rows {
		out[i] = entity.ReportCoverage{ID: entity.ID(row.ID), ReportID: entity.ID(row.ReportID), CoverageFact: entity.CoverageFact{
			Path: row.Path, Kind: row.Kind, Covered: int(row.Covered), Executable: int(row.Executable), LineMapKey: row.LineMapKey.String,
		}}
	}
	return out, nil
}
