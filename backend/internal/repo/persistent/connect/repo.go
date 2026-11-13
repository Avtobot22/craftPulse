package connect

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _activeScopeIndex = "connection_scope_active_uq"

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// EnsureInstance inserts the instance unless it exists and reads it back: a concurrent insert of the same
// key waits for the other transaction and then finds its row.
func (r *Repo) EnsureInstance(ctx context.Context, orgID entity.ID, provider entity.Provider, key string) (entity.SourceInstance, error) {
	q := New(postgres.Conn(ctx, r.pool))
	err := q.InsertInstance(ctx, InsertInstanceParams{OrganizationID: uuid.UUID(orgID), Provider: string(provider), InstanceKey: key})
	if err != nil {
		return entity.SourceInstance{}, fmt.Errorf("connect: insert instance: %w", err)
	}
	row, err := q.FindInstanceByKey(ctx, FindInstanceByKeyParams{OrganizationID: uuid.UUID(orgID), Provider: string(provider), InstanceKey: key})
	if err != nil {
		return entity.SourceInstance{}, fmt.Errorf("connect: find instance: %w", err)
	}
	return instance(row), nil
}

func (r *Repo) Instance(ctx context.Context, orgID, id entity.ID) (entity.SourceInstance, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetInstance(ctx, GetInstanceParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.SourceInstance{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.SourceInstance{}, fmt.Errorf("connect: get instance: %w", err)
	}
	return instance(row), nil
}

func (r *Repo) CreateConnection(ctx context.Context, c entity.Connection) error {
	if c.KeyVersion > math.MaxInt32 {
		return fmt.Errorf("connect: master key version %d does not fit the key_version column", c.KeyVersion)
	}
	p := InsertConnectionParams{
		OrganizationID: uuid.UUID(c.OrganizationID), ID: uuid.UUID(c.ID), InstanceID: uuid.UUID(c.InstanceID),
		Name: c.Name, AdapterKind: string(c.AdapterKind), AdapterKeyEnc: c.AdapterKeyEnc,
		ContractVersion: c.ContractVersion, AccessMode: string(c.AccessMode),
		CreatedBy: uuid.UUID(c.CreatedBy), CreatedAt: c.CreatedAt,
	}
	if c.AdapterKind == entity.AdapterClient {
		p.AdapterUrl = pgtype.Text{String: c.AdapterURL, Valid: true}
		p.KeyVersion = pgtype.Int4{Int32: int32(c.KeyVersion), Valid: true} //nolint:gosec // bounded by the check above
	}
	if err := New(postgres.Conn(ctx, r.pool)).InsertConnection(ctx, p); err != nil {
		return fmt.Errorf("connect: insert connection: %w", err)
	}
	return nil
}

func (r *Repo) Connection(ctx context.Context, orgID, id entity.ID) (entity.Connection, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetConnection(ctx, GetConnectionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Connection{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Connection{}, fmt.Errorf("connect: get connection: %w", err)
	}
	c := entity.Connection{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), InstanceID: entity.ID(row.InstanceID),
		Provider: entity.Provider(row.Provider), Name: row.Name, AdapterKind: entity.AdapterKind(row.AdapterKind),
		AdapterURL: row.AdapterUrl.String, AdapterKeyEnc: row.AdapterKeyEnc,
		ContractVersion: row.ContractVersion, AccessMode: entity.AccessMode(row.AccessMode),
		Status: entity.ConnectionStatus(row.Status), PausedUntil: timePtr(row.PausedUntil),
		AccessLostAt: timePtr(row.AccessLostAt), RowVersion: int(row.RowVersion), CreatedBy: entity.ID(row.CreatedBy),
		CreatedAt: row.CreatedAt, DeletedAt: timePtr(row.DeletedAt),
	}
	if row.KeyVersion.Valid && row.KeyVersion.Int32 > 0 {
		c.KeyVersion = uint32(row.KeyVersion.Int32)
	}
	return c, nil
}

// PauseConnection never shortens a pause another call has already set.
func (r *Repo) PauseConnection(ctx context.Context, orgID, id entity.ID, until time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).PauseConnection(ctx, PauseConnectionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), Until: until})
	if err != nil {
		return fmt.Errorf("connect: pause connection: %w", err)
	}
	return nil
}

// MarkAccessLost moves an active connection to access_lost and reports whether it did.
func (r *Repo) MarkAccessLost(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).MarkAccessLost(ctx, MarkAccessLostParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), At: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("connect: mark access lost: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) AddScope(ctx context.Context, orgID, connectionID, instanceID entity.ID, s entity.NewScope, at time.Time) (entity.ID, error) {
	p := InsertScopeParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), InstanceID: uuid.UUID(instanceID),
		ScopeKind: string(s.Kind), NativeScopeID: s.NativeScopeID, DisplayName: s.DisplayName, AddedAt: at,
	}
	if s.HistoryFrom != nil {
		p.HistoryFrom = pgtype.Timestamptz{Time: *s.HistoryFrom, Valid: true}
	}
	if s.RepositoryScopeID != nil {
		p.RepositoryScopeID = uuid.NullUUID{UUID: uuid.UUID(*s.RepositoryScopeID), Valid: true}
	}
	id, err := New(postgres.Conn(ctx, r.pool)).InsertScope(ctx, p)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == _activeScopeIndex {
		return entity.ID{}, entity.ErrScopeTaken
	}
	if err != nil {
		return entity.ID{}, fmt.Errorf("connect: insert scope: %w", err)
	}
	return entity.ID(id), nil
}

func (r *Repo) Scopes(ctx context.Context, orgID, connectionID entity.ID) ([]entity.ConnectionScope, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ListScopes(ctx, ListScopesParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)})
	if err != nil {
		return nil, fmt.Errorf("connect: list scopes: %w", err)
	}
	out := make([]entity.ConnectionScope, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, entity.ConnectionScope{
			ID: entity.ID(row.ID), ConnectionID: entity.ID(row.ConnectionID), InstanceID: entity.ID(row.InstanceID),
			Kind: entity.ScopeKind(row.ScopeKind), NativeScopeID: row.NativeScopeID, DisplayName: row.DisplayName,
			HistoryFrom: timePtr(row.HistoryFrom), RepositoryScopeID: idPtr(row.RepositoryScopeID), AreaID: idPtr(row.AreaID),
			Active: row.Status == "active", AddedAt: row.AddedAt, RemovedAt: timePtr(row.RemovedAt),
		})
	}
	return out, nil
}

// SetScopeArea records the area of a scope once; another area for the same scope is refused.
func (r *Repo) SetScopeArea(ctx context.Context, orgID, scopeID, areaID entity.ID) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).SetScopeArea(ctx, SetScopeAreaParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(scopeID), AreaID: uuid.NullUUID{UUID: uuid.UUID(areaID), Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("connect: set scope area: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) Capabilities(ctx context.Context, orgID, connectionID entity.ID) ([]entity.Capability, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ListCapabilities(ctx, ListCapabilitiesParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)})
	if err != nil {
		return nil, fmt.Errorf("connect: list capabilities: %w", err)
	}
	out := make([]entity.Capability, 0, len(rows))
	for _, row := range rows {
		out = append(out, entity.Capability{
			Name: row.Capability, State: entity.CapabilityState(row.State), ReasonCode: row.ReasonCode.String,
			CheckedAt: row.CheckedAt, SourceVersion: row.SourceVersion.String,
		})
	}
	return out, nil
}

func instance(row ConnectSourceInstance) entity.SourceInstance {
	return entity.SourceInstance{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), Provider: entity.Provider(row.Provider),
		InstanceKey: row.InstanceKey, BaseURL: row.BaseUrl.String, Edition: row.Edition.String,
		SourceVersion: row.SourceVersion.String, VerifiedAt: timePtr(row.VerifiedAt),
	}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func idPtr(id uuid.NullUUID) *entity.ID {
	if !id.Valid {
		return nil
	}
	out := entity.ID(id.UUID)
	return &out
}
