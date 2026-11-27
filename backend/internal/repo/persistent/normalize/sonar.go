package normalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) SonarAnalysisExists(ctx context.Context, org, area entity.ID, analysis string) (bool, error) {
	var exists bool
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM normalize.report
		WHERE organization_id = $1 AND area_id = $2 AND sonar_analysis_id = $3)`,
		uuid.UUID(org), uuid.UUID(area), analysis).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("normalize: find Sonar analysis: %w", err)
	}
	return exists, nil
}

func (r *Repo) SonarMeasures(ctx context.Context, org, report entity.ID) (map[string]string, error) {
	var raw []byte
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT sonar_measures FROM normalize.report
		WHERE organization_id = $1 AND id = $2 AND sonar_analysis_id IS NOT NULL`, uuid.UUID(org), uuid.UUID(report)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("normalize: read Sonar measures: %w", err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("normalize: decode Sonar measures: %w", err)
	}
	return out, nil
}

func (r *Repo) setSonarReport(ctx context.Context, org entity.ID, id uuid.UUID, rep *entity.ScanReport) error {
	measures, err := json.Marshal(rep.SonarMeasures)
	if err != nil {
		return fmt.Errorf("normalize: encode Sonar measures: %w", err)
	}
	_, err = postgres.Conn(ctx, r.pool).Exec(ctx, `UPDATE normalize.report
		SET repository_id = $3, sonar_analysis_id = $4, sonar_measures = $5::jsonb
		WHERE organization_id = $1 AND id = $2`, uuid.UUID(org), id, nullID(rep.RepositoryID), rep.SonarAnalysisID, measures)
	if err != nil {
		return fmt.Errorf("normalize: persist Sonar metadata: %w", err)
	}
	return nil
}
