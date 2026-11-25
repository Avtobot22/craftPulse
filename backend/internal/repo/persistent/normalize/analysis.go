package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) InsertAnalysisRuns(ctx context.Context, org entity.ID, runs []entity.AnalysisRun) error {
	q := r.q(ctx)
	for i := range runs {
		run := &runs[i]
		metadata, err := json.Marshal(run.AnalysisRunMetadata)
		if err != nil {
			return fmt.Errorf("normalize: encode analysis run: %w", err)
		}
		nos := make([]int32, len(run.RecordNos))
		for j, no := range run.RecordNos {
			if no == 0 {
				continue
			}
			if nos[j], err = recordNo(no); err != nil {
				return err
			}
		}
		if err := q.InsertAnalysisRun(ctx, InsertAnalysisRunParams{
			OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(run.ReceiptID), AreaID: uuid.UUID(run.RepositoryID),
			KnownSeq: run.KnownSeq, ComponentID: analysisComponent(run.ComponentID), Sha: run.SHA,
			ToolID: run.ToolID, RecordNos: nos, Metadata: metadata,
		}); err != nil {
			return fmt.Errorf("normalize: insert analysis run: %w", err)
		}
	}
	return nil
}

func (r *Repo) ReceiptAnalysisScopes(ctx context.Context, org, receipt entity.ID) ([]entity.AnalysisScope, error) {
	rows, err := r.q(ctx).ReceiptAnalysisScopes(ctx, ReceiptAnalysisScopesParams{OrganizationID: uuid.UUID(org), ReceiptID: uuid.UUID(receipt)})
	if err != nil {
		return nil, fmt.Errorf("normalize: read receipt analysis scopes: %w", err)
	}
	out := make([]entity.AnalysisScope, len(rows))
	for i, row := range rows {
		out[i] = entity.AnalysisScope{RepositoryID: entity.ID(row.AreaID), ComponentID: entity.ID(row.ComponentID.UUID), SHA: row.Sha}
	}
	return out, nil
}

func (r *Repo) ReadAnalysisFacts(ctx context.Context, org entity.ID, scope entity.AnalysisScope, seq int64, asOf time.Time) (entity.AnalysisFacts, error) {
	q := r.q(ctx)
	rows, err := q.ReadAnalysisRuns(ctx, ReadAnalysisRunsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(scope.RepositoryID),
		ComponentID: analysisComponent(scope.ComponentID), Sha: scope.SHA, KnownSeq: seq,
	})
	if err != nil {
		return entity.AnalysisFacts{}, fmt.Errorf("normalize: read analysis runs: %w", err)
	}
	out := entity.AnalysisFacts{Runs: make([]entity.AnalysisRun, 0, len(rows))}
	var hasDocs bool
	for i := range rows {
		row := &rows[i]
		run := entity.AnalysisRun{ID: entity.ID(row.ID), ReceiptID: entity.ID(row.ReceiptID), KnownSeq: row.KnownSeq, ToolID: row.ToolID, AnalysisScope: scope}
		if err := json.Unmarshal(row.Metadata, &run.AnalysisRunMetadata); err != nil {
			return out, fmt.Errorf("normalize: decode analysis metadata: %w", err)
		}
		if run.FinishedAt != nil && run.FinishedAt.After(asOf) {
			continue
		}
		if err := json.Unmarshal(row.Reports, &run.Reports); err != nil {
			return out, fmt.Errorf("normalize: decode analysis report references: %w", err)
		}
		hasDocs = hasDocs || run.Kind == entity.AnalysisDocs || run.Kind == entity.AnalysisOpenAPI
		out.Runs = append(out.Runs, run)
	}
	if !hasDocs {
		return out, nil
	}
	versions, err := q.ReadAnalysisDocumentVersions(ctx, ReadAnalysisDocumentVersionsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(scope.RepositoryID),
		ComponentID: analysisComponent(scope.ComponentID), Sha: text(scope.SHA), KnownSeq: seq, AsOf: tstz(&asOf),
	})
	if err != nil {
		return out, fmt.Errorf("normalize: read analysis document versions: %w", err)
	}
	for _, row := range versions {
		out.Versions = append(out.Versions, entity.AnalysisDocumentVersion{
			ID: entity.ID(row.ID), ReportID: entity.ID(row.ReportID), Path: row.Path, VersionRef: row.VersionRef, ObservedAt: row.ObservedAt.Time,
		})
	}
	return out, nil
}

func analysisComponent(id entity.ID) uuid.NullUUID {
	return uuid.NullUUID{UUID: uuid.UUID(id), Valid: !id.IsZero()}
}
