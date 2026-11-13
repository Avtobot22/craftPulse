// Package connect owns connections, their scopes and capabilities, and is the only caller of adapters:
// the service key, the contract version, the call timeout and the rate limit pause (TZ 10.3).
package connect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// ContractVersions are the adapter.yaml versions the core supports, the current one first (TZ 10.3).
var ContractVersions = []string{"1", "0"}

// _defaultPause holds a connection when a rate limit answer names no time to wait.
const _defaultPause = time.Minute

var ErrScopeArea = errors.New("connect: the scope is missing or already belongs to another area")

type Keyring interface {
	SealEnvelope(plaintext, aad []byte) (blob []byte, masterVersion uint32, err error)
	OpenEnvelope(blob []byte, masterVersion uint32, aad []byte) ([]byte, error)
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Repo      repo.ConnectRepo
	Adapters  repo.AdapterClient
	Addresses repo.AddressChecker
	Queue     repo.JobQueue
	Tx        repo.Transactor
	Keyring   Keyring
	Auditor   Auditor
	Clock     clock.Clock
	NewID     func() (entity.ID, error)
	// Builtin reports whether the installation runs a built-in adapter of the provider.
	Builtin func(entity.Provider) bool
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

// AdapterKeyAAD binds the encrypted key of a client adapter to its connection.
func AdapterKeyAAD(orgID, connectionID entity.ID) []byte {
	aad := append([]byte("connect.connection.adapter_key:"), orgID[:]...)
	return append(aad, connectionID[:]...)
}

// EnsureInstance finds the instance by provider and key or creates it without a connection, so a package
// of import and a connection to the same instance share its identity (TZ 12.4, 12.6).
func (u *UseCase) EnsureInstance(ctx context.Context, orgID entity.ID, provider entity.Provider, key string) (entity.SourceInstance, error) {
	var out entity.SourceInstance
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.EnsureInstance(ctx, orgID, provider, key)
		return err
	})
	return out, err
}

// State is the connection with its instance, scopes and the capabilities its activation verified.
func (u *UseCase) State(ctx context.Context, orgID, connectionID entity.ID) (entity.ConnectionState, error) {
	var out entity.ConnectionState
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		c, err := u.d.Repo.Connection(ctx, orgID, connectionID)
		if err != nil {
			return err
		}
		out.Connection = c
		if out.Instance, err = u.d.Repo.Instance(ctx, orgID, c.InstanceID); err != nil {
			return err
		}
		if out.Scopes, err = u.d.Repo.Scopes(ctx, orgID, connectionID); err != nil {
			return err
		}
		out.Capabilities, err = u.d.Repo.Capabilities(ctx, orgID, connectionID)
		return err
	})
	return out, err
}

// Scopes are the scopes of the connection with the area each one has after its first collection.
func (u *UseCase) Scopes(ctx context.Context, orgID, connectionID entity.ID) ([]entity.ConnectionScope, error) {
	var out []entity.ConnectionScope
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.Scopes(ctx, orgID, connectionID)
		return err
	})
	return out, err
}

// SetScopeArea records the area of normalize a scope was collected into; it does not move to another area.
func (u *UseCase) SetScopeArea(ctx context.Context, orgID, scopeID, areaID entity.ID) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		ok, err := u.d.Repo.SetScopeArea(ctx, orgID, scopeID, areaID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrScopeArea
		}
		return nil
	})
}

func (u *UseCase) PutCredentials(ctx context.Context, orgID, connectionID entity.ID, in entity.CredentialsInput) error {
	return u.call(ctx, orgID, connectionID, entity.OpPutCredentials, func(ctx context.Context, t entity.AdapterTarget) error {
		return u.d.Adapters.PutCredentials(ctx, t, in)
	})
}

func (u *UseCase) DeleteCredentials(ctx context.Context, orgID, connectionID entity.ID, f entity.CredentialsFilter) error {
	return u.call(ctx, orgID, connectionID, entity.OpDeleteCredentials, func(ctx context.Context, t entity.AdapterTarget) error {
		return u.d.Adapters.DeleteCredentials(ctx, t, f)
	})
}

// Probe checks the connection; the result names the newest contract version both sides support, and an
// adapter without one gets contract_unsupported, so the connection does not become working (FR-014).
func (u *UseCase) Probe(ctx context.Context, orgID, connectionID entity.ID, in entity.ProbeInput) (entity.ProbeResult, error) {
	var out entity.ProbeResult
	err := u.call(ctx, orgID, connectionID, entity.OpProbe, func(ctx context.Context, t entity.AdapterTarget) error {
		var err error
		out, err = u.d.Adapters.Probe(ctx, t, in)
		return err
	})
	if err != nil {
		return entity.ProbeResult{}, err
	}
	for _, v := range ContractVersions {
		if slices.Contains(out.ContractVersions, v) {
			out.ContractVersion = v
			return out, nil
		}
	}
	return entity.ProbeResult{}, &entity.AdapterError{
		Class:  entity.CallContractUnsupported,
		Detail: "adapter supports contract versions " + strings.Join(out.ContractVersions, ", "),
	}
}

// Fetch returns the package stream of one page; the caller closes it.
func (u *UseCase) Fetch(ctx context.Context, orgID, connectionID entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	var out io.ReadCloser
	err := u.call(ctx, orgID, connectionID, entity.OpFetch, func(ctx context.Context, t entity.AdapterTarget) error {
		var err error
		out, err = u.d.Adapters.Fetch(ctx, t, in)
		return err
	})
	return out, err
}

func (u *UseCase) Manifest(ctx context.Context, orgID, connectionID entity.ID, in entity.ManifestInput) (entity.ManifestPage, error) {
	var out entity.ManifestPage
	err := u.call(ctx, orgID, connectionID, entity.OpManifest, func(ctx context.Context, t entity.AdapterTarget) error {
		var err error
		out, err = u.d.Adapters.Manifest(ctx, t, in)
		return err
	})
	return out, err
}

func (u *UseCase) CheckAccess(ctx context.Context, orgID, connectionID entity.ID, in entity.SourceAccessInput) (entity.SourceAccessResult, error) {
	var out entity.SourceAccessResult
	err := u.call(ctx, orgID, connectionID, entity.OpCheckAccess, func(ctx context.Context, t entity.AdapterTarget) error {
		var err error
		out, err = u.d.Adapters.CheckAccess(ctx, t, in)
		return err
	})
	return out, err
}

// call reads the connection in a short transaction and calls the adapter outside any transaction
// (TZ 10.3, rule 2).
func (u *UseCase) call(ctx context.Context, orgID, connectionID entity.ID, op entity.AdapterOp, fn func(context.Context, entity.AdapterTarget) error) error {
	var c entity.Connection
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		c, err = u.d.Repo.Connection(ctx, orgID, connectionID)
		return err
	})
	if err != nil {
		return err
	}
	return u.invoke(ctx, c, op, fn)
}

// invoke refuses a call that reaches the source during a pause without a request; credentials stay in
// the adapter and are handled during a pause too. An adapter that refuses the version of the connection
// is tried with the other supported versions.
func (u *UseCase) invoke(ctx context.Context, c entity.Connection, op entity.AdapterOp, fn func(context.Context, entity.AdapterTarget) error) error {
	reachesSource := op != entity.OpPutCredentials && op != entity.OpDeleteCredentials
	if reachesSource && c.PausedUntil != nil && u.d.Clock.Now().Before(*c.PausedUntil) {
		return &entity.AdapterError{Class: entity.CallPaused, Code: entity.SourceRateLimited, RetryAt: c.PausedUntil}
	}
	t, err := u.target(c)
	if err != nil {
		return err
	}
	if !slices.Contains(ContractVersions, c.ContractVersion) {
		err = &entity.AdapterError{Class: entity.CallContractUnsupported, Detail: "connection has contract version " + c.ContractVersion}
		logFailure(ctx, c, op, err)
		return err
	}
	versions := append([]string{c.ContractVersion}, slices.DeleteFunc(slices.Clone(ContractVersions), func(v string) bool { return v == c.ContractVersion })...)
	for _, v := range versions {
		t.ContractVersion = v
		if err = fn(ctx, t); !errors.Is(err, entity.ErrContractUnsupported) {
			break
		}
	}
	if err == nil {
		return nil
	}
	logFailure(ctx, c, op, err)
	return u.settle(ctx, c, err)
}

func (u *UseCase) target(c entity.Connection) (entity.AdapterTarget, error) {
	t := entity.AdapterTarget{OrganizationID: c.OrganizationID, ConnectionID: c.ID, Kind: c.AdapterKind, Provider: c.Provider}
	if c.AdapterKind != entity.AdapterClient {
		return t, nil
	}
	key, err := u.d.Keyring.OpenEnvelope(c.AdapterKeyEnc, c.KeyVersion, AdapterKeyAAD(c.OrganizationID, c.ID))
	if err != nil {
		return entity.AdapterTarget{}, fmt.Errorf("connect: open adapter key: %w", err)
	}
	t.URL, t.Key = c.AdapterURL, entity.Secret(key)
	return t, nil
}

// settle applies what an answer means for the whole connection: rate_limited pauses every call of it,
// auth moves it to access_lost with the date of the refusal (TZ 10.7).
func (u *UseCase) settle(ctx context.Context, c entity.Connection, err error) error {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	if !ok || ae.Class != entity.CallSource {
		return err
	}
	now := u.d.Clock.Now()
	switch ae.Code {
	case entity.SourceRateLimited:
		until := now.Add(_defaultPause)
		switch {
		case ae.RetryAfter > 0:
			until = now.Add(ae.RetryAfter)
		case ae.RetryAt != nil && ae.RetryAt.After(now):
			until = *ae.RetryAt
		}
		ae.RetryAt = &until
		if perr := u.d.Tx.InOrgTx(ctx, c.OrganizationID, func(ctx context.Context) error {
			return u.d.Repo.PauseConnection(ctx, c.OrganizationID, c.ID, until)
		}); perr != nil {
			return errors.Join(err, perr)
		}
	case entity.SourceAuth:
		if lerr := u.d.Tx.InOrgTx(ctx, c.OrganizationID, func(ctx context.Context) error {
			changed, err := u.d.Repo.MarkAccessLost(ctx, c.OrganizationID, c.ID, now)
			if err != nil || !changed {
				return err
			}
			return u.d.Auditor.Record(ctx, entity.AuditEvent{
				OrganizationID: c.OrganizationID, OccurredAt: now, ActorKind: entity.ActorSystem, Operation: "connectionAccessLost",
				ObjectKind: "connection", ObjectID: c.ID, Outcome: entity.AuditSucceeded, ReasonCode: entity.SourceAuth,
				CorrelationID: entity.ID(logger.CorrelationID(ctx)),
			})
		}); lerr != nil {
			return errors.Join(err, lerr)
		}
	}
	return err
}

// logFailure writes every refused call with its connection, operation and class (NFR-018).
func logFailure(ctx context.Context, c entity.Connection, op entity.AdapterOp, err error) {
	ev := zerolog.Ctx(ctx).Warn().Str("connection_id", c.ID.String()).Str("operation", string(op))
	if ae, ok := errors.AsType[*entity.AdapterError](err); ok {
		ev = ev.Str("class", string(ae.Class)).Str("reason", ae.Code).Str("contract_version", c.ContractVersion)
	}
	ev.Err(err).Msg("adapter call failed")
}
