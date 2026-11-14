package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// LockStructureVersion holds the organization row: concurrent confirmations of the structure run one by one.
func (r *Repo) LockStructureVersion(ctx context.Context, orgID entity.ID) (int64, error) {
	v, err := New(postgres.Conn(ctx, r.pool)).LockStructureVersion(ctx, uuid.UUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, entity.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("org: lock structure version: %w", err)
	}
	return v, nil
}

func (r *Repo) StructureVersion(ctx context.Context, orgID entity.ID) (int64, error) {
	v, err := New(postgres.Conn(ctx, r.pool)).GetStructureVersion(ctx, uuid.UUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, entity.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("org: get structure version: %w", err)
	}
	return v, nil
}

func (r *Repo) SetStructureVersion(ctx context.Context, orgID entity.ID, version int64) error {
	err := New(postgres.Conn(ctx, r.pool)).SetStructureVersion(ctx, SetStructureVersionParams{
		OrganizationID: uuid.UUID(orgID), StructureVersion: version,
	})
	if err != nil {
		return fmt.Errorf("org: set structure version: %w", err)
	}
	return nil
}

func (r *Repo) ExistingPersons(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ID, error) {
	found, err := New(postgres.Conn(ctx, r.pool)).ListPersonIDs(ctx, ListPersonIDsParams{
		OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(ids),
	})
	if err != nil {
		return nil, fmt.Errorf("org: list persons: %w", err)
	}
	return fromUUIDs(found), nil
}

func (r *Repo) AllAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind) ([]entity.Area, error) {
	q := New(postgres.Conn(ctx, r.pool))
	org := uuid.UUID(orgID)
	var (
		areas []entity.Area
		err   error
	)
	switch kind {
	case entity.AreaTeam:
		var rows []ListAllTeamsRow
		rows, err = q.ListAllTeams(ctx, org)
		areas = convert(rows, func(x ListAllTeamsRow) entity.Area { return area(x.ID, x.Name, x.Status) })
	case entity.AreaProject:
		var rows []ListAllProjectsRow
		rows, err = q.ListAllProjects(ctx, org)
		areas = convert(rows, func(x ListAllProjectsRow) entity.Area { return area(x.ID, x.Name, x.Status) })
	case entity.AreaService:
		var rows []ListAllServicesRow
		rows, err = q.ListAllServices(ctx, org)
		areas = convert(rows, func(x ListAllServicesRow) entity.Area { return area(x.ID, x.Name, x.Status) })
	default:
		return nil, fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("org: list %ss: %w", kind, err)
	}
	return areas, nil
}

func (r *Repo) CreateArea(ctx context.Context, orgID entity.ID, kind entity.AreaKind, a entity.Area, at time.Time) error {
	q := New(postgres.Conn(ctx, r.pool))
	org, id := uuid.UUID(orgID), uuid.UUID(a.ID)
	var err error
	switch kind {
	case entity.AreaTeam:
		err = q.CreateTeam(ctx, CreateTeamParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status, CreatedAt: at})
	case entity.AreaProject:
		err = q.CreateProject(ctx, CreateProjectParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status, CreatedAt: at})
	case entity.AreaService:
		err = q.CreateService(ctx, CreateServiceParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status, CreatedAt: at})
	default:
		return fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("org: create %s: %w", kind, err)
	}
	return nil
}

func (r *Repo) UpdateArea(ctx context.Context, orgID entity.ID, kind entity.AreaKind, a entity.Area) error {
	q := New(postgres.Conn(ctx, r.pool))
	org, id := uuid.UUID(orgID), uuid.UUID(a.ID)
	var err error
	switch kind {
	case entity.AreaTeam:
		err = q.UpdateTeam(ctx, UpdateTeamParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status})
	case entity.AreaProject:
		err = q.UpdateProject(ctx, UpdateProjectParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status})
	case entity.AreaService:
		err = q.UpdateService(ctx, UpdateServiceParams{OrganizationID: org, ID: id, Name: a.Name, Status: a.Status})
	default:
		return fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("org: update %s: %w", kind, err)
	}
	return nil
}

// CurrentLinks returns the versions in force at the moment: not superseded and valid at it.
func (r *Repo) CurrentLinks(ctx context.Context, orgID entity.ID, kind entity.LinkKind, at time.Time) ([]entity.Link, error) {
	q := New(postgres.Conn(ctx, r.pool))
	org := uuid.UUID(orgID)
	var (
		rows []ListCurrentTeamProjectsRow
		err  error
	)
	switch kind {
	case entity.LinkTeamProject:
		rows, err = q.ListCurrentTeamProjects(ctx, ListCurrentTeamProjectsParams{OrganizationID: org, At: at})
	case entity.LinkTeamMember:
		var mr []ListCurrentTeamMembersRow
		mr, err = q.ListCurrentTeamMembers(ctx, ListCurrentTeamMembersParams{OrganizationID: org, At: at})
		rows = convert(mr, func(x ListCurrentTeamMembersRow) ListCurrentTeamProjectsRow { return ListCurrentTeamProjectsRow(x) })
	case entity.LinkProjectArea:
		var pr []ListCurrentProjectAreasRow
		pr, err = q.ListCurrentProjectAreas(ctx, ListCurrentProjectAreasParams{OrganizationID: org, At: at})
		rows = convert(pr, func(x ListCurrentProjectAreasRow) ListCurrentTeamProjectsRow {
			return ListCurrentTeamProjectsRow{ID: x.ID, TeamID: x.ProjectID, ToID: x.ToID, ValidFrom: x.ValidFrom, ValidTo: x.ValidTo, Open: x.Open}
		})
	default:
		return nil, fmt.Errorf("org: unknown link kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("org: list %s links: %w", kind, err)
	}
	return convert(rows, func(x ListCurrentTeamProjectsRow) entity.Link {
		l := entity.Link{RowID: entity.ID(x.ID), From: entity.ID(x.TeamID), To: entity.ID(x.ToID), ValidFrom: x.ValidFrom}
		if !x.Open {
			l.ValidTo = x.ValidTo
		}
		return l
	}), nil
}

func (r *Repo) SupersedeLink(ctx context.Context, orgID entity.ID, kind entity.LinkKind, rowID entity.ID, seq int64, at time.Time) error {
	q := New(postgres.Conn(ctx, r.pool))
	var err error
	switch kind {
	case entity.LinkTeamProject:
		err = q.SupersedeTeamProject(ctx, SupersedeTeamProjectParams{Seq: seq, At: at, OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(rowID)})
	case entity.LinkTeamMember:
		err = q.SupersedeTeamMember(ctx, SupersedeTeamMemberParams{Seq: seq, At: at, OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(rowID)})
	case entity.LinkProjectArea:
		err = q.SupersedeProjectArea(ctx, SupersedeProjectAreaParams{Seq: seq, At: at, OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(rowID)})
	default:
		return fmt.Errorf("org: unknown link kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("org: supersede %s link: %w", kind, err)
	}
	return nil
}

// InsertLink adds a version known since seq; an interval overlapping a current one gives entity.ErrIntervalOverlap.
func (r *Repo) InsertLink(ctx context.Context, orgID entity.ID, kind entity.LinkKind, l entity.Link, seq int64) error {
	q := New(postgres.Conn(ctx, r.pool))
	org, from, to := uuid.UUID(orgID), uuid.UUID(l.From), uuid.UUID(l.To)
	validTo := pgtype.Timestamptz{Time: l.ValidTo, Valid: !l.ValidTo.IsZero()}
	var err error
	switch kind {
	case entity.LinkTeamProject:
		err = q.InsertTeamProject(ctx, InsertTeamProjectParams{
			OrganizationID: org, TeamID: from, ToID: to, ValidFrom: l.ValidFrom, ValidTo: validTo, Seq: seq,
		})
	case entity.LinkTeamMember:
		err = q.InsertTeamMember(ctx, InsertTeamMemberParams{
			OrganizationID: org, TeamID: from, ToID: to, ValidFrom: l.ValidFrom, ValidTo: validTo, Seq: seq,
		})
	case entity.LinkProjectArea:
		err = q.InsertProjectArea(ctx, InsertProjectAreaParams{
			OrganizationID: org, ProjectID: from, ToID: to, ValidFrom: l.ValidFrom, ValidTo: validTo, Seq: seq,
		})
	default:
		return fmt.Errorf("org: unknown link kind %q", kind)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.ExclusionViolation {
		return entity.ErrIntervalOverlap
	}
	if err != nil {
		return fmt.Errorf("org: insert %s link: %w", kind, err)
	}
	return nil
}

// PageAreas reads one page more than p.Limit, so the caller knows whether a next page exists.
func (r *Repo) PageAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, scope entity.Scope, f entity.AreaFilter,
	p entity.PageRequest, at time.Time,
) ([]entity.Area, error) {
	if p.Limit < 1 || p.Limit >= 1<<30 {
		return nil, fmt.Errorf("org: page limit %d is out of range", p.Limit)
	}
	q := New(postgres.Conn(ctx, r.pool))
	org, limit := uuid.UUID(orgID), int32(p.Limit+1) //nolint:gosec // bounded by the check above
	var after PageTeamsAscParams
	if p.After != nil {
		after = PageTeamsAscParams{After: true, AfterName: p.After.Name, AfterID: uuid.UUID(p.After.ID)}
	}
	var (
		rows []PageTeamsAscRow
		err  error
	)
	switch kind {
	case entity.AreaTeam:
		arg := PageTeamsAscParams{
			OrganizationID: org, Whole: scope.Whole(), Ids: toUUIDs(scope.Teams()),
			After: after.After, AfterName: after.AfterName, AfterID: after.AfterID, RowLimit: limit,
		}
		if p.Desc {
			var dr []PageTeamsDescRow
			dr, err = q.PageTeamsDesc(ctx, PageTeamsDescParams(arg))
			rows = convert(dr, func(x PageTeamsDescRow) PageTeamsAscRow { return PageTeamsAscRow(x) })
		} else {
			rows, err = q.PageTeamsAsc(ctx, arg)
		}
	case entity.AreaProject:
		arg := PageProjectsAscParams{
			OrganizationID: org, Whole: scope.Whole(), Ids: toUUIDs(scope.Projects()),
			TeamID: uuid.NullUUID{UUID: uuid.UUID(f.TeamID), Valid: !f.TeamID.IsZero()}, At: at,
			After: after.After, AfterName: after.AfterName, AfterID: after.AfterID, RowLimit: limit,
		}
		if p.Desc {
			var dr []PageProjectsDescRow
			dr, err = q.PageProjectsDesc(ctx, PageProjectsDescParams(arg))
			rows = convert(dr, func(x PageProjectsDescRow) PageTeamsAscRow { return PageTeamsAscRow(x) })
		} else {
			var ar []PageProjectsAscRow
			ar, err = q.PageProjectsAsc(ctx, arg)
			rows = convert(ar, func(x PageProjectsAscRow) PageTeamsAscRow { return PageTeamsAscRow(x) })
		}
	case entity.AreaService:
		arg := PageServicesAscParams{
			OrganizationID: org, Whole: scope.Whole(), Ids: toUUIDs(scope.Services()),
			After: after.After, AfterName: after.AfterName, AfterID: after.AfterID, RowLimit: limit,
		}
		if p.Desc {
			var dr []PageServicesDescRow
			dr, err = q.PageServicesDesc(ctx, PageServicesDescParams(arg))
			rows = convert(dr, func(x PageServicesDescRow) PageTeamsAscRow { return PageTeamsAscRow(x) })
		} else {
			var ar []PageServicesAscRow
			ar, err = q.PageServicesAsc(ctx, arg)
			rows = convert(ar, func(x PageServicesAscRow) PageTeamsAscRow { return PageTeamsAscRow(x) })
		}
	default:
		return nil, fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("org: page %ss: %w", kind, err)
	}
	return convert(rows, func(x PageTeamsAscRow) entity.Area { return area(x.ID, x.Name, x.Status) }), nil
}
