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
		CreatedBy: uuid.UUID(c.CreatedBy), CreatedAt: c.CreatedAt, CredentialsSetAt: tstz(c.CredentialsSetAt),
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
	return connection(row), nil
}

// LockConnection reads the connection for update; changes of one connection run one after another.
func (r *Repo) LockConnection(ctx context.Context, orgID, id entity.ID) (entity.Connection, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockConnection(ctx, LockConnectionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Connection{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Connection{}, fmt.Errorf("connect: lock connection: %w", err)
	}
	return connection(GetConnectionRow(row)), nil
}

func (r *Repo) PageConnections(ctx context.Context, orgID entity.ID, f entity.ConnectionFilter) ([]entity.Connection, error) {
	if f.Limit < 1 || f.Limit > 201 {
		return nil, errors.New("connect: invalid connection page limit")
	}
	p := PageConnectionsParams{OrganizationID: uuid.UUID(orgID), Status: string(f.Status), PageLimit: int32(f.Limit)}
	if f.After != nil {
		p.HasCursor, p.AfterTime, p.AfterID = true, f.After.OccurredAt, uuid.UUID(f.After.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageConnections(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("connect: page connections: %w", err)
	}
	out := make([]entity.Connection, 0, len(rows))
	for i := range rows {
		out = append(out, connection(GetConnectionRow(rows[i])))
	}
	return out, nil
}

// SaveConnection writes every changeable field of a connection the caller has locked.
func (r *Repo) SaveConnection(ctx context.Context, c entity.Connection) error {
	if c.KeyVersion > math.MaxInt32 || c.RowVersion > math.MaxInt32 || c.ActivatedRowVersion > math.MaxInt32 {
		return errors.New("connect: version does not fit its column")
	}
	p := SaveConnectionParams{
		OrganizationID: uuid.UUID(c.OrganizationID), ID: uuid.UUID(c.ID), InstanceID: uuid.UUID(c.InstanceID), Name: c.Name,
		AdapterKeyEnc: c.AdapterKeyEnc, ContractVersion: c.ContractVersion, AccessMode: string(c.AccessMode), Status: string(c.Status),
		AccessLostAt: tstz(c.AccessLostAt), CredentialsSetAt: tstz(c.CredentialsSetAt), SyncPaused: c.SyncPaused,
		RowVersion: int32(c.RowVersion), DeletedAt: tstz(c.DeletedAt), //nolint:gosec // bounded by the check above
	}
	if c.AdapterKind == entity.AdapterClient {
		p.AdapterUrl = pgtype.Text{String: c.AdapterURL, Valid: true}
		p.KeyVersion = pgtype.Int4{Int32: int32(c.KeyVersion), Valid: true} //nolint:gosec // bounded by the check above
	}
	if c.ActivatedProbeID != nil {
		p.ActivatedProbeID = uuid.NullUUID{UUID: uuid.UUID(*c.ActivatedProbeID), Valid: true}
		p.ActivatedRowVersion = pgtype.Int4{Int32: int32(c.ActivatedRowVersion), Valid: true} //nolint:gosec // bounded by the check above
	}
	err := New(postgres.Conn(ctx, r.pool)).SaveConnection(ctx, p)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == _activeScopeIndex {
		return entity.ErrScopeTaken
	}
	if err != nil {
		return fmt.Errorf("connect: save connection: %w", err)
	}
	return nil
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
	m, err := r.ScopesOf(ctx, orgID, []entity.ID{connectionID})
	return m[connectionID], err
}

func (r *Repo) ScopesOf(ctx context.Context, orgID entity.ID, connectionIDs []entity.ID) (map[entity.ID][]entity.ConnectionScope, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ScopesOf(ctx, ScopesOfParams{OrganizationID: uuid.UUID(orgID), ConnectionIds: uuids(connectionIDs)})
	if err != nil {
		return nil, fmt.Errorf("connect: list scopes: %w", err)
	}
	out := make(map[entity.ID][]entity.ConnectionScope, len(connectionIDs))
	for i := range rows {
		row := &rows[i]
		id := entity.ID(row.ConnectionID)
		out[id] = append(out[id], entity.ConnectionScope{
			ID: entity.ID(row.ID), ConnectionID: id, InstanceID: entity.ID(row.InstanceID),
			Kind: entity.ScopeKind(row.ScopeKind), NativeScopeID: row.NativeScopeID, DisplayName: row.DisplayName,
			HistoryFrom: timePtr(row.HistoryFrom), RepositoryScopeID: idPtr(row.RepositoryScopeID), AreaID: idPtr(row.AreaID),
			Active: row.Status == "active", AddedAt: row.AddedAt, RemovedAt: timePtr(row.RemovedAt),
		})
	}
	return out, nil
}

func (r *Repo) RemoveScope(ctx context.Context, orgID, scopeID entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).RemoveScope(ctx, RemoveScopeParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(scopeID), At: tstz(&at)})
	if err != nil {
		return fmt.Errorf("connect: remove scope: %w", err)
	}
	return nil
}

func (r *Repo) RemoveScopes(ctx context.Context, orgID, connectionID entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).RemoveConnectionScopes(ctx, RemoveConnectionScopesParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), At: tstz(&at),
	})
	if err != nil {
		return fmt.Errorf("connect: remove scopes: %w", err)
	}
	return nil
}

func (r *Repo) SetScopeHistory(ctx context.Context, orgID, scopeID entity.ID, from *time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).SetScopeHistory(ctx, SetScopeHistoryParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(scopeID), HistoryFrom: tstz(from)})
	if err != nil {
		return fmt.Errorf("connect: set scope history: %w", err)
	}
	return nil
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
	m, err := r.CapabilitiesOf(ctx, orgID, []entity.ID{connectionID})
	return m[connectionID], err
}

func (r *Repo) CapabilitiesOf(ctx context.Context, orgID entity.ID, connectionIDs []entity.ID) (map[entity.ID][]entity.Capability, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).CapabilitiesOf(ctx, CapabilitiesOfParams{OrganizationID: uuid.UUID(orgID), ConnectionIds: uuids(connectionIDs)})
	if err != nil {
		return nil, fmt.Errorf("connect: list capabilities: %w", err)
	}
	out := make(map[entity.ID][]entity.Capability, len(connectionIDs))
	for _, row := range rows {
		id := entity.ID(row.ConnectionID)
		out[id] = append(out[id], entity.Capability{
			Name: row.Capability, State: entity.CapabilityState(row.State), ReasonCode: row.ReasonCode.String,
			CheckedAt: row.CheckedAt, SourceVersion: row.SourceVersion.String,
		})
	}
	return out, nil
}

// ReplaceCapabilities stores the capabilities an activation confirmed instead of the previous ones.
func (r *Repo) ReplaceCapabilities(ctx context.Context, orgID, connectionID entity.ID, caps []entity.Capability) error {
	q := New(postgres.Conn(ctx, r.pool))
	if err := q.DeleteCapabilities(ctx, DeleteCapabilitiesParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)}); err != nil {
		return fmt.Errorf("connect: delete capabilities: %w", err)
	}
	for _, c := range caps {
		err := q.InsertCapability(ctx, InsertCapabilityParams{
			OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), Capability: c.Name, State: string(c.State),
			ReasonCode: text(c.ReasonCode), CheckedAt: c.CheckedAt, SourceVersion: text(c.SourceVersion),
		})
		if err != nil {
			return fmt.Errorf("connect: insert capability: %w", err)
		}
	}
	return nil
}

func (r *Repo) SetInstanceBaseURL(ctx context.Context, orgID, id entity.ID, baseURL string) error {
	err := New(postgres.Conn(ctx, r.pool)).SetInstanceBaseURL(ctx, SetInstanceBaseURLParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), BaseUrl: text(baseURL)})
	if err != nil {
		return fmt.Errorf("connect: set instance address: %w", err)
	}
	return nil
}

// VerifyInstance records what a successful probe found out about the instance.
func (r *Repo) VerifyInstance(ctx context.Context, in entity.SourceInstance) error {
	err := New(postgres.Conn(ctx, r.pool)).VerifyInstance(ctx, VerifyInstanceParams{
		OrganizationID: uuid.UUID(in.OrganizationID), ID: uuid.UUID(in.ID), BaseUrl: text(in.BaseURL),
		Edition: text(in.Edition), SourceVersion: text(in.SourceVersion), VerifiedAt: tstz(in.VerifiedAt),
	})
	if err != nil {
		return fmt.Errorf("connect: verify instance: %w", err)
	}
	return nil
}

func connection(row GetConnectionRow) entity.Connection {
	c := entity.Connection{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), InstanceID: entity.ID(row.InstanceID),
		Provider: entity.Provider(row.Provider), Name: row.Name, AdapterKind: entity.AdapterKind(row.AdapterKind),
		AdapterURL: row.AdapterUrl.String, AdapterKeyEnc: row.AdapterKeyEnc,
		ContractVersion: row.ContractVersion, AccessMode: entity.AccessMode(row.AccessMode),
		Status: entity.ConnectionStatus(row.Status), PausedUntil: timePtr(row.PausedUntil),
		AccessLostAt: timePtr(row.AccessLostAt), CredentialsSetAt: timePtr(row.CredentialsSetAt), SyncPaused: row.SyncPaused,
		RowVersion: int(row.RowVersion), ActivatedProbeID: idPtr(row.ActivatedProbeID), ActivatedRowVersion: int(row.ActivatedRowVersion.Int32),
		CreatedBy: entity.ID(row.CreatedBy), CreatedAt: row.CreatedAt, DeletedAt: timePtr(row.DeletedAt),
	}
	if row.KeyVersion.Valid && row.KeyVersion.Int32 > 0 {
		c.KeyVersion = uint32(row.KeyVersion.Int32)
	}
	return c
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

func tstz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func uuids(ids []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		out[i] = uuid.UUID(id)
	}
	return out
}
