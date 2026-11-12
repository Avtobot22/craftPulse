package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) CreateOrganization(ctx context.Context, o entity.NewOrganization, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateOrganization(ctx, CreateOrganizationParams{
		OrganizationID:    uuid.UUID(o.ID),
		Name:              o.Name,
		Status:            string(entity.OrgStatusSetup),
		FirstAdminIssuer:  o.Provider.Issuer,
		FirstAdminSubject: o.FirstAdminSubject,
		CreatedAt:         at,
	})
	if err != nil {
		return fmt.Errorf("org: create organization: %w", err)
	}
	return nil
}

func (r *Repo) Organization(ctx context.Context, orgID entity.ID) (entity.Organization, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetOrganization(ctx, uuid.UUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Organization{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Organization{}, fmt.Errorf("org: get organization: %w", err)
	}
	return organization(GetOrganizationRow(row)), nil
}

// LockOrganization holds the row until the transaction ends, so two first logins do not both assign the role.
func (r *Repo) LockOrganization(ctx context.Context, orgID entity.ID) (entity.Organization, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockOrganization(ctx, uuid.UUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Organization{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Organization{}, fmt.Errorf("org: lock organization: %w", err)
	}
	return organization(GetOrganizationRow(row)), nil
}

func (r *Repo) ActivateOrganization(ctx context.Context, orgID, userID entity.ID) error {
	err := New(postgres.Conn(ctx, r.pool)).ActivateOrganization(ctx, ActivateOrganizationParams{
		UserID: nullID(userID), OrganizationID: uuid.UUID(orgID),
	})
	if err != nil {
		return fmt.Errorf("org: activate organization: %w", err)
	}
	return nil
}

func (r *Repo) LockUserByIdentity(ctx context.Context, orgID entity.ID, issuer, subject string) (entity.User, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockUserByIdentity(ctx, LockUserByIdentityParams{
		OrganizationID: uuid.UUID(orgID), Issuer: issuer, Subject: subject,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, false, nil
	}
	if err != nil {
		return entity.User{}, false, fmt.Errorf("org: lock user: %w", err)
	}
	return user(GetUserRow(row)), true, nil
}

// CreateUser does nothing when a concurrent login has created the same user.
func (r *Repo) CreateUser(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateUser(ctx, CreateUserParams{
		OrganizationID: uuid.UUID(orgID),
		Issuer:         id.Issuer,
		Subject:        id.Subject,
		DisplayName:    text(id.DisplayName),
		CreatedAt:      at,
	})
	if err != nil {
		return fmt.Errorf("org: create user: %w", err)
	}
	return nil
}

func (r *Repo) SetUserDisplayName(ctx context.Context, orgID, userID entity.ID, name string) error {
	err := New(postgres.Conn(ctx, r.pool)).SetUserDisplayName(ctx, SetUserDisplayNameParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(userID), DisplayName: text(name),
	})
	if err != nil {
		return fmt.Errorf("org: set user display name: %w", err)
	}
	return nil
}

func (r *Repo) User(ctx context.Context, orgID, userID entity.ID) (entity.User, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetUser(ctx, GetUserParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.User{}, fmt.Errorf("org: get user: %w", err)
	}
	return user(row), nil
}

func (r *Repo) AssignAdministrator(ctx context.Context, orgID, userID entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).AssignAdministrator(ctx, AssignAdministratorParams{
		OrganizationID: uuid.UUID(orgID), UserID: uuid.UUID(userID), At: at,
	})
	if err != nil {
		return fmt.Errorf("org: assign administrator: %w", err)
	}
	return nil
}

func (r *Repo) ActiveRoles(ctx context.Context, orgID, userID entity.ID) ([]entity.RoleAssignment, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ListActiveRoles(ctx, ListActiveRolesParams{
		OrganizationID: uuid.UUID(orgID), UserID: uuid.UUID(userID),
	})
	if err != nil {
		return nil, fmt.Errorf("org: list roles: %w", err)
	}
	roles := make([]entity.RoleAssignment, len(rows))
	for i, row := range rows {
		roles[i] = entity.RoleAssignment{
			Role:       entity.Role(row.Role),
			TeamID:     entity.ID(row.TeamID.UUID),
			ProjectID:  entity.ID(row.ProjectID.UUID),
			ServiceID:  entity.ID(row.ServiceID.UUID),
			AssignedAt: row.AssignedAt,
		}
	}
	return roles, nil
}

func (r *Repo) MemberTeamIDs(ctx context.Context, orgID, personID entity.ID, at time.Time) ([]entity.ID, error) {
	ids, err := New(postgres.Conn(ctx, r.pool)).ListMemberTeamIDs(ctx, ListMemberTeamIDsParams{
		OrganizationID: uuid.UUID(orgID), PersonID: uuid.UUID(personID), At: at,
	})
	if err != nil {
		return nil, fmt.Errorf("org: list member teams: %w", err)
	}
	return fromUUIDs(ids), nil
}

func (r *Repo) TeamProjectIDs(ctx context.Context, orgID entity.ID, teamIDs []entity.ID, at time.Time) ([]entity.ID, error) {
	ids, err := New(postgres.Conn(ctx, r.pool)).ListTeamProjectIDs(ctx, ListTeamProjectIDsParams{
		OrganizationID: uuid.UUID(orgID), TeamIds: toUUIDs(teamIDs), At: at,
	})
	if err != nil {
		return nil, fmt.Errorf("org: list team projects: %w", err)
	}
	return fromUUIDs(ids), nil
}

func (r *Repo) ActiveAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, whole bool, ids []entity.ID) ([]entity.Area, error) {
	q := New(postgres.Conn(ctx, r.pool))
	var (
		rows []ListActiveTeamsRow
		err  error
	)
	switch kind {
	case entity.AreaTeam:
		rows, err = q.ListActiveTeams(ctx, ListActiveTeamsParams{OrganizationID: uuid.UUID(orgID), Whole: whole, Ids: toUUIDs(ids)})
	case entity.AreaProject:
		var pr []ListActiveProjectsRow
		pr, err = q.ListActiveProjects(ctx, ListActiveProjectsParams{OrganizationID: uuid.UUID(orgID), Whole: whole, Ids: toUUIDs(ids)})
		for _, p := range pr {
			rows = append(rows, ListActiveTeamsRow(p))
		}
	case entity.AreaService:
		var sr []ListActiveServicesRow
		sr, err = q.ListActiveServices(ctx, ListActiveServicesParams{OrganizationID: uuid.UUID(orgID), Whole: whole, Ids: toUUIDs(ids)})
		for _, s := range sr {
			rows = append(rows, ListActiveTeamsRow(s))
		}
	default:
		return nil, fmt.Errorf("org: unknown area kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("org: list %ss: %w", kind, err)
	}
	areas := make([]entity.Area, len(rows))
	for i, row := range rows {
		areas[i] = entity.Area{ID: entity.ID(row.ID), Name: row.Name, Status: row.Status}
	}
	return areas, nil
}

func organization(row GetOrganizationRow) entity.Organization {
	return entity.Organization{
		ID:                entity.ID(row.OrganizationID),
		Name:              row.Name,
		Status:            entity.OrgStatus(row.Status),
		FirstAdminIssuer:  row.FirstAdminIssuer,
		FirstAdminSubject: row.FirstAdminSubject,
	}
}

func user(row GetUserRow) entity.User {
	return entity.User{
		ID:          entity.ID(row.ID),
		Issuer:      row.Issuer,
		Subject:     row.Subject,
		PersonID:    entity.ID(row.PersonID.UUID),
		DisplayName: row.DisplayName.String,
		Status:      entity.UserStatus(row.Status),
	}
}

func nullID(id entity.ID) uuid.NullUUID {
	return uuid.NullUUID{UUID: uuid.UUID(id), Valid: !id.IsZero()}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func toUUIDs(ids []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		out[i] = uuid.UUID(id)
	}
	return out
}

func fromUUIDs(ids []uuid.UUID) []entity.ID {
	out := make([]entity.ID, len(ids))
	for i, id := range ids {
		out[i] = entity.ID(id)
	}
	return out
}
