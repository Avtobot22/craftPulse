package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _runningProbeIndex = "probe_running_uq"

// probeCapability is one element of connect.probe.capabilities: a checked capability without credentials
// and without answers of the source.
type probeCapability struct {
	Capability    string    `json:"capability"`
	State         string    `json:"state"`
	ReasonCode    string    `json:"reason_code,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
	SourceVersion string    `json:"source_version,omitempty"`
}

func (r *Repo) RunningProbe(ctx context.Context, orgID, connectionID entity.ID) (entity.Probe, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).RunningProbe(ctx, RunningProbeParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Probe{}, false, nil
	}
	if err != nil {
		return entity.Probe{}, false, fmt.Errorf("connect: running probe: %w", err)
	}
	p, err := probe(row)
	return p, err == nil, err
}

// CreateProbe starts a probe; a probe already running for the connection gives ErrStateConflict.
func (r *Repo) CreateProbe(ctx context.Context, orgID, connectionID entity.ID, configurationVersion int, at time.Time) (entity.Probe, error) {
	if configurationVersion > math.MaxInt32 {
		return entity.Probe{}, errors.New("connect: configuration version does not fit its column")
	}
	row, err := New(postgres.Conn(ctx, r.pool)).InsertProbe(ctx, InsertProbeParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID),
		ConfigurationVersion: int32(configurationVersion), StartedAt: at, //nolint:gosec // bounded by the check above
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == _runningProbeIndex {
		return entity.Probe{}, entity.ErrStateConflict
	}
	if err != nil {
		return entity.Probe{}, fmt.Errorf("connect: insert probe: %w", err)
	}
	return probe(row)
}

func (r *Repo) Probe(ctx context.Context, orgID, connectionID, probeID entity.ID) (entity.Probe, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetProbe(ctx, GetProbeParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), ID: uuid.UUID(probeID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Probe{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Probe{}, fmt.Errorf("connect: get probe: %w", err)
	}
	return probe(row)
}

func (r *Repo) LatestProbe(ctx context.Context, orgID, connectionID entity.ID) (entity.Probe, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LatestProbe(ctx, LatestProbeParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Probe{}, false, nil
	}
	if err != nil {
		return entity.Probe{}, false, fmt.Errorf("connect: latest probe: %w", err)
	}
	p, err := probe(row)
	return p, err == nil, err
}

// StartProbeRun marks a pending probe running and reports false for a probe that is already finished.
func (r *Repo) StartProbeRun(ctx context.Context, orgID, probeID entity.ID) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).StartProbeRun(ctx, StartProbeRunParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(probeID)})
	if err != nil {
		return false, fmt.Errorf("connect: start probe run: %w", err)
	}
	return n > 0, nil
}

// FinishProbe stores the outcome of a probe that is still running; a finished probe is left as it is.
func (r *Repo) FinishProbe(ctx context.Context, orgID entity.ID, p entity.Probe) error {
	caps := make([]probeCapability, 0, len(p.Capabilities))
	for _, c := range p.Capabilities {
		caps = append(caps, probeCapability{Capability: c.Name, State: string(c.State), ReasonCode: c.ReasonCode, CheckedAt: c.CheckedAt, SourceVersion: c.SourceVersion})
	}
	raw, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("connect: encode capabilities: %w", err)
	}
	err = New(postgres.Conn(ctx, r.pool)).FinishProbe(ctx, FinishProbeParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(p.ID), Status: string(p.Status), FinishedAt: tstz(p.FinishedAt),
		InstanceKey: text(p.InstanceKey), Edition: text(p.Edition), SourceVersion: text(p.SourceVersion),
		ContractVersion: text(p.ContractVersion), AccessMode: text(string(p.AccessMode)), Capabilities: raw,
		ReasonCode: text(p.ReasonCode),
	})
	if err != nil {
		return fmt.Errorf("connect: finish probe: %w", err)
	}
	return nil
}

func probe(row ConnectProbe) (entity.Probe, error) {
	var caps []probeCapability
	if err := json.Unmarshal(row.Capabilities, &caps); err != nil {
		return entity.Probe{}, fmt.Errorf("connect: decode probe capabilities: %w", err)
	}
	out := entity.Probe{
		ID: entity.ID(row.ID), ConnectionID: entity.ID(row.ConnectionID), ConfigurationVersion: int(row.ConfigurationVersion),
		Status: entity.ProbeStatus(row.Status), StartedAt: row.StartedAt, FinishedAt: timePtr(row.FinishedAt),
		InstanceKey: row.InstanceKey.String, Edition: row.Edition.String, SourceVersion: row.SourceVersion.String,
		ContractVersion: row.ContractVersion.String, AccessMode: entity.AccessMode(row.AccessMode.String),
		ReasonCode: row.ReasonCode.String, Capabilities: make([]entity.Capability, 0, len(caps)),
	}
	for _, c := range caps {
		out.Capabilities = append(out.Capabilities, entity.Capability{
			Name: c.Capability, State: entity.CapabilityState(c.State), ReasonCode: c.ReasonCode, CheckedAt: c.CheckedAt, SourceVersion: c.SourceVersion,
		})
	}
	return out, nil
}
