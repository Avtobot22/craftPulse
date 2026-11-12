package usecase

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type Installation interface {
	Apply(ctx context.Context, params entity.InstallParams) ([]entity.AppliedOrganization, error)
	Check(ctx context.Context, params entity.InstallParams) (entity.Protocol, error)
}

// Logins is the auth module for the browser: login through the provider, the session and logout.
type Logins interface {
	StartLogin(ctx context.Context, slug, returnTo string) (entity.LoginStart, error)
	CompleteLogin(ctx context.Context, cb entity.LoginCallback) (entity.LoginResult, error)
	Authenticate(ctx context.Context, token string) (entity.Subject, error)
	Logout(ctx context.Context, s entity.Subject) error
}

// Access decides whether the subject may run the action of an operation (TZ 13.4).
type Access interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
	CurrentUser(ctx context.Context, s entity.Subject) (entity.CurrentUser, error)
}

// Structure is the org module for the confirmed structure of the organization (UC-02, FR-012).
type Structure interface {
	Structure(ctx context.Context, s entity.Subject) (entity.Structure, error)
	ReplaceStructure(ctx context.Context, s entity.Subject, baseVersion int64, in entity.StructureInput) (entity.Structure, error)
	ListAreas(ctx context.Context, s entity.Subject, scope entity.Scope, kind entity.AreaKind, f entity.AreaFilter,
		p entity.PageRequest) (entity.Page, error)
}
