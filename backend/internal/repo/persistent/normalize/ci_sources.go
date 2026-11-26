package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// Look up the sources of the selected versions, including undated runs and attempts.
// A child attempt uses the repository of its own run.
const _gitLabCISources = `
WITH selected AS (
    SELECT v.run_id AS object_id, v.run_id, v.receipt_id, 'ci_run' AS kind
    FROM normalize.ci_run_version v
    WHERE v.organization_id = $1 AND v.id = ANY($4::uuid[])
    UNION ALL
    SELECT a.attempt_id, a.run_id, a.receipt_id, 'ci_attempt'
    FROM normalize.ci_attempt_version a
    WHERE a.organization_id = $1 AND a.id = ANY($4::uuid[])
)
SELECT o.id, o.object_kind, o.native_id, COALESCE(src.base_url, src.instance_key) AS instance_url,
       COALESCE(repo.web_url, '') AS repository_url, COALESCE(repo.path, '') AS repository_path,
       COALESCE(CASE WHEN rec.manifest #>> '{area,native_id}' = area.native_id
                     THEN rec.manifest #>> '{area,key}' END, '') AS manifest_path
FROM selected v
JOIN normalize.source_object run ON run.organization_id = $1
    AND run.id = v.run_id AND run.deleted_at IS NULL
JOIN normalize.source_object o ON o.organization_id = run.organization_id
    AND o.id = v.object_id AND o.object_kind = v.kind AND o.instance_id = run.instance_id AND o.deleted_at IS NULL
JOIN normalize.source_object area ON area.organization_id = run.organization_id
    AND area.id = run.current_area_id AND area.instance_id = run.instance_id AND area.deleted_at IS NULL
JOIN connect.source_instance src ON src.organization_id = run.organization_id
    AND src.id = run.instance_id AND src.provider = 'gitlab'
LEFT JOIN normalize.repository_version repo ON repo.organization_id = area.organization_id
    AND repo.repository_id = area.id AND repo.superseded_seq IS NULL
LEFT JOIN ingest.receipt rec ON rec.organization_id = run.organization_id
    AND rec.id = v.receipt_id AND rec.source_instance_id = run.instance_id
WHERE area.id = ANY($2::uuid[])
    AND (o.current_area_id IS NULL OR o.current_area_id = area.id)
    AND (NOT run.restricted OR run.id = ANY($3::uuid[]))
    AND (NOT o.restricted OR o.id = ANY($3::uuid[]))`

func (r *Repo) GitLabCISources(ctx context.Context, org entity.ID, areas, allowed, versions []entity.ID) ([]entity.GitLabCISource, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, _gitLabCISources, uuid.UUID(org), ids(areas), ids(allowed), ids(versions))
	if err != nil {
		return nil, fmt.Errorf("normalize: read GitLab CI sources: %w", err)
	}
	defer rows.Close()
	sources, err := pgx.CollectRows(rows, scanGitLabCISource)
	if err != nil {
		return nil, fmt.Errorf("normalize: scan GitLab CI sources: %w", err)
	}
	return sources, nil
}

func scanGitLabCISource(row pgx.CollectableRow) (entity.GitLabCISource, error) {
	var source entity.GitLabCISource
	var id uuid.UUID
	err := row.Scan(&id, &source.Kind, &source.NativeID, &source.InstanceURL, &source.RepositoryURL, &source.RepositoryPath, &source.ManifestPath)
	source.ObjectID = entity.ID(id)
	return source, err
}
