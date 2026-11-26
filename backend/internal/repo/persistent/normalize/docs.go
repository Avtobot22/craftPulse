package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
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

// LatestDocumentationChecks selects the latest observation of each kind and
// retains reports until a comparable check completes in the same root/profile/branch.
func (r *Repo) LatestDocumentationChecks(ctx context.Context, org entity.ID, root entity.DocumentationRoot, through, after int64) ([]entity.DocumentationCheck, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, _latestDocumentationChecks, uuid.UUID(org), uuid.UUID(root.AreaID),
		root.ProfileID, root.PathPrefix, uuid.UUID(root.ID), through, after)
	if err != nil {
		return nil, fmt.Errorf("normalize: read current documentation checks: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[entity.DocumentationCheck])
	if err != nil {
		return nil, fmt.Errorf("normalize: scan current documentation checks: %w", err)
	}
	return out, nil
}

const _documentationChecks = `
WITH scoped AS (
    SELECT r.id, r.report_kind, r.sha, COALESCE(r.ref_name, '')::text AS ref_name,
        r.known_seq, d.checks, r.run_id, r.receipt_id, r.rules_hash, r.config_hash,
        COALESCE(rv.web_url, '') AS repository_url,
        COALESCE(rc.manifest #>> '{source,instance_key}', '') AS instance_key,
        COALESCE(rc.manifest #>> '{area,key}', '') AS project_path,
        COALESCE(rc.manifest #>> '{source,provider}', '') AS provider,
        COALESCE(r.finished_at, r.started_at, rc.received_at)::timestamptz AS observed_at,
        COALESCE(s.id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS sarif_id,
        (s.id IS NOT NULL AND r.completeness = 'complete' AND COALESCE(r.skipped_count, 0) = 0
            AND r.exit_class IN ('success', 'findings') AND r.finished_at IS NOT NULL
            AND s.completeness = 'complete' AND s.exit_class IN ('success', 'findings')
            AND a.outcome IS DISTINCT FROM 'canceled')::boolean AS complete
    FROM normalize.report r
    JOIN normalize.docs_report d ON d.organization_id = r.organization_id AND d.report_id = r.id
    JOIN ingest.receipt rc ON rc.organization_id = r.organization_id AND rc.id = r.receipt_id
    LEFT JOIN normalize.repository_version rv ON rv.organization_id = r.organization_id
        AND rv.repository_id = r.area_id AND rv.superseded_seq IS NULL
    LEFT JOIN normalize.ci_attempt_version a ON a.organization_id = r.organization_id
        AND a.attempt_id = r.attempt_id AND a.superseded_seq IS NULL
    LEFT JOIN LATERAL (
        SELECT sarif.id, sarif.completeness, sarif.exit_class FROM normalize.report sarif
        WHERE sarif.organization_id = r.organization_id AND sarif.area_id = r.area_id
            AND sarif.sha = r.sha AND sarif.profile_id = r.profile_id AND sarif.report_kind = r.report_kind
            AND sarif.docs_run_id = r.docs_run_id AND sarif.docs_file_hash = d.sarif_hash
            AND sarif.report_format = 'sarif' AND sarif.known_seq <= $6::bigint
        ORDER BY (sarif.receipt_id = r.receipt_id) DESC, sarif.known_seq DESC, sarif.id DESC
        LIMIT 1
    ) s ON true
    WHERE r.organization_id = $1 AND r.area_id = $2
        AND r.sha IS NOT NULL AND r.profile_id = $3
        AND r.report_kind IN ('docs', 'openapi')
        AND r.known_seq > $7::bigint AND r.known_seq <= $6::bigint
        AND (NULLIF(rv.default_branch, '') IS NULL OR rv.default_branch = r.ref_name)
        AND d.path_prefix = $4 AND (d.root_id = $5 OR d.root_id IS NULL)
)`

const _latestDocumentationChecks = _documentationChecks + `, stream AS (
    SELECT * FROM scoped
    WHERE ref_name = (SELECT ref_name FROM scoped ORDER BY observed_at DESC, known_seq DESC, id DESC LIMIT 1)
), ranked AS (
    SELECT *,
        row_number() OVER (PARTITION BY report_kind ORDER BY observed_at DESC, known_seq DESC, id DESC) AS kind_rank,
        -- Missing provenance cannot establish comparability, even against another report with missing hashes.
        count(*) FILTER (WHERE complete AND rules_hash IS NOT NULL AND config_hash IS NOT NULL) OVER (
            PARTITION BY report_kind, rules_hash, config_hash
            ORDER BY observed_at DESC, known_seq DESC, id DESC
            ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
        ) AS newer_comparable_checks
    FROM stream
)
SELECT sha, report_kind, checks, observed_at, sarif_id, complete, kind_rank = 1 AS latest,
    repository_url, instance_key, project_path, provider
FROM ranked
WHERE newer_comparable_checks = 0
ORDER BY observed_at DESC, known_seq DESC, id DESC;
`

// DocumentationCheckCompleted finds a new pipeline that finished every check in the requested scope.
func (r *Repo) DocumentationCheckCompleted(ctx context.Context, org entity.ID, root entity.DocumentationRoot, through, after int64, requestedAt time.Time) (bool, error) {
	var complete bool
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, _documentationCheckCompleted, uuid.UUID(org), uuid.UUID(root.AreaID),
		root.ProfileID, root.PathPrefix, uuid.UUID(root.ID), through, after, requestedAt).Scan(&complete)
	if err != nil {
		return false, fmt.Errorf("normalize: read documentation check completion: %w", err)
	}
	return complete, nil
}

const _documentationCheckCompleted = _documentationChecks + `
SELECT EXISTS (
    SELECT 1 FROM scoped
    WHERE complete AND observed_at >= $8
    GROUP BY COALESCE(run_id, receipt_id), sha, ref_name,
        CASE WHEN cardinality(checks) = 0 THEN ARRAY['docs', 'openapi']::text[] ELSE checks END
    HAVING array_agg(DISTINCT report_kind) @>
        CASE WHEN cardinality(checks) = 0 THEN ARRAY['docs', 'openapi']::text[] ELSE checks END
);`
