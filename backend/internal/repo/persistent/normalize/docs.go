package normalize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) insertDocsFacts(ctx context.Context, org entity.ID, report uuid.UUID, rep *entity.ScanReport, facts *entity.DocsReportFacts) error {
	if facts == nil {
		return nil
	}
	p := InsertDocsReportParams{
		OrganizationID: uuid.UUID(org), ReportID: report, RootID: nullID(facts.RootID), PathPrefix: facts.PathPrefix,
		OwnerRule: text(facts.OwnerRule), Checks: append([]string{}, facts.Checks...), SarifHash: facts.SARIFHash,
	}
	if facts.Links != nil {
		var err error
		p.Links, err = json.Marshal(facts.Links)
		if err != nil {
			return fmt.Errorf("normalize: encode link counts: %w", err)
		}
	}
	if err := r.q(ctx).InsertDocsReport(ctx, p); err != nil {
		return fmt.Errorf("normalize: insert documentation report: %w", err)
	}
	if len(facts.Documents) == 0 {
		return nil
	}
	data, err := json.Marshal(facts.Documents)
	if err != nil {
		return fmt.Errorf("normalize: encode document versions: %w", err)
	}
	n, err := r.q(ctx).InsertDocsReportDocuments(ctx, InsertDocsReportDocumentsParams{
		OrganizationID: uuid.UUID(org), ReportID: report, AreaID: uuid.UUID(rep.AreaID), KnownSeq: rep.KnownSeq, Documents: data,
	})
	if err != nil {
		return fmt.Errorf("normalize: insert document versions: %w", err)
	}
	if n != int64(len(facts.Documents)) {
		return fmt.Errorf("normalize: conflicting document version in report %s", report)
	}
	return nil
}

func (r *Repo) DocumentsAt(ctx context.Context, org, area entity.ID, prefix, sha string, seq int64) ([]entity.DocumentVersion, error) {
	rows, err := r.q(ctx).DocumentsAt(ctx, DocumentsAtParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), VersionRef: sha, KnownSeq: seq, PathPrefix: prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: list documents at commit: %w", err)
	}
	out := make([]entity.DocumentVersion, 0, len(rows))
	for i := range rows {
		v := &rows[i]
		out = append(out, entity.DocumentVersion{
			ID: entity.ID(v.ID), DocumentID: entity.ID(v.DocumentID), VersionRef: v.VersionRef, Path: v.Path, Format: v.DocFormat,
			Title: v.Title.String, ContentHash: v.ContentHash, Owner: v.Owner.String, ACLState: v.AclState,
		})
	}
	return out, nil
}

func (r *Repo) DocumentationReportsAt(ctx context.Context, org entity.ID, root *entity.DocumentationRoot, sha string, seq int64) ([]entity.DocsReport, []entity.DocsFinding, error) {
	rows, err := r.q(ctx).DocumentationReportsAt(ctx, DocumentationReportsAtParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(root.AreaID), Sha: text(sha), KnownSeq: seq,
		PathPrefix: root.PathPrefix, RootID: nullID(&root.ID), ProfileID: text(root.ProfileID),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("normalize: read documentation reports: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	reports := make([]entity.DocsReport, len(rows))
	byID := make(map[uuid.UUID]*entity.DocsReport, len(rows))
	var docIDs, sarifIDs []uuid.UUID
	for i := range rows {
		row := &rows[i]
		rep := &reports[i]
		*rep = entity.DocsReport{
			ID: entity.ID(row.ID), Kind: row.ReportKind, SARIFID: entity.ID(row.SarifID),
			Completeness: row.Completeness, Skipped: int(row.SkippedCount.Int32), ObservedAt: row.ObservedAt, KnownSeq: row.KnownSeq,
			DocsReportFacts: entity.DocsReportFacts{OwnerRule: row.OwnerRule.String, Checks: row.Checks},
		}
		if len(row.Links) != 0 {
			if err := json.Unmarshal(row.Links, &rep.Links); err != nil {
				return nil, nil, fmt.Errorf("normalize: decode link counts: %w", err)
			}
		}
		byID[row.ID], docIDs = rep, append(docIDs, row.ID)
		if !rep.SARIFID.IsZero() {
			sarifIDs = append(sarifIDs, row.SarifID)
		}
	}
	if err := r.loadReportDocuments(ctx, org, docIDs, byID); err != nil {
		return nil, nil, err
	}
	findings, err := r.docsFindingsAt(ctx, org, root, sarifIDs, seq)
	return reports, findings, err
}

func (r *Repo) loadReportDocuments(ctx context.Context, org entity.ID, ids []uuid.UUID, byID map[uuid.UUID]*entity.DocsReport) error {
	rows, err := r.q(ctx).DocumentationReportDocuments(ctx, DocumentationReportDocumentsParams{OrganizationID: uuid.UUID(org), ReportIds: ids})
	if err != nil {
		return fmt.Errorf("normalize: read documentation report inventory: %w", err)
	}
	for i := range rows {
		row := &rows[i]
		rec := entity.DocsReportDocument{
			DocumentVersion: entity.DocumentVersion{
				ID: entity.ID(row.ID), DocumentID: entity.ID(row.DocumentID), VersionRef: row.VersionRef, Path: row.Path, Format: row.DocFormat,
				Title: row.Title.String, ContentHash: row.ContentHash, Owner: row.Owner.String, ACLState: row.AclState,
			},
		}
		if row.SpecValid.Valid {
			rec.SpecValid = new(row.SpecValid.Bool)
		}
		rep := byID[row.ReportID]
		rep.Documents = append(rep.Documents, rec)
	}
	return nil
}

func (r *Repo) docsFindingsAt(ctx context.Context, org entity.ID, root *entity.DocumentationRoot, reports []uuid.UUID, seq int64) ([]entity.DocsFinding, error) {
	if len(reports) == 0 {
		return nil, nil
	}
	rows, err := r.q(ctx).DocumentationFindingsAt(ctx, DocumentationFindingsAtParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(root.AreaID), PathPrefix: root.PathPrefix, ReportIds: reports, KnownSeq: seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read documentation findings: %w", err)
	}
	out := make([]entity.DocsFinding, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		f := entity.DocsFinding{
			ID: entity.ID(row.ID), ReportID: entity.ID(row.ReportID), Rule: row.RuleID, Kind: row.FindingKind, Path: row.Path.String,
			Severity: row.Severity, FirstSeenAt: row.FirstSeenAt, Evidence: row.Evidence, HasLinkTarget: row.HasLinkTarget,
		}
		if row.LineStart.Valid {
			f.Line = new(int(row.LineStart.Int32))
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *Repo) ReceiptDocumentationRoots(ctx context.Context, org, receipt entity.ID) ([]entity.DocsReceiptRoot, error) {
	rows, err := r.q(ctx).ReceiptDocumentationRoots(ctx, ReceiptDocumentationRootsParams{
		OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(receipt),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read receipt documentation roots: %w", err)
	}
	out := make([]entity.DocsReceiptRoot, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		root := entity.DocsReceiptRoot{
			AreaID: entity.ID(row.AreaID), ProfileID: row.ProfileID.String, PathPrefix: row.PathPrefix, SHA: row.Sha.String,
		}
		if row.RootID.Valid {
			root.RootID = new(entity.ID(row.RootID.UUID))
		}
		out = append(out, root)
	}
	return out, nil
}
