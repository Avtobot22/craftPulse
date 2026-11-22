package normalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) insertCodeFacts(ctx context.Context, org entity.ID, report uuid.UUID, facts *entity.CodeFacts) error {
	if facts == nil {
		return nil
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return fmt.Errorf("normalize: encode code facts: %w", err)
	}
	if err := r.q(ctx).InsertCodeFacts(ctx, InsertCodeFactsParams{
		OrganizationID: uuid.UUID(org), ReportID: report, Facts: data,
	}); err != nil {
		return fmt.Errorf("normalize: insert code facts: %w", err)
	}
	return nil
}

func (r *Repo) CodeReportAt(ctx context.Context, org entity.ID, scope entity.CodeScope, seq int64) (*entity.CodeReport, error) {
	var component *entity.ID
	if !scope.ComponentID.IsZero() {
		component = &scope.ComponentID
	}
	row, err := r.q(ctx).CodeReportAt(ctx, CodeReportAtParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(scope.RepositoryID), ComponentID: nullID(component),
		Sha: text(scope.SHA), Format: scope.Format, Producer: text(scope.Producer), ProfileID: text(scope.ProfileID), KnownSeq: seq,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("normalize: read code report: %w", err)
	}
	out := &entity.CodeReport{ID: entity.ID(row.ID), Completeness: row.Completeness, SkippedCount: int(row.SkippedCount.Int32)}
	if err := json.Unmarshal(row.Facts, &out.Facts); err != nil {
		return nil, fmt.Errorf("normalize: decode code facts: %w", err)
	}
	if err := json.Unmarshal(row.SkippedTargets, &out.Skipped); err != nil {
		return nil, fmt.Errorf("normalize: decode skipped code targets: %w", err)
	}
	return out, nil
}

func (r *Repo) ReceiptCodeScopes(ctx context.Context, org, receipt entity.ID) ([]entity.CodeScope, error) {
	rows, err := r.q(ctx).ReceiptCodeScopes(ctx, ReceiptCodeScopesParams{OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(receipt)})
	if err != nil {
		return nil, fmt.Errorf("normalize: read code scopes: %w", err)
	}
	out := make([]entity.CodeScope, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, entity.CodeScope{
			RepositoryID: entity.ID(row.AreaID), ComponentID: entity.ID(row.ComponentID.UUID), SHA: row.Sha.String,
			Format: row.ReportFormat, Producer: row.Producer.String, ProfileID: row.ProfileID.String,
		})
	}
	return out, nil
}
