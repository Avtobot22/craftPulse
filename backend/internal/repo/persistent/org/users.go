package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// LockUser holds the user row until the transaction ends: role changes and blocking of one user run one by one.
func (r *Repo) LockUser(ctx context.Context, orgID, userID entity.ID) (entity.User, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockUser(ctx, LockUserParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.User{}, fmt.Errorf("org: lock user: %w", err)
	}
	return user(GetUserRow(row)), nil
}

// SetUserStatus stores blockedAt for a blocked user; an active user has none.
func (r *Repo) SetUserStatus(ctx context.Context, orgID, userID entity.ID, status entity.UserStatus, blockedAt time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).SetUserStatus(ctx, SetUserStatusParams{
		Status:         string(status),
		BlockedAt:      pgtype.Timestamptz{Time: blockedAt, Valid: !blockedAt.IsZero()},
		OrganizationID: uuid.UUID(orgID),
		ID:             uuid.UUID(userID),
	})
	if err != nil {
		return fmt.Errorf("org: set user status: %w", err)
	}
	return nil
}

// RoleSetVersion counts assignments and revocations: rows are never deleted or reassigned, so the
// number grows with every change of the set and serves as its version.
func (r *Repo) RoleSetVersion(ctx context.Context, orgID, userID entity.ID) (int64, error) {
	v, err := New(postgres.Conn(ctx, r.pool)).RoleSetVersion(ctx, RoleSetVersionParams{
		OrganizationID: uuid.UUID(orgID), UserID: uuid.UUID(userID),
	})
	if err != nil {
		return 0, fmt.Errorf("org: role set version: %w", err)
	}
	return v, nil
}

func (r *Repo) RevokeRole(ctx context.Context, orgID, assignmentID, by entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).RevokeRole(ctx, RevokeRoleParams{
		At: at, RevokedBy: uuid.UUID(by), OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(assignmentID),
	})
	if err != nil {
		return fmt.Errorf("org: revoke role: %w", err)
	}
	return nil
}

func (r *Repo) AssignRole(ctx context.Context, orgID, userID entity.ID, ra entity.RoleAssignment, by entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).AssignRole(ctx, AssignRoleParams{
		OrganizationID: uuid.UUID(orgID),
		UserID:         uuid.UUID(userID),
		Role:           string(ra.Role),
		TeamID:         nullID(ra.TeamID),
		ProjectID:      nullID(ra.ProjectID),
		ServiceID:      nullID(ra.ServiceID),
		AssignedBy:     uuid.UUID(by),
		AssignedAt:     at,
	})
	if err != nil {
		return fmt.Errorf("org: assign role: %w", err)
	}
	return nil
}

func (r *Repo) ExistingAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, ids []entity.ID) ([]entity.ID, error) {
	q := New(postgres.Conn(ctx, r.pool))
	org, uuids := uuid.UUID(orgID), toUUIDs(ids)
	var (
		found []uuid.UUID
		err   error
	)
	switch kind {
	case entity.AreaTeam:
		found, err = q.ListTeamIDs(ctx, ListTeamIDsParams{OrganizationID: org, Ids: uuids})
	case entity.AreaProject:
		found, err = q.ListProjectIDs(ctx, ListProjectIDsParams{OrganizationID: org, Ids: uuids})
	case entity.AreaService:
		found, err = q.ListServiceIDs(ctx, ListServiceIDsParams{OrganizationID: org, Ids: uuids})
	default:
		return nil, fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("org: list %s IDs: %w", kind, err)
	}
	return fromUUIDs(found), nil
}

// PageUsers reads one page more than req.Limit, so the caller knows whether a next page exists.
func (r *Repo) PageUsers(ctx context.Context, orgID entity.ID, req entity.UserListRequest) ([]entity.User, error) {
	if req.Limit < 1 || req.Limit >= 1<<30 {
		return nil, fmt.Errorf("org: page limit %d is out of range", req.Limit)
	}
	q := New(postgres.Conn(ctx, r.pool))
	arg := PageUsersByNameAscParams{
		OrganizationID: uuid.UUID(orgID),
		Status:         pgtype.Text{String: string(req.Status), Valid: req.Status != ""},
		RowLimit:       int32(req.Limit + 1), //nolint:gosec // bounded by the check above
	}
	byCreated := PageUsersByCreatedAscParams{OrganizationID: arg.OrganizationID, Status: arg.Status, RowLimit: arg.RowLimit}
	if req.After != nil {
		arg.After, arg.AfterName, arg.AfterID = true, req.After.Name, uuid.UUID(req.After.ID)
		byCreated.After, byCreated.AfterAt, byCreated.AfterID = true, req.After.CreatedAt, uuid.UUID(req.After.ID)
	}
	var (
		rows []PageUsersByNameAscRow
		err  error
	)
	switch {
	case req.Sort == entity.UserByCreated && req.Desc:
		var cr []PageUsersByCreatedDescRow
		cr, err = q.PageUsersByCreatedDesc(ctx, PageUsersByCreatedDescParams(byCreated))
		rows = convert(cr, func(x PageUsersByCreatedDescRow) PageUsersByNameAscRow { return PageUsersByNameAscRow(x) })
	case req.Sort == entity.UserByCreated:
		var cr []PageUsersByCreatedAscRow
		cr, err = q.PageUsersByCreatedAsc(ctx, byCreated)
		rows = convert(cr, func(x PageUsersByCreatedAscRow) PageUsersByNameAscRow { return PageUsersByNameAscRow(x) })
	case req.Desc:
		var nr []PageUsersByNameDescRow
		nr, err = q.PageUsersByNameDesc(ctx, PageUsersByNameDescParams(arg))
		rows = convert(nr, func(x PageUsersByNameDescRow) PageUsersByNameAscRow { return PageUsersByNameAscRow(x) })
	default:
		rows, err = q.PageUsersByNameAsc(ctx, arg)
	}
	if err != nil {
		return nil, fmt.Errorf("org: page users: %w", err)
	}
	return convert(rows, func(x PageUsersByNameAscRow) entity.User { return user(GetUserRow(x)) }), nil
}
