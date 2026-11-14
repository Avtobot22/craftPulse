package org

import (
	"context"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

// Sources is the connect module: it knows which source areas its connections collected (TZ 10.3).
type Sources interface {
	AreaSources(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaSource, error)
}

// Deps of the module; installation uses only Orgs, Tenants and Clock.
type Deps struct {
	Orgs        repo.OrgRepo
	Tenants     repo.TenantRepo
	Tx          repo.Transactor
	Auditor     Auditor
	Credentials Credentials
	Sources     Sources
	Clock       clock.Clock
	NewID       func() (entity.ID, error)
}

type UseCase struct {
	orgs        repo.OrgRepo
	tenants     repo.TenantRepo
	tx          repo.Transactor
	auditor     Auditor
	credentials Credentials
	sources     Sources
	clock       clock.Clock
	newID       func() (entity.ID, error)
}

func New(d Deps) *UseCase {
	return &UseCase{
		orgs: d.Orgs, tenants: d.Tenants, tx: d.Tx, auditor: d.Auditor, credentials: d.Credentials,
		sources: d.Sources, clock: d.Clock, newID: d.NewID,
	}
}

// CreateOrganization runs inside the caller's organization transaction: the tenant row and the
// organization appear together or not at all (TZ 12.5.1).
func (u *UseCase) CreateOrganization(ctx context.Context, o entity.NewOrganization) error {
	now := u.clock.Now()
	if err := u.tenants.CreateTenant(ctx, o.ID, now); err != nil {
		return err
	}
	return u.orgs.CreateOrganization(ctx, o, now)
}

// SignIn finds or creates the user of a confirmed identity and applies the first administrator rule
// (FR-064). It runs in the caller's organization transaction, so a refused login leaves no user behind.
func (u *UseCase) SignIn(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) (entity.SignIn, error) {
	o, err := u.orgs.LockOrganization(ctx, orgID)
	if err != nil {
		return entity.SignIn{}, err
	}
	setup := o.Status == entity.OrgStatusSetup
	switch {
	case setup && (id.Issuer != o.FirstAdminIssuer || id.Subject != o.FirstAdminSubject):
		return entity.SignIn{}, entity.ErrOrganizationNotConfigured
	case !setup && o.Status != entity.OrgStatusActive:
		return entity.SignIn{}, entity.ErrOrganizationUnavailable
	}

	user, err := u.user(ctx, orgID, id, at)
	if err != nil {
		return entity.SignIn{}, err
	}
	if user.Status == entity.UserBlocked {
		return entity.SignIn{}, entity.ErrUserBlocked
	}
	if !setup {
		return entity.SignIn{User: user}, nil
	}
	if err := u.orgs.AssignAdministrator(ctx, orgID, user.ID, at); err != nil {
		return entity.SignIn{}, err
	}
	if err := u.orgs.ActivateOrganization(ctx, orgID, user.ID); err != nil {
		return entity.SignIn{}, err
	}
	return entity.SignIn{User: user, FirstAdministrator: true}, nil
}

func (u *UseCase) user(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) (entity.User, error) {
	user, found, err := u.orgs.LockUserByIdentity(ctx, orgID, id.Issuer, id.Subject)
	if err != nil {
		return entity.User{}, err
	}
	if found {
		if id.DisplayName != "" && id.DisplayName != user.DisplayName {
			if err := u.orgs.SetUserDisplayName(ctx, orgID, user.ID, id.DisplayName); err != nil {
				return entity.User{}, err
			}
			user.DisplayName = id.DisplayName
		}
		return user, nil
	}
	// A concurrent first login may win the insert; the second read then waits for it and finds the row.
	if err := u.orgs.CreateUser(ctx, orgID, id, at); err != nil {
		return entity.User{}, err
	}
	user, found, err = u.orgs.LockUserByIdentity(ctx, orgID, id.Issuer, id.Subject)
	if err != nil {
		return entity.User{}, err
	}
	if !found {
		return entity.User{}, fmt.Errorf("org: user %s of %s is not found after creation", id.Subject, id.Issuer)
	}
	return user, nil
}

// Profile reads the user, roles and areas on every request: a role change applies to the next one (TZ 7.2).
// It runs in the caller's organization transaction.
func (u *UseCase) Profile(ctx context.Context, orgID, userID entity.ID, at time.Time) (entity.Profile, error) {
	user, err := u.orgs.User(ctx, orgID, userID)
	if err != nil {
		return entity.Profile{}, err
	}
	o, err := u.orgs.Organization(ctx, orgID)
	if err != nil {
		return entity.Profile{}, err
	}
	roles, err := u.orgs.ActiveRoles(ctx, orgID, userID)
	if err != nil {
		return entity.Profile{}, err
	}
	p := entity.Profile{User: user, Organization: o, Roles: roles}

	var teams, projects, services []entity.ID
	for _, r := range roles {
		if r.Role == entity.RoleSecurityAccess || r.Role == entity.RoleWikiAccess {
			continue
		}
		switch {
		case !r.TeamID.IsZero():
			teams = append(teams, r.TeamID)
		case !r.ProjectID.IsZero():
			projects = append(projects, r.ProjectID)
		case !r.ServiceID.IsZero():
			services = append(services, r.ServiceID)
		default:
			p.Whole = true
		}
	}
	if !p.Whole && !user.PersonID.IsZero() {
		member, err := u.orgs.MemberTeamIDs(ctx, orgID, user.PersonID, at)
		if err != nil {
			return entity.Profile{}, err
		}
		teams = append(teams, member...)
	}
	if !p.Whole && len(teams) > 0 {
		owned, err := u.orgs.TeamProjectIDs(ctx, orgID, teams, at)
		if err != nil {
			return entity.Profile{}, err
		}
		projects = append(projects, owned...)
	}
	if p.Areas.Teams, err = u.orgs.VisibleAreas(ctx, orgID, entity.AreaTeam, p.Whole, teams); err != nil {
		return entity.Profile{}, err
	}
	if p.Areas.Projects, err = u.orgs.VisibleAreas(ctx, orgID, entity.AreaProject, p.Whole, projects); err != nil {
		return entity.Profile{}, err
	}
	if p.Areas.Services, err = u.orgs.VisibleAreas(ctx, orgID, entity.AreaService, p.Whole, services); err != nil {
		return entity.Profile{}, err
	}

	var uploadTeams, uploadProjects []entity.ID
	uploadWhole := false
	for _, r := range roles {
		if r.Role != entity.RoleAdministrator && r.Role != entity.RoleTechLead {
			continue
		}
		switch {
		case !r.ProjectID.IsZero():
			uploadProjects = append(uploadProjects, r.ProjectID)
		case !r.TeamID.IsZero():
			uploadTeams = append(uploadTeams, r.TeamID)
		case r.ServiceID.IsZero():
			uploadWhole = true
		}
	}
	if len(uploadTeams) > 0 {
		owned, err := u.orgs.TeamProjectIDs(ctx, orgID, uploadTeams, at)
		if err != nil {
			return entity.Profile{}, err
		}
		uploadProjects = append(uploadProjects, owned...)
	}
	allowed, err := u.orgs.VisibleAreas(ctx, orgID, entity.AreaProject, uploadWhole, uploadProjects)
	if err != nil {
		return entity.Profile{}, err
	}
	for _, a := range allowed {
		p.UploadProjects = append(p.UploadProjects, a.ID)
	}
	return p, nil
}

// EnsureSourceAccounts records the accounts a package names in the caller's transaction and returns
// their IDs by native ID: normalize writes them through org, which owns them (TZ 10.3).
func (u *UseCase) EnsureSourceAccounts(ctx context.Context, orgID, instanceID entity.ID, accounts []entity.SourceAccount) (map[string]entity.ID, error) {
	if len(accounts) == 0 {
		return map[string]entity.ID{}, nil
	}
	return u.orgs.EnsureSourceAccounts(ctx, orgID, instanceID, accounts, u.clock.Now())
}
