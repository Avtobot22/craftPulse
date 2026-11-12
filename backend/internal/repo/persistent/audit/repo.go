package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
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

func (r *Repo) InsertEvent(ctx context.Context, e entity.AuditEvent) error {
	var details []byte
	if len(e.Details) > 0 {
		var err error
		if details, err = json.Marshal(e.Details); err != nil {
			return fmt.Errorf("audit: encode details: %w", err)
		}
	}
	err := New(postgres.Conn(ctx, r.pool)).InsertEvent(ctx, InsertEventParams{
		OrganizationID: uuid.UUID(e.OrganizationID),
		OccurredAt:     e.OccurredAt,
		ActorKind:      string(e.ActorKind),
		ActorID:        nullID(e.ActorID),
		Operation:      e.Operation,
		ObjectKind:     text(e.ObjectKind),
		ObjectID:       nullID(e.ObjectID),
		AreaID:         nullID(e.AreaID),
		Outcome:        string(e.Outcome),
		ReasonCode:     text(e.ReasonCode),
		BaseVersion:    text(e.BaseVersion),
		NewVersion:     text(e.NewVersion),
		CorrelationID:  uuid.UUID(e.CorrelationID),
		Details:        details,
	})
	if err != nil {
		return fmt.Errorf("audit: insert event: %w", err)
	}
	return nil
}

func nullID(id entity.ID) uuid.NullUUID {
	return uuid.NullUUID{UUID: uuid.UUID(id), Valid: !id.IsZero()}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
