package normalize

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) SaveWikiPage(ctx context.Context, org, area entity.ID, seq int64, page entity.WikiPage) (bool, error) {
	conn := postgres.Conn(ctx, r.pool)
	var version uuid.UUID
	err := conn.QueryRow(ctx, _ensureWikiVersion, org, area, seq, page.DocumentID, page.PageNativeID, page.VersionRef,
		page.Title, page.ContentHash, page.ACLState, page.Restriction, page.SourceURL, page.UpdatedAt).Scan(&version)
	if err != nil {
		return false, fmt.Errorf("normalize: ensure wiki version: %w", err)
	}
	tag, err := conn.Exec(ctx, _observeWikiVersion, org, version, area, seq, page.AncestorIDs,
		page.Restriction, page.ACLState, page.SourceURL, page.ContentHash, page.Deleted, page.ObservedAt)
	if err != nil {
		return false, fmt.Errorf("normalize: record wiki observation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	_, err = conn.Exec(ctx, _placeWikiPage, org, page.DocumentID)
	if err != nil {
		return false, fmt.Errorf("normalize: set wiki page restriction: %w", err)
	}
	return true, nil
}

const _ensureWikiVersion = `
WITH inserted AS (
    INSERT INTO normalize.document_version
        (organization_id, area_id, known_seq, document_id, path, page_native_id, version_ref,
         title, content_hash, acl_state, restriction_native, source_url, source_updated_at, doc_format)
    VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9, $10, $11, $12, 'confluence')
    ON CONFLICT (organization_id, document_id, version_ref) DO NOTHING
    RETURNING id
)
SELECT id FROM inserted
UNION ALL
SELECT id FROM normalize.document_version WHERE organization_id = $1 AND document_id = $4 AND version_ref = $6
`

const _observeWikiVersion = `
WITH observation AS (
INSERT INTO normalize.wiki_page_observation AS o
    (organization_id, version_id, area_id, known_seq, ancestor_ids, restriction_native, acl_state, source_url, content_hash, deleted, observed_at)
SELECT $1::uuid, $2::uuid, $3::uuid, $4::bigint, $5::text[], $6::text, $7::text, COALESCE($8::text, prior.source_url),
    $9::bytea, $10::boolean, $11::timestamptz
FROM (SELECT 1) seed
LEFT JOIN LATERAL (
    SELECT * FROM normalize.wiki_page_observation
    WHERE organization_id = $1 AND version_id = $2
    ORDER BY known_seq DESC LIMIT 1
) prior ON true
WHERE prior.version_id IS NULL
    OR prior.observed_at < $11
    OR (prior.observed_at = $11 AND
        (prior.area_id, prior.ancestor_ids, prior.restriction_native, prior.acl_state, prior.source_url, prior.content_hash, prior.deleted)
        IS DISTINCT FROM ($3, $5, $6, $7, COALESCE($8, prior.source_url), $9, $10))
ON CONFLICT (organization_id, version_id, known_seq) DO UPDATE SET
    area_id = EXCLUDED.area_id, ancestor_ids = EXCLUDED.ancestor_ids, restriction_native = EXCLUDED.restriction_native,
    acl_state = EXCLUDED.acl_state, source_url = EXCLUDED.source_url, content_hash = EXCLUDED.content_hash, deleted = EXCLUDED.deleted,
    observed_at = EXCLUDED.observed_at
RETURNING organization_id, version_id, restriction_native, acl_state, source_url, content_hash
)
UPDATE normalize.document_version v
SET restriction_native = o.restriction_native, acl_state = o.acl_state, source_url = o.source_url, content_hash = COALESCE(o.content_hash, v.content_hash)
FROM observation o
WHERE (v.organization_id, v.id) = (o.organization_id, o.version_id)
`

const _placeWikiPage = `
UPDATE normalize.source_object obj
SET current_area_id = latest.area_id,
    restricted = latest.restriction_native IS NOT NULL OR latest.acl_state = 'unknown' OR latest.deleted
FROM (
    SELECT o.area_id, o.restriction_native, o.acl_state, o.deleted
    FROM normalize.document_version v
    JOIN normalize.wiki_page_observation o ON (o.organization_id, o.version_id) = (v.organization_id, v.id)
    WHERE v.organization_id = $1 AND v.document_id = $2 AND v.doc_format = 'confluence'
    ORDER BY length(v.version_ref) DESC, v.version_ref DESC, o.known_seq DESC LIMIT 1
) latest
WHERE obj.organization_id = $1 AND obj.id = $2
`

func (r *Repo) WikiDocumentsAt(ctx context.Context, org, area entity.ID, prefix string, seq int64) ([]entity.DocumentVersion, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, _wikiDocumentsAt, org, area, prefix, seq)
	if err != nil {
		return nil, fmt.Errorf("normalize: query wiki inventory: %w", err)
	}
	docs, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[entity.DocumentVersion])
	if err != nil {
		return nil, fmt.Errorf("normalize: scan wiki inventory: %w", err)
	}
	return docs, nil
}

const _wikiDocumentsAt = `
WITH candidates AS (
    SELECT DISTINCT v.document_id
    FROM normalize.wiki_page_observation o
    JOIN normalize.document_version v ON (v.organization_id, v.id) = (o.organization_id, o.version_id)
    WHERE o.organization_id = $1 AND o.area_id = $2 AND o.known_seq <= $4
)
SELECT latest.id, obj.id AS document_id, latest.path, latest.version_ref, latest.doc_format AS format,
    COALESCE(latest.title, '') AS title, latest.content_hash, latest.acl_state, latest.page_native_id, latest.source_url,
    (obj.restricted OR latest.restriction_native IS NOT NULL) AS restricted
FROM candidates c
JOIN normalize.source_object obj ON obj.organization_id = $1 AND obj.id = c.document_id
JOIN LATERAL (
    SELECT v.id, v.path, v.version_ref, v.doc_format, v.title, v.page_native_id,
        o.area_id, o.ancestor_ids, o.restriction_native, o.acl_state, o.source_url, o.content_hash, o.deleted
    FROM normalize.document_version v
    JOIN normalize.wiki_page_observation o ON (o.organization_id, o.version_id) = (v.organization_id, v.id)
    WHERE v.organization_id = $1 AND v.document_id = obj.id AND v.doc_format = 'confluence' AND o.known_seq <= $4
    ORDER BY length(v.version_ref) DESC, v.version_ref DESC, o.known_seq DESC LIMIT 1
) latest ON true
WHERE latest.area_id = $2 AND NOT latest.deleted AND latest.acl_state = 'known'
    AND ($3 = '' OR $3 = latest.page_native_id OR $3 = ANY(latest.ancestor_ids))
ORDER BY latest.page_native_id
`

func (r *Repo) DocumentationAreaKinds(ctx context.Context, org entity.ID, ids []entity.ID) (map[entity.ID]entity.ScopeKind, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
SELECT area_id, area_kind FROM normalize.area
WHERE organization_id = $1 AND area_id = ANY($2::uuid[]) AND area_kind IN ('repository', 'wiki_space')
`, org, ids)
	if err != nil {
		return nil, fmt.Errorf("normalize: query documentation areas: %w", err)
	}
	out := make(map[entity.ID]entity.ScopeKind)
	var id entity.ID
	var kind entity.ScopeKind
	_, err = pgx.ForEachRow(rows, []any{&id, &kind}, func() error {
		out[id] = kind
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: scan documentation areas: %w", err)
	}
	return out, nil
}

// CurrentWikiVersions returns only the current versions that were read with known permissions.
func (r *Repo) CurrentWikiVersions(ctx context.Context, org, area entity.ID, ids []string) (map[entity.ObjectKey]entity.SyncObjectVersion, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, _currentWikiVersions, org, area, ids)
	if err != nil {
		return nil, fmt.Errorf("normalize: query wiki versions for reconciliation: %w", err)
	}
	var nativeID, revision string
	var updated time.Time
	out := make(map[entity.ObjectKey]entity.SyncObjectVersion)
	_, err = pgx.ForEachRow(rows, []any{&nativeID, &revision, &updated}, func() error {
		out[entity.ObjectKey{ObjectKind: string(entity.ObjectDocument), NativeID: nativeID}] = entity.SyncObjectVersion{Revision: revision, UpdatedAt: new(updated)}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: scan wiki versions for reconciliation: %w", err)
	}
	return out, nil
}

const _currentWikiVersions = `
SELECT obj.native_id, latest.version_ref, latest.source_updated_at
FROM normalize.source_object obj
JOIN LATERAL (
    SELECT v.version_ref, v.source_updated_at, o.acl_state, o.content_hash, o.deleted
    FROM normalize.document_version v
    JOIN normalize.wiki_page_observation o ON (o.organization_id, o.version_id) = (v.organization_id, v.id)
    WHERE v.organization_id = obj.organization_id AND v.document_id = obj.id AND v.doc_format = 'confluence'
    ORDER BY length(v.version_ref) DESC, v.version_ref DESC, o.known_seq DESC LIMIT 1
) latest ON true
WHERE obj.organization_id = $1 AND obj.current_area_id = $2 AND obj.object_kind = 'document'
    AND obj.native_id = ANY($3::text[]) AND NOT latest.deleted AND latest.acl_state = 'known'
    AND latest.content_hash IS NOT NULL
`

func (r *Repo) WikiVersionBoundary(ctx context.Context, org, area entity.ID, seq int64, asOf time.Time) (*time.Time, error) {
	var at *time.Time
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `
SELECT min(source_updated_at) FROM normalize.document_version
WHERE organization_id = $1 AND area_id = $2 AND known_seq <= $3 AND doc_format = 'confluence'
    AND source_updated_at <= $4
`, org, area, seq, asOf).Scan(&at)
	if err != nil {
		return nil, fmt.Errorf("normalize: query wiki version boundary: %w", err)
	}
	return at, nil
}
