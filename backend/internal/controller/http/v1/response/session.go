package response

import (
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func CurrentUser(cu entity.CurrentUser) openapi.CurrentUser {
	p := cu.Profile
	var out openapi.CurrentUser
	out.User = openapi.User{
		Id:          openapi_types.UUID(p.User.ID),
		DisplayName: optionalText(p.User.DisplayName),
		PersonId:    optionalID(p.User.PersonID),
		Status:      openapi.UserStatus(p.User.Status),
	}
	out.Organization.Id = openapi_types.UUID(p.Organization.ID)
	out.Organization.Name = p.Organization.Name
	out.Organization.Slug = cu.Slug
	out.Organization.Status = openapi.CurrentUserOrganizationStatus(p.Organization.Status)
	out.Roles = make([]openapi.RoleAssignment, len(p.Roles))
	for i, r := range p.Roles {
		out.Roles[i] = openapi.RoleAssignment{
			Role:       openapi.Role(r.Role),
			TeamId:     optionalID(r.TeamID),
			ProjectId:  optionalID(r.ProjectID),
			ServiceId:  optionalID(r.ServiceID),
			AssignedAt: r.AssignedAt.UTC(),
		}
	}
	out.Areas.Teams = areas(p.Areas.Teams)
	out.Areas.Projects = areas(p.Areas.Projects)
	out.Areas.Services = areas(p.Areas.Services)
	out.Actions = cu.Actions
	out.Session.ExpiresAt = cu.Session.ExpiresAt.UTC()
	out.Session.IdleExpiresAt = cu.Session.IdleExpiresAt.UTC()
	return out
}

func areas(in []entity.Area) []openapi.Team {
	out := make([]openapi.Team, len(in))
	for i, a := range in {
		out[i] = openapi.Team{Id: openapi_types.UUID(a.ID), Name: a.Name, Status: openapi.TeamStatus(a.Status)}
	}
	return out
}

func optionalID(id entity.ID) *openapi_types.UUID {
	if id.IsZero() {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
