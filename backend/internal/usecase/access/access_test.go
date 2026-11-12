package access

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestDecide(t *testing.T) {
	t.Parallel()
	team, other := entity.ID{1}, entity.ID{2}
	admin := entity.Profile{Whole: true, Roles: []entity.RoleAssignment{{Role: entity.RoleAdministrator}}}
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
			name: "action outside the table", giveProfile: admin, giveRequest: entity.AccessRequest{Action: "report.read"},
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
