package view

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// CreateOperation writes a queued descriptor; false when the same user has an active one of the resource.
func (r *Repo) CreateOperation(ctx context.Context, op entity.Operation) (bool, error) {
	_, err := New(postgres.Conn(ctx, r.pool)).CreateOperation(ctx, CreateOperationParams{
		OrganizationID: uuid.UUID(op.OrganizationID), ID: uuid.UUID(op.ID), RequestedBy: uuid.UUID(op.RequestedBy), Kind: string(op.Kind),
		ResourceKind: op.ResourceKind, ResourceID: uuid.UUID(op.ResourceID), Action: op.Action, Parameters: op.Parameters,
		RequestHash: op.RequestHash, CreatedAt: op.CreatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("view: create operation: %w", err)
	}
	return true, nil
}

// ActiveOperation locks the active descriptor of a user for a resource.
func (r *Repo) ActiveOperation(ctx context.Context, op entity.Operation) (entity.Operation, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).ActiveOperation(ctx, ActiveOperationParams{
		OrganizationID: uuid.UUID(op.OrganizationID), RequestedBy: uuid.UUID(op.RequestedBy), Kind: string(op.Kind),
		ResourceKind: op.ResourceKind, ResourceID: uuid.UUID(op.ResourceID),
	})
	return operation(row, err)
}

func (r *Repo) Operation(ctx context.Context, orgID, id entity.ID) (entity.Operation, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetOperation(ctx, GetOperationParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	return operation(row, err)
}

// UpdateOperation moves an active descriptor to its new status; a finished one does not change.
func (r *Repo) UpdateOperation(ctx context.Context, op entity.Operation) error {
	p := UpdateOperationParams{
		OrganizationID: uuid.UUID(op.OrganizationID), ID: uuid.UUID(op.ID), Status: string(op.Status), UpdatedAt: op.UpdatedAt,
		ResultUri: pgtype.Text{String: op.ResultURI, Valid: op.ResultURI != ""}, ErrorCode: pgtype.Text{String: op.ErrorCode, Valid: op.ErrorCode != ""},
	}
	if op.CompletedAt != nil {
		p.CompletedAt = pgtype.Timestamptz{Time: *op.CompletedAt, Valid: true}
	}
	if op.CorrelationID != nil {
		p.CorrelationID = uuid.NullUUID{UUID: uuid.UUID(*op.CorrelationID), Valid: true}
	}
	if err := New(postgres.Conn(ctx, r.pool)).UpdateOperation(ctx, p); err != nil {
		return fmt.Errorf("view: update operation: %w", err)
	}
	return nil
}

func operation(row ViewOperation, err error) (entity.Operation, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Operation{}, false, nil
	}
	if err != nil {
		return entity.Operation{}, false, fmt.Errorf("view: read operation: %w", err)
	}
	op := entity.Operation{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), RequestedBy: entity.ID(row.RequestedBy),
		Kind: entity.OperationKind(row.Kind), ResourceKind: row.ResourceKind, ResourceID: entity.ID(row.ResourceID), Action: row.Action,
		Parameters: row.Parameters, RequestHash: row.RequestHash, Status: entity.OperationStatus(row.Status), CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt, ResultURI: row.ResultUri.String, ErrorCode: row.ErrorCode.String,
	}
	if row.CompletedAt.Valid {
		op.CompletedAt = &row.CompletedAt.Time
	}
	if row.CorrelationID.Valid {
		id := entity.ID(row.CorrelationID.UUID)
		op.CorrelationID = &id
	}
	return op, true, nil
}
