package connect

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	// _probeKept is how long a finished probe that was not activated stays readable (TZ 12.9).
	_probeKept   = 24 * time.Hour
	_preparePoll = 5 * time.Second
)

// StartProbe creates a probe and its job in one transaction. A start while a probe of the same settings
// runs returns that probe; a running probe of older settings gives ErrStateConflict.
func (u *UseCase) StartProbe(ctx context.Context, s entity.Subject, connectionID entity.ID) (entity.Probe, error) {
	var out entity.Probe
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		c, err := u.d.Repo.LockConnection(ctx, s.OrganizationID, connectionID)
		if err != nil {
			return err
		}
		if c.Status == entity.ConnectionDeleted {
			return entity.ErrNotFound
		}
		running, ok, err := u.d.Repo.RunningProbe(ctx, s.OrganizationID, connectionID)
		if err != nil {
			return err
		}
		if ok {
			if running.ConfigurationVersion != c.RowVersion {
				return entity.ErrStateConflict
			}
			out = running
			return nil
		}
		if out, err = u.d.Repo.CreateProbe(ctx, s.OrganizationID, connectionID, c.RowVersion, u.d.Clock.Now()); err != nil {
			return err
		}
		job := entity.ConnectProbeJob{OrganizationID: s.OrganizationID, ConnectionID: connectionID, ProbeID: out.ID, CorrelationID: entity.ID(logger.CorrelationID(ctx))}
		if err := u.d.Queue.Enqueue(ctx, job, entity.QueueInteractive); err != nil {
			return err
		}
		return u.audit(ctx, s, "startConnectionProbe", connectionID, strconv.Itoa(c.RowVersion), "")
	})
	return out, err
}

// GetProbe hides a probe of a deleted connection and a finished probe older than a day that the
// connection does not point to, the same way as a probe that never existed.
func (u *UseCase) GetProbe(ctx context.Context, s entity.Subject, connectionID, probeID entity.ID) (entity.Probe, error) {
	var out entity.Probe
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		c, err := u.d.Repo.Connection(ctx, s.OrganizationID, connectionID)
		if err != nil {
			return err
		}
		if c.Status == entity.ConnectionDeleted {
			return entity.ErrNotFound
		}
		if out, err = u.d.Repo.Probe(ctx, s.OrganizationID, connectionID, probeID); err != nil {
			return err
		}
		activated := c.ActivatedProbeID != nil && *c.ActivatedProbeID == probeID
		if out.FinishedAt != nil && u.d.Clock.Now().Sub(*out.FinishedAt) > _probeKept && !activated {
			return entity.ErrNotFound
		}
		return nil
	})
	return out, err
}

// RunProbe is the job of a probe: the adapter is called outside a transaction, and the outcome is stored
// only if the settings did not change meanwhile (configuration_changed otherwise). A rate limit, a pause
// or preparing returns *entity.RetryLaterError, so the job waits without spending an attempt.
func (u *UseCase) RunProbe(ctx context.Context, orgID, connectionID, probeID entity.ID) error {
	var (
		p     entity.Probe
		c     entity.Connection
		input entity.ProbeInput
		live  bool
	)
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if live, err = u.d.Repo.StartProbeRun(ctx, orgID, probeID); err != nil || !live {
			return err
		}
		if p, err = u.d.Repo.Probe(ctx, orgID, connectionID, probeID); err != nil {
			return err
		}
		if c, err = u.d.Repo.Connection(ctx, orgID, connectionID); err != nil {
			return err
		}
		inst, err := u.d.Repo.Instance(ctx, orgID, c.InstanceID)
		if err != nil {
			return err
		}
		scopes, err := u.d.Repo.Scopes(ctx, orgID, connectionID)
		if err != nil {
			return err
		}
		input.BaseURL = inst.BaseURL
		for i := range scopes {
			if sc := &scopes[i]; sc.Active {
				input.Scopes = append(input.Scopes, entity.ScopeRef{Kind: sc.Kind, NativeScopeID: sc.NativeScopeID})
			}
		}
		return nil
	})
	if err != nil || !live {
		return err
	}
	if c.Status == entity.ConnectionDeleted || c.RowVersion != p.ConfigurationVersion {
		return u.finishProbe(ctx, orgID, p, entity.ProbeResult{}, entity.ProbeConfigurationChanged)
	}
	res, err := u.Probe(ctx, orgID, connectionID, input)
	if err == nil {
		return u.finishProbe(ctx, orgID, p, res, "")
	}
	if at, ok := u.retryAt(err); ok {
		return &entity.RetryLaterError{At: at, Err: err}
	}
	reason, ok := probeReason(err)
	if !ok {
		return err
	}
	return u.finishProbe(ctx, orgID, p, entity.ProbeResult{}, reason)
}

func (u *UseCase) finishProbe(ctx context.Context, orgID entity.ID, p entity.Probe, res entity.ProbeResult, reason string) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		c, err := u.d.Repo.LockConnection(ctx, orgID, p.ConnectionID)
		if err != nil {
			return err
		}
		now := u.d.Clock.Now()
		p.FinishedAt = &now
		switch {
		case c.Status == entity.ConnectionDeleted || c.RowVersion != p.ConfigurationVersion:
			p.Status, p.ReasonCode = entity.ProbeFailed, entity.ProbeConfigurationChanged
		case reason != "":
			p.Status, p.ReasonCode = entity.ProbeFailed, reason
		default:
			p.Status = entity.ProbeSucceeded
			p.InstanceKey, p.Edition, p.SourceVersion = res.InstanceKey, res.Edition, res.Version
			p.ContractVersion, p.AccessMode, p.Capabilities = res.ContractVersion, res.AccessMode, complete(res.Capabilities, now)
		}
		return u.d.Repo.FinishProbe(ctx, orgID, p)
	})
}

func (u *UseCase) retryAt(err error) (time.Time, bool) {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	if !ok {
		return time.Time{}, false
	}
	now := u.d.Clock.Now()
	switch {
	case ae.Class == entity.CallPaused, ae.Class == entity.CallSource && ae.Code == entity.SourceRateLimited:
		if ae.RetryAt != nil {
			return *ae.RetryAt, true
		}
		return now.Add(_defaultPause), true
	case ae.Class == entity.CallSource && ae.Code == entity.SourcePreparing:
		switch {
		case ae.RetryAfter > 0:
			return now.Add(ae.RetryAfter), true
		case ae.RetryAt != nil:
			return *ae.RetryAt, true
		}
		return now.Add(_preparePoll), true
	}
	return time.Time{}, false
}

// probeReason names a failed probe by the class of the failure (ConnectionProbe.reason_code); an error
// that is not an answer of the call is not a probe outcome and is retried by the job.
func probeReason(err error) (string, bool) {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	if !ok {
		return "", false
	}
	switch ae.Class {
	case entity.CallContractUnsupported:
		return entity.ProbeContractUnsupported, true
	case entity.CallAdapterUnavailable:
		switch ae.Code {
		case entity.AdapterTimeout:
			return entity.ProbeTimeout, true
		case entity.AdapterCertificateUntrusted:
			return entity.ProbeCertificateUntrusted, true
		case entity.AdapterOutsideNetworkScope:
			return entity.ProbeOutsideNetworkScope, true
		}
	case entity.CallSource:
		switch ae.Code {
		case "unsupported_edition":
			return entity.ProbeEditionMismatch, true
		case entity.SourceAuth, "permission_missing":
			return entity.ProbeAuthFailed, true
		case entity.SourceTimeout:
			return entity.ProbeTimeout, true
		case "unsupported_contract_version":
			return entity.ProbeContractUnsupported, true
		}
	case entity.CallPaused:
	}
	return entity.ProbeAdapterUnavailable, true
}

// Activate confirms the capabilities of the latest successful probe of the current settings (UC-04,
// UI-17). In one transaction the connection moves to the instance the probe named, becomes active,
// takes the probe's capabilities and queues the first load of each of its data kinds. A repeat of the
// same activation returns the connection before If-Match is compared and queues nothing.
func (u *UseCase) Activate(ctx context.Context, s entity.Subject, connectionID entity.ID, baseVersion int, probeID entity.ID) (entity.ConnectionState, error) {
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		c, err := u.d.Repo.LockConnection(ctx, s.OrganizationID, connectionID)
		if err != nil {
			return err
		}
		switch {
		case c.Status == entity.ConnectionDeleted:
			return entity.ErrNotFound
		case c.Status == entity.ConnectionActive && c.ActivatedProbeID != nil && *c.ActivatedProbeID == probeID && c.RowVersion == c.ActivatedRowVersion:
			return nil
		case c.RowVersion != baseVersion:
			return entity.ErrVersionConflict
		case c.Status != entity.ConnectionDraft && c.Status != entity.ConnectionAccessLost:
			return entity.ErrStateConflict
		}
		p, err := u.activatable(ctx, c, probeID)
		if err != nil {
			return err
		}
		return u.applyActivation(ctx, s, c, p)
	})
	if err != nil {
		return entity.ConnectionState{}, err
	}
	return u.State(ctx, s.OrganizationID, connectionID)
}

// activatable is the probe when it is the latest one of the connection, succeeded and ran on the
// current settings; anything else is a state conflict.
func (u *UseCase) activatable(ctx context.Context, c entity.Connection, probeID entity.ID) (entity.Probe, error) {
	p, err := u.d.Repo.Probe(ctx, c.OrganizationID, c.ID, probeID)
	if errors.Is(err, entity.ErrNotFound) {
		return entity.Probe{}, entity.ErrStateConflict
	}
	if err != nil {
		return entity.Probe{}, err
	}
	latest, _, err := u.d.Repo.LatestProbe(ctx, c.OrganizationID, c.ID)
	if err != nil {
		return entity.Probe{}, err
	}
	if latest.ID != p.ID || p.Status != entity.ProbeSucceeded || p.ConfigurationVersion != c.RowVersion {
		return entity.Probe{}, entity.ErrStateConflict
	}
	return p, nil
}

func (u *UseCase) applyActivation(ctx context.Context, s entity.Subject, c entity.Connection, p entity.Probe) error {
	org, now := s.OrganizationID, u.d.Clock.Now()
	current, err := u.d.Repo.Instance(ctx, org, c.InstanceID)
	if err != nil {
		return err
	}
	inst, err := u.d.Repo.EnsureInstance(ctx, org, current.Provider, p.InstanceKey)
	if err != nil {
		return err
	}
	inst.BaseURL, inst.Edition, inst.SourceVersion, inst.VerifiedAt = current.BaseURL, p.Edition, p.SourceVersion, &now
	if err := u.d.Repo.VerifyInstance(ctx, inst); err != nil {
		return err
	}
	if err := u.d.Repo.ReplaceCapabilities(ctx, org, c.ID, p.Capabilities); err != nil {
		return err
	}
	base := c.RowVersion
	c.InstanceID, c.Status, c.AccessLostAt = inst.ID, entity.ConnectionActive, nil
	c.RowVersion++
	c.ActivatedProbeID, c.ActivatedRowVersion = &p.ID, c.RowVersion
	c.ContractVersion = p.ContractVersion
	if p.AccessMode != "" {
		c.AccessMode = p.AccessMode
	}
	if err := u.d.Repo.SaveConnection(ctx, c); err != nil {
		return err
	}
	scopes, err := u.d.Repo.Scopes(ctx, org, c.ID)
	if err != nil {
		return err
	}
	corr := entity.ID(logger.CorrelationID(ctx))
	for _, kind := range dataKinds(c, scopes) {
		job := entity.SyncPageJob{OrganizationID: org, ConnectionID: c.ID, DataKind: kind, CorrelationID: corr}
		if err := u.d.Queue.Enqueue(ctx, job, entity.QueueBulk); err != nil {
			return err
		}
	}
	return u.audit(ctx, s, "activateConnection", c.ID, strconv.Itoa(base), strconv.Itoa(c.RowVersion))
}

// complete keeps the capabilities the adapter checked, unknown names dropped, and marks the rest missing:
// only what was checked becomes working (FR-014).
func complete(checked []entity.Capability, at time.Time) []entity.Capability {
	out := make([]entity.Capability, 0, len(entity.Capabilities))
	for _, name := range entity.Capabilities {
		i := slices.IndexFunc(checked, func(c entity.Capability) bool { return c.Name == name })
		if i >= 0 {
			out = append(out, checked[i])
			continue
		}
		out = append(out, entity.Capability{Name: name, State: entity.CapabilityMissing, ReasonCode: entity.CapabilityNotChecked, CheckedAt: at})
	}
	return out
}
