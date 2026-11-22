// Package change keeps the versions of the matrix of checks of projects (TZ 12.5.7).
package change

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

func (r *Repo) ExpectedPipelineReports(ctx context.Context, org, area entity.ID, at time.Time, whole bool, projects []entity.ID) ([]entity.ExpectedReport, error) {
	projectIDs := make([]uuid.UUID, len(projects))
	for i, project := range projects {
		projectIDs[i] = uuid.UUID(project)
	}
	rows, err := r.q(ctx).PipelineExpectedReports(ctx, PipelineExpectedReportsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), RunAt: at, Whole: whole, ProjectIds: projectIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("change: read pipeline expectations: %w", err)
	}
	out := make([]entity.ExpectedReport, len(rows))
	for i, row := range rows {
		out[i] = entity.ExpectedReport{ComponentID: entity.ID(row.ComponentID), Kind: row.AnalysisKind, Language: row.Language}
	}
	return out, nil
}

// LatestProfileVersion is the last version of the matrix of a project with its rows; entity.ErrNotFound without one.
func (r *Repo) LatestProfileVersion(ctx context.Context, orgID, projectID entity.ID) (entity.CheckProfileVersion, error) {
	row, err := r.q(ctx).LatestProfileVersion(ctx, LatestProfileVersionParams{OrganizationID: uuid.UUID(orgID), ProjectID: uuid.UUID(projectID)})
	return r.withEntries(ctx, row, err)
}

func (r *Repo) ProfileVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.CheckProfileVersion, error) {
	row, err := r.q(ctx).ProfileVersionByConfirmation(ctx, ProfileVersionByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(confirmationID),
	})
	return r.withEntries(ctx, row, err)
}

func (r *Repo) withEntries(ctx context.Context, row ChangeProfileVersion, err error) (entity.CheckProfileVersion, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.CheckProfileVersion{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.CheckProfileVersion{}, fmt.Errorf("change: read profile version: %w", err)
	}
	v := entity.CheckProfileVersion{
		ID: entity.ID(row.ID), ProjectID: entity.ID(row.ProjectID), VersionNo: int(row.VersionNo), ContentHash: row.ContentHash, KnownSeq: row.KnownSeq,
		Confirmation: entity.Confirmation{ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID)},
	}
	if row.BaseVersionNo.Valid {
		base := int(row.BaseVersionNo.Int32)
		v.BaseVersionNo = &base
	}
	rows, err := r.q(ctx).ProfileEntries(ctx, ProfileEntriesParams{OrganizationID: row.OrganizationID, ProfileVersionID: row.ID})
	if err != nil {
		return entity.CheckProfileVersion{}, fmt.Errorf("change: read profile entries: %w", err)
	}
	v.Entries = make([]entity.ProfileEntry, len(rows))
	for i := range rows {
		e := &rows[i]
		v.Entries[i] = entity.ProfileEntry{
			ComponentID: entity.ID(e.ComponentID), Language: e.Language, AnalysisKind: e.AnalysisKind, Mode: e.Mode, ProfileID: e.ProfileID,
			RulesHash: e.RulesHash,
		}
		if err := json.Unmarshal(e.ToolVersions, &v.Entries[i].ToolVersions); err != nil {
			return entity.CheckProfileVersion{}, fmt.Errorf("change: tool versions of a profile entry: %w", err)
		}
	}
	return v, nil
}

func (r *Repo) InsertProfileVersion(ctx context.Context, orgID entity.ID, v entity.CheckProfileVersion) error {
	if v.VersionNo < 1 || v.VersionNo > math.MaxInt32 {
		return fmt.Errorf("change: version number %d is out of range", v.VersionNo)
	}
	p := InsertProfileVersionParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(v.ID), ProjectID: uuid.UUID(v.ProjectID), VersionNo: int32(v.VersionNo),
		ContentHash: v.ContentHash, KnownSeq: v.KnownSeq, ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt,
		ConfirmationID: uuid.UUID(v.ConfirmationID),
	}
	if v.BaseVersionNo != nil {
		p.BaseVersionNo = pgtype.Int4{Int32: int32(*v.BaseVersionNo), Valid: true} //nolint:gosec // the base is one below a checked version number
	}
	q := r.q(ctx)
	if err := q.InsertProfileVersion(ctx, p); err != nil {
		return fmt.Errorf("change: insert profile version: %w", err)
	}
	params := make([]InsertProfileEntriesParams, len(v.Entries))
	for i := range v.Entries {
		e := &v.Entries[i]
		tools, err := json.Marshal(e.ToolVersions)
		if err != nil {
			return fmt.Errorf("change: encode tool versions: %w", err)
		}
		params[i] = InsertProfileEntriesParams{
			OrganizationID: uuid.UUID(orgID), ProfileVersionID: uuid.UUID(v.ID), ComponentID: uuid.UUID(e.ComponentID), Language: e.Language,
			AnalysisKind: e.AnalysisKind, Mode: e.Mode, ProfileID: e.ProfileID, ToolVersions: tools, RulesHash: e.RulesHash,
		}
	}
	var batchErr error
	q.InsertProfileEntries(ctx, params).Exec(func(_ int, err error) {
		if batchErr == nil {
			batchErr = err
		}
	})
	if batchErr != nil {
		return fmt.Errorf("change: insert profile entries: %w", batchErr)
	}
	return nil
}
