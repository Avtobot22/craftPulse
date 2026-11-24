package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) ReadSecurityFindings(ctx context.Context, org entity.ID, reports []entity.ID) ([]entity.SecurityFinding, error) {
	rows, err := r.q(ctx).ReadSecurityFindings(ctx, ReadSecurityFindingsParams{OrganizationID: uuid.UUID(org), ReportIds: ids(reports)})
	if err != nil {
		return nil, fmt.Errorf("normalize: read security findings: %w", err)
	}
	out := make([]entity.SecurityFinding, len(rows))
	for i := range rows {
		row := &rows[i]
		f := &out[i]
		*f = entity.SecurityFinding{
			ID: entity.ID(row.ID), ReportID: entity.ID(row.ReportID),
			FindingOccurrence: entity.FindingOccurrence{
				RuleID: row.RuleID, Kind: row.FindingKind, Path: row.Path, PackagePURL: row.PackagePurl,
				NativeSeverity: row.NativeSeverity, NativeConfidence: row.NativeConfidence, MappedSeverity: row.MappedSeverity,
				SeverityMappingVersion: int(row.SeverityMappingVersion), Fingerprint: row.Fingerprint, Evidence: row.Evidence,
			},
		}
		if row.LineStart.Valid {
			n := int(row.LineStart.Int32)
			f.LineStart = &n
		}
		if row.LineEnd.Valid {
			n := int(row.LineEnd.Int32)
			f.LineEnd = &n
		}
	}
	return out, nil
}

func (r *Repo) ReceiptSecurityScopes(ctx context.Context, org, receipt entity.ID) ([]entity.SecurityScope, error) {
	rows, err := r.q(ctx).ReceiptSecurityScopes(ctx, ReceiptSecurityScopesParams{OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(receipt)})
	if err != nil {
		return nil, fmt.Errorf("normalize: read security scopes: %w", err)
	}
	out := make([]entity.SecurityScope, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.SecurityScope{AreaID: entity.ID(row.AreaID), ComponentID: idPtr(row.ComponentID), SHA: row.Sha}
	}
	return out, nil
}
