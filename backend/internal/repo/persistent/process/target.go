package process

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) LatestTargetVersionNo(ctx context.Context, orgID, teamID entity.ID, g entity.TargetGroup) (int, error) {
	no, err := r.q(ctx).LatestTargetVersionNo(ctx, LatestTargetVersionNoParams{
		OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), TaskType: g.TaskType, SizeLabel: g.SizeLabel,
	})
	if err != nil {
		return 0, fmt.Errorf("process: latest target version: %w", err)
	}
	return int(no), nil
}

func (r *Repo) CurrentTargetVersion(ctx context.Context, orgID, teamID entity.ID, g entity.TargetGroup) (entity.TargetVersion, error) {
	row, err := r.q(ctx).CurrentTargetVersion(ctx, CurrentTargetVersionParams{
		OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), TaskType: g.TaskType, SizeLabel: g.SizeLabel,
	})
	if err != nil {
		return entity.TargetVersion{}, notFound("current target version", err)
	}
	return targetVersion(&row), nil
}

func (r *Repo) TargetVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.TargetVersion, error) {
	row, err := r.q(ctx).TargetVersionByConfirmation(ctx, TargetVersionByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(confirmationID),
	})
	if err != nil {
		return entity.TargetVersion{}, notFound("target version by confirmation", err)
	}
	return targetVersion((*CurrentTargetVersionRow)(&row)), nil
}

// PageTargetVersions reads up to Limit+1 versions newest first by their knowledge number.
func (r *Repo) PageTargetVersions(ctx context.Context, orgID, teamID entity.ID, f entity.TargetVersionFilter) ([]entity.TargetVersion, error) {
	if f.Limit < 1 || f.Limit >= math.MaxInt32 {
		return nil, errOutOfRange
	}
	params := PageTargetVersionsParams{
		OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), TaskType: text(f.TaskType), SizeLabel: text(f.SizeLabel),
		RowLimit: int32(f.Limit + 1),
	}
	if f.Before != nil {
		params.Before = pgtype.Int8{Int64: *f.Before, Valid: true}
	}
	rows, err := r.q(ctx).PageTargetVersions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("process: page target versions: %w", err)
	}
	out := make([]entity.TargetVersion, len(rows))
	for i := range rows {
		out[i] = targetVersion((*CurrentTargetVersionRow)(&rows[i]))
	}
	return out, nil
}

// TargetVersionsAt is the last version of every group of a team with known_seq not above seq.
func (r *Repo) TargetVersionsAt(ctx context.Context, orgID, teamID entity.ID, seq int64) ([]entity.TargetVersion, error) {
	rows, err := r.q(ctx).TargetVersionsAt(ctx, TargetVersionsAtParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), KnownSeq: seq})
	if err != nil {
		return nil, fmt.Errorf("process: target versions at: %w", err)
	}
	out := make([]entity.TargetVersion, len(rows))
	for i := range rows {
		out[i] = targetVersion((*CurrentTargetVersionRow)(&rows[i]))
	}
	return out, nil
}

func (r *Repo) InsertTargetVersion(ctx context.Context, orgID entity.ID, v entity.TargetVersion) error {
	no, base, err := versionNumbers(v.VersionNo, v.BaseVersionNo)
	if err != nil {
		return err
	}
	params := InsertTargetVersionParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(v.ID), TeamID: uuid.UUID(v.TeamID), TaskType: v.TaskType, SizeLabel: v.SizeLabel,
		VersionNo: no, BaseVersionNo: base, DurationS: v.DurationSeconds, Basis: v.Basis, KnownSeq: v.KnownSeq,
		ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt, ConfirmationID: uuid.UUID(v.ConfirmationID),
	}
	if b := v.BasisOf; b != nil {
		if b.N < 1 || b.N > math.MaxInt32 {
			return errOutOfRange
		}
		params.BasisRevisionID = uuid.NullUUID{UUID: uuid.UUID(b.RevisionID), Valid: true}
		params.BasisValueS = pgtype.Int8{Int64: b.ValueSeconds, Valid: true}
		params.BasisN = pgtype.Int4{Int32: int32(b.N), Valid: true}
		params.BasisFrom = pgtype.Timestamptz{Time: b.Window.From, Valid: true}
		params.BasisTo = pgtype.Timestamptz{Time: b.Window.To, Valid: true}
	}
	if err := r.q(ctx).InsertTargetVersion(ctx, params); err != nil {
		return fmt.Errorf("process: insert target version: %w", err)
	}
	return nil
}

func targetVersion(row *CurrentTargetVersionRow) entity.TargetVersion {
	v := entity.TargetVersion{
		ID: entity.ID(row.ID), TeamID: entity.ID(row.TeamID), TargetGroup: entity.TargetGroup{TaskType: row.TaskType, SizeLabel: row.SizeLabel},
		VersionNo: int(row.VersionNo), BaseVersionNo: intPtr(row.BaseVersionNo), DurationSeconds: row.DurationS, Basis: row.Basis,
		KnownSeq: row.KnownSeq,
		Confirmation: entity.Confirmation{
			ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt.UTC(), ConfirmationID: entity.ID(row.ConfirmationID),
		},
	}
	if row.BasisRevisionID.Valid {
		v.BasisOf = &entity.TargetBasis{
			RevisionID: entity.ID(row.BasisRevisionID.UUID), ValueSeconds: row.BasisValueS.Int64, N: int(row.BasisN.Int32),
			Window: entity.Window{From: row.BasisWindow.Lower.Time.UTC(), To: row.BasisWindow.Upper.Time.UTC()},
		}
	}
	return v
}

func text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
