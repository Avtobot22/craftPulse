// Package view serves what users read; this part keeps the public descriptors of the runs they ask for
// (TZ 5.2, 12.5.8): a descriptor shows a run without its job in the queue.
package view

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	// _keepFor is how long a finished operation can be read.
	_keepFor             = 24 * time.Hour
	_resourceConnection  = "connection"
	_actionManageConnect = "connections.manage"
)

// Sync is the sync module: a run of collection and where it is.
type Sync interface {
	RequestRun(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, corr entity.ID) error
	RunStatus(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, since time.Time) (entity.SyncRunStatus, error)
}

// Authorizer is the access module: the action of the operation is checked again on every read.
type Authorizer interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
}

type Deps struct {
	Repo   repo.ViewRepo
	Tx     repo.Transactor
	Sync   Sync
	Access Authorizer
	Clock  clock.Clock
	NewID  func() (entity.ID, error)
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

// syncParameters is what a sync_run keeps of its request: the request itself and the correlation ID its
// jobs log with.
type syncParameters struct {
	entity.SyncRunRequest
	CorrelationID entity.ID `json:"correlation_id"`
}

// RequestSyncRun creates the descriptor of a run and queues the run in one transaction. A repeat of the same
// request while the run is active returns its descriptor; another request for the connection meanwhile is a
// conflict (TZ 12.5.8).
func (u *UseCase) RequestSyncRun(ctx context.Context, s entity.Subject, connectionID entity.ID, req entity.SyncRunRequest) (entity.Operation, error) {
	request, err := json.Marshal(req)
	if err != nil {
		return entity.Operation{}, fmt.Errorf("view: encode request: %w", err)
	}
	hash := sha256.Sum256(request)
	corr := entity.ID(logger.CorrelationID(ctx))
	params, err := json.Marshal(syncParameters{SyncRunRequest: req, CorrelationID: corr})
	if err != nil {
		return entity.Operation{}, fmt.Errorf("view: encode parameters: %w", err)
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.Operation{}, err
	}
	now := u.d.Clock.Now()
	op := entity.Operation{
		ID: id, OrganizationID: s.OrganizationID, RequestedBy: s.UserID, Kind: entity.OperationSyncRun, ResourceKind: _resourceConnection,
		ResourceID: connectionID, Action: _actionManageConnect, Parameters: params, RequestHash: hash[:], Status: entity.OperationQueued,
		CreatedAt: now, UpdatedAt: now,
	}
	var out entity.Operation
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		active, found, err := u.d.Repo.ActiveOperation(ctx, op)
		if err != nil {
			return err
		}
		if found {
			if active, err = u.refresh(ctx, active); err != nil {
				return err
			}
			if !active.Done() {
				if !bytes.Equal(active.RequestHash, op.RequestHash) {
					return entity.ErrStateConflict
				}
				out = active
				return nil
			}
		}
		created, err := u.d.Repo.CreateOperation(ctx, op)
		if err != nil {
			return err
		}
		if !created {
			return entity.ErrStateConflict
		}
		out = op
		return u.d.Sync.RequestRun(ctx, s.OrganizationID, connectionID, req, corr)
	})
	return out, err
}

// Operation reads an operation of its initiator with the action of the run checked again. Another user's
// operation, one finished more than 24 h ago and one whose resource is no longer allowed are all not found.
func (u *UseCase) Operation(ctx context.Context, s entity.Subject, id entity.ID) (entity.Operation, error) {
	var op entity.Operation
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var (
			found bool
			err   error
		)
		op, found, err = u.d.Repo.Operation(ctx, s.OrganizationID, id)
		if err != nil {
			return err
		}
		if !found || op.RequestedBy != s.UserID || (op.CompletedAt != nil && u.d.Clock.Now().Sub(*op.CompletedAt) > _keepFor) {
			return entity.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return entity.Operation{}, err
	}
	_, err = u.d.Access.Authorize(ctx, s, entity.AccessRequest{Operation: "getBackgroundOperation", Action: op.Action})
	if errors.Is(err, entity.ErrForbidden) || errors.Is(err, entity.ErrNotFound) {
		return entity.Operation{}, entity.ErrNotFound
	}
	if err != nil || op.Done() {
		return op, err
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		op, err = u.refresh(ctx, op)
		return err
	})
	return op, err
}

// refresh takes the status of an active run from sync and records it; a finished run gets its result or
// the safe class of its failure with the correlation ID of its jobs.
func (u *UseCase) refresh(ctx context.Context, op entity.Operation) (entity.Operation, error) {
	var p syncParameters
	if err := json.Unmarshal(op.Parameters, &p); err != nil {
		return entity.Operation{}, fmt.Errorf("view: decode parameters: %w", err)
	}
	st, err := u.d.Sync.RunStatus(ctx, op.OrganizationID, op.ResourceID, p.SyncRunRequest, op.CreatedAt)
	if err != nil {
		return entity.Operation{}, err
	}
	if st.Status == op.Status {
		return op, nil
	}
	now := u.d.Clock.Now()
	op.Status, op.UpdatedAt = st.Status, now
	switch st.Status {
	case entity.OperationSucceeded:
		op.CompletedAt, op.ResultURI = &now, "/api/v1/connections/"+op.ResourceID.String()+"/sync-status"
	case entity.OperationFailed:
		corr := p.CorrelationID
		op.CompletedAt, op.ErrorCode, op.CorrelationID = &now, st.ErrorCode, &corr
	case entity.OperationQueued, entity.OperationRunning:
	}
	return op, u.d.Repo.UpdateOperation(ctx, op)
}
