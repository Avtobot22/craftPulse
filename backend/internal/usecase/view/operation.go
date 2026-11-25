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
	_resourceSeries      = "series"
	_actionManageConnect = "connections.manage"
	_actionReadReport    = "report.read"
)

// Sync is the sync module: a run of collection and where it is.
type Sync interface {
	RequestRun(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, corr entity.ID) error
	RunStatus(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, since time.Time) (entity.SyncRunStatus, error)
}

// Series is the calc module: the team of a series, its manual refresh in the transaction of the descriptor and
// where the refresh is.
type Series interface {
	SeriesTeam(ctx context.Context, orgID, seriesID entity.ID) (entity.ID, error)
	RequestRefresh(ctx context.Context, orgID, seriesID, corr entity.ID) (entity.SeriesRefresh, error)
	RefreshStatus(ctx context.Context, orgID entity.ID, r entity.SeriesRefresh) (entity.RefreshStatus, error)
}

// Authorizer is the access module: the action of the operation is checked again on every read.
type Authorizer interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
}

type Deps struct {
	Summaries        Summaries
	Tasks            TaskDeps
	ReviewCalculator ReviewCalculator
	ProjectAreas     ProjectAreas
	Code             CodeFacts
	SourceRights     SourceRights
	CodeLinks        CodeLinks
	RunCalculator    RunCalculator
	PipelineChecks   PipelineChecks
	Repo             repo.ViewRepo
	Tx               repo.Transactor
	Sync             Sync
	Series           Series
	Reports          Reports
	Instances        Instances
	Access           Authorizer
	Clock            clock.Clock
	NewID            func() (entity.ID, error)

	SummaryFindingRights SummaryFindingRights
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

// refreshParameters is what a series_refresh keeps: the revision number after which a revision answers it and
// the correlation ID its jobs log with.
type refreshParameters struct {
	AfterRevisionNo int       `json:"after_revision_no"`
	CorrelationID   entity.ID `json:"correlation_id"`
}

// RequestSeriesRefresh creates the descriptor of a manual refresh of a series and queues the refresh in one
// transaction (FR-023). A series of a team the user may not read is not found. A repeat while the refresh is
// active returns its descriptor; calc does not double a chain the series already builds.
func (u *UseCase) RequestSeriesRefresh(ctx context.Context, s entity.Subject, seriesID entity.ID) (entity.Operation, error) {
	org := s.OrganizationID
	team, err := u.d.Series.SeriesTeam(ctx, org, seriesID)
	if err != nil {
		return entity.Operation{}, err
	}
	_, err = u.d.Access.Authorize(ctx, s, entity.AccessRequest{
		Operation: "refreshSeries", Action: _actionReadReport, Area: entity.AreaRef{Kind: entity.AreaTeam, ID: team},
	})
	if errors.Is(err, entity.ErrForbidden) || errors.Is(err, entity.ErrNotFound) {
		return entity.Operation{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Operation{}, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.Operation{}, err
	}
	hash := sha256.Sum256(seriesID[:])
	corr := entity.ID(logger.CorrelationID(ctx))
	now := u.d.Clock.Now()
	op := entity.Operation{
		ID: id, OrganizationID: org, RequestedBy: s.UserID, Kind: entity.OperationSeriesRefresh, ResourceKind: _resourceSeries,
		ResourceID: seriesID, Action: _actionReadReport, RequestHash: hash[:], Status: entity.OperationQueued, CreatedAt: now, UpdatedAt: now,
	}
	var out entity.Operation
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		active, found, err := u.activeOf(ctx, op)
		if err != nil || found {
			out = active
			return err
		}
		r, err := u.d.Series.RequestRefresh(ctx, org, seriesID, corr)
		if err != nil {
			return err
		}
		if op.Parameters, err = json.Marshal(refreshParameters{AfterRevisionNo: r.AfterRevisionNo, CorrelationID: corr}); err != nil {
			return fmt.Errorf("view: encode parameters: %w", err)
		}
		created, err := u.d.Repo.CreateOperation(ctx, op)
		if err != nil || created {
			out = op
			return err
		}
		if out, found, err = u.activeOf(ctx, op); err == nil && !found {
			err = entity.ErrStateConflict
		}
		return err
	})
	return out, err
}

// activeOf is the active descriptor of the user for the resource of op, refreshed; false when there is none.
func (u *UseCase) activeOf(ctx context.Context, op entity.Operation) (entity.Operation, bool, error) {
	active, found, err := u.d.Repo.ActiveOperation(ctx, op)
	if err != nil || !found {
		return entity.Operation{}, false, err
	}
	if active, err = u.refresh(ctx, active); err != nil {
		return entity.Operation{}, false, err
	}
	return active, !active.Done(), nil
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
	// The module reads the status in its own transaction, so this one does not hold a connection while it waits
	// for another: polls of a full pool would block each other until the idle transactions are cut.
	next, changed, err := u.advance(ctx, op)
	if err != nil || !changed {
		return next, err
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error { return u.d.Repo.UpdateOperation(ctx, next) })
	return next, err
}

// refresh takes the status of an active run from its module and records it in the caller's transaction.
func (u *UseCase) refresh(ctx context.Context, op entity.Operation) (entity.Operation, error) {
	next, changed, err := u.advance(ctx, op)
	if err != nil || !changed {
		return next, err
	}
	return next, u.d.Repo.UpdateOperation(ctx, next)
}

// advance takes the status of an active run from its module; changed tells whether the descriptor is to be recorded.
func (u *UseCase) advance(ctx context.Context, op entity.Operation) (entity.Operation, bool, error) {
	if op.Kind == entity.OperationSeriesRefresh {
		return u.advanceSeries(ctx, op)
	}
	return u.advanceSync(ctx, op)
}

// advanceSeries takes the status of a refresh from calc: a succeeded one leads to its exact new revision.
func (u *UseCase) advanceSeries(ctx context.Context, op entity.Operation) (entity.Operation, bool, error) {
	var p refreshParameters
	if err := json.Unmarshal(op.Parameters, &p); err != nil {
		return entity.Operation{}, false, fmt.Errorf("view: decode parameters: %w", err)
	}
	st, err := u.d.Series.RefreshStatus(ctx, op.OrganizationID, entity.SeriesRefresh{SeriesID: op.ResourceID, AfterRevisionNo: p.AfterRevisionNo})
	if err != nil {
		return entity.Operation{}, false, err
	}
	if st.Status == op.Status {
		return op, false, nil
	}
	now := u.d.Clock.Now()
	op.Status, op.UpdatedAt = st.Status, now
	switch st.Status {
	case entity.OperationSucceeded:
		op.CompletedAt, op.ResultURI = &now, "/api/v1/revisions/"+st.RevisionID.String()
	case entity.OperationFailed:
		corr := p.CorrelationID
		op.CompletedAt, op.ErrorCode, op.CorrelationID = &now, entity.OperationCalculationFailed, &corr
	case entity.OperationQueued, entity.OperationRunning:
	}
	return op, true, nil
}

// advanceSync takes the status of an active run from sync; a finished run gets its result or the safe class of its
// failure with the correlation ID of its jobs.
func (u *UseCase) advanceSync(ctx context.Context, op entity.Operation) (entity.Operation, bool, error) {
	var p syncParameters
	if err := json.Unmarshal(op.Parameters, &p); err != nil {
		return entity.Operation{}, false, fmt.Errorf("view: decode parameters: %w", err)
	}
	st, err := u.d.Sync.RunStatus(ctx, op.OrganizationID, op.ResourceID, p.SyncRunRequest, op.CreatedAt)
	if err != nil {
		return entity.Operation{}, false, err
	}
	if st.Status == op.Status {
		return op, false, nil
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
	return op, true, nil
}

// _purgeBatch bounds the rows one transaction of a purge deletes.
const _purgeBatch = 1000

// PurgeOperations deletes the operations completed more than a day ago: Operation already hides them, and an active
// one is never deleted (TZ 12.9). Each portion commits on its own.
func (u *UseCase) PurgeOperations(ctx context.Context, orgID entity.ID) error {
	before := u.d.Clock.Now().Add(-_keepFor)
	for {
		var n int64
		err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
			var err error
			n, err = u.d.Repo.PurgeOperations(ctx, orgID, before, _purgeBatch)
			return err
		})
		if err != nil || n < _purgeBatch {
			return err
		}
	}
}
