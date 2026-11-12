package response

import (
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func CurrentUser(cu entity.CurrentUser) openapi.CurrentUser {
	p := cu.Profile
	var out openapi.CurrentUser
	out.User = User(p.User)
	out.Organization.Id = openapi_types.UUID(p.Organization.ID)
	out.Organization.Name = p.Organization.Name
	out.Organization.Slug = cu.Slug
	out.Organization.Status = openapi.CurrentUserOrganizationStatus(p.Organization.Status)
	out.Roles = Roles(p.Roles).Data
	out.Areas.Teams = Areas(p.Areas.Teams)
	out.Areas.Projects = Areas(p.Areas.Projects)
	out.Areas.Services = Areas(p.Areas.Services)
	out.Actions = cu.Actions
	out.Session.ExpiresAt = cu.Session.ExpiresAt.UTC()
	out.Session.IdleExpiresAt = cu.Session.IdleExpiresAt.UTC()
	return out
}

// Areas maps teams, projects or services: the contract gives them one schema.
func Areas(in []entity.Area) []openapi.Team {
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

func Structure(st entity.Structure) openapi.OrganizationStructure {
	out := openapi.OrganizationStructure{
		Version:      int(st.Version),
		Teams:        Areas(st.Teams),
		Projects:     Areas(st.Projects),
		Services:     Areas(st.Services),
		Components:   []openapi.Component{},
		ProjectAreas: []openapi.ProjectAreaLink{},
		TeamProjects: make([]openapi.TeamProjectLink, len(st.TeamProjects)),
		TeamMembers:  make([]openapi.TeamMemberLink, len(st.TeamMembers)),
	}
	for i, l := range st.TeamProjects {
		out.TeamProjects[i] = openapi.TeamProjectLink{
			TeamId: openapi_types.UUID(l.From), ProjectId: openapi_types.UUID(l.To),
			ValidFrom: l.ValidFrom.UTC(), ValidTo: optionalTime(l.ValidTo),
		}
	}
	for i, l := range st.TeamMembers {
		out.TeamMembers[i] = openapi.TeamMemberLink{
			TeamId: openapi_types.UUID(l.From), PersonId: openapi_types.UUID(l.To),
			ValidFrom: l.ValidFrom.UTC(), ValidTo: optionalTime(l.ValidTo),
		}
	}
	return out
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func User(u entity.User) openapi.User {
	return openapi.User{
		Id:          openapi_types.UUID(u.ID),
		DisplayName: optionalText(u.DisplayName),
		PersonId:    optionalID(u.PersonID),
		Status:      openapi.UserStatus(u.Status),
	}
}

func Roles(roles []entity.RoleAssignment) openapi.RoleAssignmentList {
	out := openapi.RoleAssignmentList{Data: make([]openapi.RoleAssignment, len(roles))}
	for i, r := range roles {
		out.Data[i] = openapi.RoleAssignment{
			Role:       openapi.Role(r.Role),
			TeamId:     optionalID(r.TeamID),
			ProjectId:  optionalID(r.ProjectID),
			ServiceId:  optionalID(r.ServiceID),
			AssignedAt: r.AssignedAt.UTC(),
		}
	}
	return out
}

// AuditEvent leaves out the details: they are for the journal, not for the API (TZ 13.8).
func AuditEvent(e *entity.AuditEvent) openapi.AuditEvent {
	return openapi.AuditEvent{
		Id:            openapi_types.UUID(e.ID),
		OccurredAt:    e.OccurredAt.UTC(),
		ActorKind:     openapi.AuditEventActorKind(e.ActorKind),
		ActorId:       optionalID(e.ActorID),
		Operation:     e.Operation,
		ObjectKind:    optionalText(e.ObjectKind),
		ObjectId:      optionalID(e.ObjectID),
		Outcome:       openapi.AuditEventOutcome(e.Outcome),
		ReasonCode:    optionalText(e.ReasonCode),
		BaseVersion:   optionalText(e.BaseVersion),
		NewVersion:    optionalText(e.NewVersion),
		CorrelationId: openapi_types.UUID(e.CorrelationID),
	}
}

func Protocol(p entity.Protocol) openapi.InstallationProtocol {
	out := openapi.InstallationProtocol{
		Id:         openapi_types.UUID(p.ID),
		ReleaseSha: p.ReleaseSHA,
		StartedAt:  p.StartedAt.UTC(),
		FinishedAt: optionalTime(p.FinishedAt),
		Outcome:    openapi.InstallationProtocolOutcome(p.Outcome),
	}
	out.Checks = make([]struct {
		CheckKind  openapi.InstallationProtocolChecksCheckKind `json:"check_kind"`
		Outcome    openapi.InstallationProtocolChecksOutcome   `json:"outcome"`
		ReasonCode *string                                     `json:"reason_code"`
	}, len(p.Checks))
	for i, c := range p.Checks {
		out.Checks[i].CheckKind = openapi.InstallationProtocolChecksCheckKind(c.Kind)
		out.Checks[i].Outcome = openapi.InstallationProtocolChecksOutcome(c.Outcome)
		out.Checks[i].ReasonCode = optionalText(c.Reason)
	}
	return out
}
