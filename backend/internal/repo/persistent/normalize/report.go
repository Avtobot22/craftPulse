package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) ReportedRecords(ctx context.Context, orgID, receiptID entity.ID, nos []int) ([]int, error) {
	list := make([]int32, len(nos))
	for i, no := range nos {
		n, err := recordNo(no)
		if err != nil {
			return nil, err
		}
		list[i] = n
	}
	rows, err := r.q(ctx).ReportedRecords(ctx, ReportedRecordsParams{OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNos: list})
	if err != nil {
		return nil, fmt.Errorf("normalize: reported records: %w", err)
	}
	out := make([]int, len(rows))
	for i, no := range rows {
		out[i] = int(no)
	}
	return out, nil
}

// InsertReport writes a report with its findings in the caller's transaction.
func (r *Repo) InsertReport(ctx context.Context, orgID entity.ID, rep *entity.ScanReport, findings []entity.FindingOccurrence) error {
	no, err := recordNo(rep.Record.RecordNo)
	if err != nil {
		return err
	}
	skipped, err := json.Marshal(rep.SkippedTargets)
	if err != nil {
		return fmt.Errorf("normalize: encode skipped targets: %w", err)
	}
	if rep.SkippedTargets == nil {
		skipped = []byte("[]")
	}
	p := InsertReportParams{
		OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(rep.Record.ReceiptID), RecordNo: no, KnownSeq: rep.KnownSeq,
		AreaID: uuid.UUID(rep.AreaID), ComponentID: nullID(rep.ComponentID), ReportKind: rep.Kind, Mode: rep.Mode,
		Producer: text(rep.Producer), ProducerVersion: text(rep.ProducerVersion), ReportFormat: rep.Format,
		SchemaVersion: rep.SchemaVersion, ParserVersion: rep.ParserVersion, ProfileID: text(rep.ProfileID),
		RulesHash: rep.RulesHash, ConfigHash: rep.ConfigHash, DbVersion: text(rep.DBVersion), DbPublishedAt: tstz(rep.DBPublishedAt),
		ScopeHash: rep.ScopeHash, Sha: text(rep.SHA), ArtifactDigest: text(rep.ArtifactDigest), RunID: nullID(rep.RunID),
		AttemptID: nullID(rep.AttemptID), StartedAt: tstz(rep.StartedAt), FinishedAt: tstz(rep.FinishedAt), ExitClass: rep.ExitClass,
		Completeness: rep.Completeness, SkippedTargets: skipped,
	}
	if p.ScannedCount, err = int4(rep.ScannedCount); err != nil {
		return err
	}
	if p.SkippedCount, err = int4(rep.SkippedCount); err != nil {
		return err
	}
	if h := rep.History; h != nil {
		p.HistoryFromSha, p.HistoryToSha, p.HistoryTruncated = text(h.FromSHA), text(h.ToSHA), pgtype.Bool{Bool: h.Truncated, Valid: true}
	}
	id, err := r.q(ctx).InsertReport(ctx, p)
	if err != nil {
		return fmt.Errorf("normalize: insert report: %w", err)
	}
	if len(findings) == 0 {
		return nil
	}
	f := InsertFindingsParams{
		OrganizationID: uuid.UUID(orgID), ReportID: id, KnownSeq: rep.KnownSeq,
		NativeIds: make([]string, len(findings)), RuleIds: make([]string, len(findings)), FindingKinds: make([]string, len(findings)),
		Paths: make([]string, len(findings)), LineStarts: make([]int32, len(findings)), LineEnds: make([]int32, len(findings)),
		PackagePurls: make([]string, len(findings)), NativeSeverities: make([]string, len(findings)),
		NativeConfidences: make([]string, len(findings)), Fingerprints: make([][]byte, len(findings)), Evidences: make([]string, len(findings)),
	}
	for i := range findings {
		o := &findings[i]
		if f.LineStarts[i], err = line(o.LineStart); err != nil {
			return err
		}
		if f.LineEnds[i], err = line(o.LineEnd); err != nil {
			return err
		}
		f.NativeIds[i], f.RuleIds[i], f.FindingKinds[i], f.Paths[i] = o.NativeID, o.RuleID, o.Kind, o.Path
		f.PackagePurls[i], f.NativeSeverities[i], f.NativeConfidences[i] = o.PackagePURL, o.NativeSeverity, o.NativeConfidence
		f.Fingerprints[i], f.Evidences[i] = o.Fingerprint, string(o.Evidence)
	}
	if err := r.q(ctx).InsertFindings(ctx, f); err != nil {
		return fmt.Errorf("normalize: insert findings: %w", err)
	}
	return nil
}

func int4(n *int) (pgtype.Int4, error) {
	if n == nil {
		return pgtype.Int4{}, nil
	}
	if *n < 0 || *n > math.MaxInt32 {
		return pgtype.Int4{}, fmt.Errorf("normalize: count %d does not fit its column", *n)
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}, nil
}

// line is a line of a finding, zero when it is unknown.
func line(n *int) (int32, error) {
	if n == nil {
		return 0, nil
	}
	if *n < 1 || *n > math.MaxInt32 {
		return 0, fmt.Errorf("normalize: line %d does not fit its column", *n)
	}
	return int32(*n), nil
}
