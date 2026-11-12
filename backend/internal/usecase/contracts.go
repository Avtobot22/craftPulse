package usecase

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type Installation interface {
	Apply(ctx context.Context, params entity.InstallParams) ([]entity.AppliedOrganization, error)
	Check(ctx context.Context, params entity.InstallParams) (entity.Protocol, error)
}
