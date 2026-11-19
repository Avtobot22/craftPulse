package access

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestDecide(t *testing.T) {
	t.Parallel()
	team, other := entity.ID{1}, entity.ID{2}
	admin := entity.Profile{
		Whole: true, Roles: []entity.RoleAssignment{{Role: entity.RoleAdministrator}},
		Areas: entity.Areas{Teams: []entity.Area{{ID: team}, {ID: other}}},
	}
	developer := entity.Profile{
		Roles: []entity.RoleAssignment{{Role: entity.RoleDeveloper, TeamID: team}},
		Areas: entity.Areas{Teams: []entity.Area{{ID: team}}},
	}
	newcomer := entity.Profile{}
	tests := []struct {
		name        string
		giveProfile entity.Profile
		giveRequest entity.AccessRequest
		wantReason  string
		wantErr     error
	}{
		{name: "own session without roles", giveProfile: newcomer, giveRequest: entity.AccessRequest{Action: "session.read"}},
		{
			name: "read without area", giveProfile: newcomer, giveRequest: entity.AccessRequest{Action: "org.read"},
			wantReason: ReasonNoPermission, wantErr: entity.ErrForbidden,
		},
		{
			name: "read in own team", giveProfile: developer,
			giveRequest: entity.AccessRequest{Action: "org.read", Area: entity.AreaRef{Kind: entity.AreaTeam, ID: team}},
		},
		{
			name: "other team is not visible", giveProfile: developer,
			giveRequest: entity.AccessRequest{Action: "org.read", Area: entity.AreaRef{Kind: entity.AreaTeam, ID: other}},
			wantReason:  ReasonOutsideScope, wantErr: entity.ErrNotFound,
		},
		{
			name: "administration needs administrator", giveProfile: developer, giveRequest: entity.AccessRequest{Action: "users.manage"},
			wantReason: ReasonNoPermission, wantErr: entity.ErrForbidden,
		},
		{
			name: "administrator sees every area", giveProfile: admin,
			giveRequest: entity.AccessRequest{Action: "org.manage", Area: entity.AreaRef{Kind: entity.AreaTeam, ID: other}},
		},
		{
			name: "action outside the table", giveProfile: admin, giveRequest: entity.AccessRequest{Action: "mr.read"},
			wantReason: ReasonNoPermission, wantErr: entity.ErrForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reason, err := decide(tt.giveProfile, tt.giveRequest)

			assert.Equal(t, tt.wantReason, reason)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestUploadSubjectScope(t *testing.T) {
	t.Parallel()
	project := entity.ID{3}
	s := entity.Subject{OrganizationID: entity.ID{1}, UploadTokenID: entity.ID{2}, ProjectID: project}
	u := &UseCase{}
	for _, action := range []string{"ingest.upload", "receipts.read"} {
		scope, err := u.Authorize(t.Context(), s, entity.AccessRequest{Action: action, Area: entity.AreaRef{Kind: entity.AreaProject, ID: project}})
		assert.NoError(t, err)
		assert.Equal(t, entity.GrantScope(entity.ScopeGrant{Projects: []entity.ID{project}}), scope)
		_, err = u.Authorize(t.Context(), s, entity.AccessRequest{Action: action, Area: entity.AreaRef{Kind: entity.AreaProject, ID: entity.ID{4}}})
		assert.ErrorIs(t, err, entity.ErrNotFound)
	}
	for _, action := range []string{"org.read", "upload_tokens.manage", "session.read", "users.manage", ""} {
		_, err := u.Authorize(t.Context(), s, entity.AccessRequest{Action: action})
		assert.ErrorIs(t, err, entity.ErrForbidden)
	}
}

func TestProjectManagerDecidesInOwnProjects(t *testing.T) {
	t.Parallel()
	core, other := entity.ID{1}, entity.ID{2}
	manager := entity.Profile{
		Roles: []entity.RoleAssignment{{Role: entity.RoleProjectManager, ProjectID: core}, {Role: entity.RoleDeveloper, ProjectID: other}},
		Areas: entity.Areas{Projects: []entity.Area{{ID: core}, {ID: other}}},
	}
	_, err := decide(manager, entity.AccessRequest{Action: "links.decide"})
	assert.NoError(t, err)
	_, err = decide(manager, entity.AccessRequest{Action: "links.decide", Area: entity.AreaRef{Kind: entity.AreaProject, ID: other}})
	assert.ErrorIs(t, err, entity.ErrForbidden, "a developer role does not decide links in its project")
	assert.Equal(t, []entity.ID{core}, managedScope(manager).Projects(), "source checks see only the managed projects")

	developer := entity.Profile{Roles: []entity.RoleAssignment{{Role: entity.RoleDeveloper, ProjectID: core}}, Areas: entity.Areas{Projects: []entity.Area{{ID: core}}}}
	_, err = decide(developer, entity.AccessRequest{Action: "links.decide"})
	assert.ErrorIs(t, err, entity.ErrForbidden)

	whole := entity.Profile{Whole: true, Roles: []entity.RoleAssignment{{Role: entity.RoleProjectManager}}, Areas: manager.Areas}
	assert.True(t, managedScope(whole).Whole())
	assert.ElementsMatch(t, []entity.ID{core, other}, managedScope(whole).Projects())
}
