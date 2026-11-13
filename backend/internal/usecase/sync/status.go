package sync

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Status is the state of collection of a connection by data kind (FR-020): the last attempt and its error,
// the last success, the lag, the completeness and the last reconciliation stay apart.
func (u *UseCase) Status(ctx context.Context, orgID, connectionID entity.ID) (entity.SyncStatus, error) {
	cs, err := u.d.Connect.State(ctx, orgID, connectionID)
	if err != nil {
		return entity.SyncStatus{}, err
	}
	out := entity.SyncStatus{ConnectionID: connectionID, ConnectionStatus: cs.Connection.Status, PausedUntil: cs.Connection.PausedUntil}
	var states []entity.SyncState
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		if states, err = u.d.Repo.States(ctx, orgID, connectionID); err != nil {
			return err
		}
		last, found, err := u.d.Repo.LastReconcile(ctx, orgID, connectionID, "")
		if found {
			out.LastReconcile = &last
		}
		return err
	})
	if err != nil {
		return entity.SyncStatus{}, err
	}
	now := u.d.Clock.Now()
	for i := range states {
		st := states[i]
		item := entity.SyncDataKindStatus{SyncState: st, FailedPagePresent: st.FailedCursor != ""}
		if st.Watermark != nil {
			lag := int64(max(now.Sub(*st.Watermark), 0) / time.Second)
			item.LagSeconds = &lag
		}
		for _, schema := range _schemas[st.DataKind] {
			for _, m := range u.d.Ingest.AffectedMetrics(schema) {
				if !slices.Contains(item.AffectedMetrics, m) {
					item.AffectedMetrics = append(item.AffectedMetrics, m)
				}
			}
		}
		out.DataKinds = append(out.DataKinds, item)
	}
	return out, nil
}

// LastSuccess is the last success of every connection: the earliest of its data kinds, nil while one of
// them has none, so a connection is as fresh as its stalest data kind. A data kind the adapter does not
// serve is left out.
func (u *UseCase) LastSuccess(ctx context.Context, orgID entity.ID) (map[entity.ID]*time.Time, error) {
	var states []entity.SyncState
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		states, err = u.d.Repo.States(ctx, orgID, entity.ID{})
		return err
	})
	if err != nil {
		return nil, err
	}
	out := map[entity.ID]*time.Time{}
	for i := range states {
		st := &states[i]
		if unsupported(st) {
			continue
		}
		prev, seen := out[st.ConnectionID]
		switch {
		case !seen:
			out[st.ConnectionID] = st.LastSuccessAt
		case prev == nil || st.LastSuccessAt == nil:
			out[st.ConnectionID] = nil
		case st.LastSuccessAt.Before(*prev):
			out[st.ConnectionID] = st.LastSuccessAt
		}
	}
	return out, nil
}

// RequestRun queues a collection from the saved checkpoint or a reconciliation in the caller's transaction.
// A request while an attempt of the data kind is active adds nothing (NFR-006); a connection without access
// waits for new credentials.
func (u *UseCase) RequestRun(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, corr entity.ID) error {
	if err := validRun(req); err != nil {
		return err
	}
	cs, err := u.d.Connect.State(ctx, orgID, connectionID)
	if err != nil {
		return err
	}
	switch cs.Connection.Status {
	case entity.ConnectionActive:
	case entity.ConnectionDeleted:
		return entity.ErrNotFound
	case entity.ConnectionDraft, entity.ConnectionAccessLost:
		return entity.ErrStateConflict
	}
	now := u.d.Clock.Now()
	if req.Kind == entity.SyncReconcile {
		return u.queueReconcile(ctx, orgID, connectionID, req.DataKind, corr, 0, now)
	}
	st, found, err := u.d.Repo.State(ctx, orgID, connectionID, req.DataKind)
	if err != nil {
		return err
	}
	busy, err := u.busy(ctx, orgID, connectionID, st, found)
	if err != nil || busy {
		return err
	}
	if st.Phase == entity.SyncStopped {
		st.Phase = entity.SyncBackfill
		if st.LastSuccessAt != nil {
			st.Phase = entity.SyncIncremental
		}
		if err := u.d.Repo.SaveState(ctx, orgID, st); err != nil {
			return err
		}
	}
	job := entity.SyncPageJob{OrganizationID: orgID, ConnectionID: connectionID, DataKind: req.DataKind, CorrelationID: corr}
	return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}

func validRun(req entity.SyncRunRequest) error {
	var fields []entity.FieldError
	if req.Kind != entity.SyncResume && req.Kind != entity.SyncReconcile {
		fields = append(fields, entity.FieldError{Field: "/kind", Code: entity.FieldNotAllowedValue})
	}
	if _, ok := _schemas[req.DataKind]; !ok {
		fields = append(fields, entity.FieldError{Field: "/data_kind", Code: entity.FieldNotAllowedValue})
	}
	if len(fields) > 0 {
		return &entity.ValidationError{Fields: fields}
	}
	return nil
}

// busy tells an active attempt of a data kind: a held lease, a round in the middle, a page waiting for its
// repeat, or the first page of a connection that has no state yet. A stopped collection waits for a request.
func (u *UseCase) busy(ctx context.Context, org, conn entity.ID, st entity.SyncState, found bool) (bool, error) {
	held, err := u.d.Repo.LeaseHeld(ctx, org, conn, u.d.Clock.Now())
	if err != nil || held || !found {
		return held || !found, err
	}
	if st.Phase == entity.SyncStopped {
		return false, nil
	}
	p, err := decode(st.Cursor)
	if err != nil {
		return false, err
	}
	return !p.NewRound || st.FailedCursor != "", nil
}

// RunStatus tells where a run requested at since is. A reconciliation is the latest one of the data kind;
// a collection is done when the round it continued ended, and failed when the collection stopped.
func (u *UseCase) RunStatus(ctx context.Context, orgID, connectionID entity.ID, req entity.SyncRunRequest, since time.Time) (entity.SyncRunStatus, error) {
	var out entity.SyncRunStatus
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		if req.Kind == entity.SyncReconcile {
			run, found, err := u.d.Repo.LastReconcile(ctx, orgID, connectionID, req.DataKind)
			if err != nil || !found {
				out.Status = entity.OperationQueued
				return err
			}
			out = reconcileStatus(run)
			return nil
		}
		st, found, err := u.d.Repo.State(ctx, orgID, connectionID, req.DataKind)
		if err != nil {
			return err
		}
		busy, err := u.busy(ctx, orgID, connectionID, st, found)
		if err != nil {
			return err
		}
		out = collectionStatus(st, found, busy, since)
		return nil
	})
	return out, err
}

func reconcileStatus(run entity.ReconcileRun) entity.SyncRunStatus {
	switch run.Outcome {
	case entity.ReconcileRunning:
		return entity.SyncRunStatus{Status: entity.OperationRunning}
	case entity.ReconcileFailed:
		return entity.SyncRunStatus{Status: entity.OperationFailed, ErrorCode: safeCode(run.ReasonCode), FinishedAt: run.FinishedAt}
	}
	return entity.SyncRunStatus{Status: entity.OperationSucceeded, FinishedAt: run.FinishedAt}
}

func collectionStatus(st entity.SyncState, found, busy bool, since time.Time) entity.SyncRunStatus {
	switch {
	case !found || st.LastAttemptAt == nil || st.LastAttemptAt.Before(since):
		return entity.SyncRunStatus{Status: entity.OperationQueued}
	case st.Phase == entity.SyncStopped:
		return entity.SyncRunStatus{Status: entity.OperationFailed, ErrorCode: safeCode(st.LastErrorCode), FinishedAt: st.LastAttemptAt}
	case busy:
		return entity.SyncRunStatus{Status: entity.OperationRunning}
	}
	return entity.SyncRunStatus{Status: entity.OperationSucceeded, FinishedAt: st.LastAttemptAt}
}

// safeCode is the class of a failure a user may see; the cause stays in the logs.
func safeCode(code string) string {
	switch code {
	case entity.SourceAuth, "permission_missing":
		return entity.OperationAccessLost
	}
	return entity.OperationSourceUnavailable
}

// unsupported tells a data kind the adapter does not serve: its collection stopped for good.
func unsupported(st *entity.SyncState) bool {
	return st.Phase == entity.SyncStopped && (st.LastErrorCode == string(entity.CallSource)+":not_supported" ||
		strings.HasPrefix(st.LastErrorCode, string(entity.CallContractUnsupported)+":"))
}
