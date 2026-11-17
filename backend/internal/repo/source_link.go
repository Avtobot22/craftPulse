package repo

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type SourceLinkRepo interface {
	SourceLinks(context.Context, entity.ID, entity.ID) ([]entity.SourceLink, error)
	ReserveSourceLink(context.Context, entity.ID, entity.ID, entity.ID) (int64, error)
	CreateSourceLinkAttempt(context.Context, entity.ID, entity.SourceLinkAttempt) error
	TakeSourceLinkAttempt(context.Context, entity.ID, entity.ID, entity.ID, []byte) (entity.SourceLinkAttempt, bool, error)
	ConfirmSourceLink(context.Context, entity.ID, entity.ID, entity.ID, int64, time.Time) (bool, error)
	RevokeSourceLink(context.Context, entity.ID, entity.ID, entity.ID) (bool, error)
	ExpireSourceLink(context.Context, entity.ID, entity.ID, entity.ID, int64) error
}
