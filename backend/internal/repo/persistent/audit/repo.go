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

// PageEvents reads one page more than q.Limit, newest first.
func (r *Repo) PageEvents(ctx context.Context, orgID entity.ID, q entity.AuditQuery) ([]entity.AuditEvent, error) {
	if q.Limit < 1 || q.Limit >= 1<<30 {
		return nil, fmt.Errorf("audit: page limit %d is out of range", q.Limit)
	}
	arg := PageEventsParams{
		OrganizationID: uuid.UUID(orgID),
		ActorID:        nullID(q.ActorID),
		Outcome:        text(string(q.Outcome)),
		OccurredAfter:  pgtype.Timestamptz{Time: q.From, Valid: !q.From.IsZero()},
		OccurredBefore: pgtype.Timestamptz{Time: q.To, Valid: !q.To.IsZero()},
		RowLimit:       int32(q.Limit + 1), //nolint:gosec // bounded by the check above
	}
	if q.After != nil {
		arg.After, arg.AfterAt, arg.AfterID = true, q.After.OccurredAt, uuid.UUID(q.After.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageEvents(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("audit: page events: %w", err)
	}
	events := make([]entity.AuditEvent, len(rows))
	for i := range rows {
		row := &rows[i]
		events[i] = entity.AuditEvent{
			ID:             entity.ID(row.ID),
			OrganizationID: orgID,
			OccurredAt:     row.OccurredAt,
			ActorKind:      entity.ActorKind(row.ActorKind),
			ActorID:        entity.ID(row.ActorID.UUID),
			Operation:      row.Operation,
			ObjectKind:     row.ObjectKind.String,
			ObjectID:       entity.ID(row.ObjectID.UUID),
			Outcome:        entity.AuditOutcome(row.Outcome),
			ReasonCode:     row.ReasonCode.String,
			BaseVersion:    row.BaseVersion.String,
			NewVersion:     row.NewVersion.String,
			CorrelationID:  entity.ID(row.CorrelationID),
		}
	}
	return events, nil
}
