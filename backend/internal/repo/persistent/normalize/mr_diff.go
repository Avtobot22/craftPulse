package normalize

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type diffRow struct {
	MRID         entity.ID           `json:"mr_id"`
	Revision     string              `json:"revision_key"`
	Head         string              `json:"head_sha"`
	Base         string              `json:"base_sha"`
	Record       int                 `json:"record_no"`
	Hash         string              `json:"content_hash"`
	Completeness string              `json:"completeness"`
	Files        []entity.MRDiffFile `json:"files"`
}

func (r *Repo) InsertMRDiffs(ctx context.Context, org, area, receipt entity.ID, seq int64, diffs []entity.MRDiffFact) (int64, error) {
	rows := make([]diffRow, len(diffs))
	for i := range diffs {
		d := &diffs[i]
		rows[i] = diffRow{
			MRID: d.MRID, Revision: d.RevisionKey, Head: d.HeadSHA, Base: d.BaseSHA, Record: d.Record.RecordNo,
			Hash: hex.EncodeToString(d.ContentHash), Completeness: d.Completeness, Files: d.Files,
		}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return 0, fmt.Errorf("normalize: encode MR diffs: %w", err)
	}
	count, err := r.q(ctx).InsertMRDiffs(ctx, InsertMRDiffsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.NullUUID{UUID: uuid.UUID(area), Valid: true},
		ReceiptID: uuid.UUID(receipt), KnownSeq: seq, Diffs: raw,
	})
	if err != nil {
		return 0, fmt.Errorf("normalize: insert MR diffs: %w", err)
	}
	return count, nil
}

func (r *Repo) ReadMRDiff(ctx context.Context, org, mr entity.ID, areas, allowed []entity.ID, head string, seq int64) (entity.MRDiff, error) {
	row, err := r.q(ctx).ReadMRDiff(ctx, ReadMRDiffParams{
		OrganizationID: uuid.UUID(org), MrID: uuid.UUID(mr), Areas: ids(areas), Allowed: ids(allowed), HeadSha: head, KnownSeq: seq,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return entity.MRDiff{}, entity.ErrNotFound
	case err != nil:
		return entity.MRDiff{}, fmt.Errorf("normalize: read MR diff: %w", err)
	}
	out := entity.MRDiff{
		MRID: entity.ID(row.MrID), RevisionID: entity.ID(row.RevisionID), RevisionKey: row.RevisionKey,
		HeadSHA: row.HeadSha, BaseSHA: row.BaseSha.String, KnownSeq: row.KnownSeq, Completeness: row.Completeness,
	}
	if err := json.Unmarshal(row.Files, &out.Files); err != nil {
		return entity.MRDiff{}, fmt.Errorf("normalize: decode MR diff files: %w", err)
	}
	return out, nil
}
