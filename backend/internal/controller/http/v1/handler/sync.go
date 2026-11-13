package handler

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _pollAfter is how soon a client asks again about an active operation.
const _pollAfter = 3

// _errorCodes are the codes of the API for the reasons sync records.
var _errorCodes = map[string]string{entity.SourceAuth: "unauthorized", "permission_missing": "forbidden"}

func (h *connections) GetSyncStatus(ctx context.Context, req openapi.GetSyncStatusRequestObject) (openapi.GetSyncStatusResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	st, err := h.sync.Status(ctx, s.OrganizationID, entity.ID(req.ConnectionId))
	if err != nil {
		return nil, err
	}
	out := openapi.SyncStatus{
		ConnectionId: req.ConnectionId, ConnectionStatus: openapi.SyncStatusConnectionStatus(st.ConnectionStatus), PausedUntil: st.PausedUntil,
		DataKinds: make([]openapi.SyncDataKindStatus, 0, len(st.DataKinds)),
	}
	for i := range st.DataKinds {
		out.DataKinds = append(out.DataKinds, dataKindStatus(&st.DataKinds[i]))
	}
	if r := st.LastReconcile; r != nil {
		out.LastReconcile = &struct {
			ChangedCount  *int                                   `json:"changed_count"`
			ExpectedCount *int                                   `json:"expected_count"`
			FinishedAt    *time.Time                             `json:"finished_at"`
			MissingCount  *int                                   `json:"missing_count"`
			ObservedCount *int                                   `json:"observed_count"`
			Outcome       openapi.SyncStatusLastReconcileOutcome `json:"outcome"`
			StartedAt     time.Time                              `json:"started_at"`
		}{
			ChangedCount: r.ChangedCount, ExpectedCount: r.ExpectedCount, FinishedAt: r.FinishedAt, MissingCount: r.MissingCount,
			ObservedCount: r.ObservedCount, Outcome: openapi.SyncStatusLastReconcileOutcome(r.Outcome), StartedAt: r.StartedAt,
		}
	}
	return openapi.GetSyncStatus200JSONResponse(out), nil
}

func dataKindStatus(d *entity.SyncDataKindStatus) openapi.SyncDataKindStatus {
	out := openapi.SyncDataKindStatus{
		DataKind: openapi.DataKind(d.DataKind), Phase: openapi.SyncDataKindStatusPhase(d.Phase), LastAttemptAt: d.LastAttemptAt,
		FailedPagePresent: d.FailedPagePresent, LastSuccessAt: d.LastSuccessAt, Watermark: d.Watermark, HistoryFrom: d.HistoryFrom,
		Completeness: openapi.SyncDataKindStatusCompleteness(d.Completeness), UnknownFields: []string{}, AffectedMetrics: d.AffectedMetrics,
	}
	if out.AffectedMetrics == nil {
		out.AffectedMetrics = []string{}
	}
	switch d.LastOutcome {
	case "succeeded":
		o := openapi.SyncDataKindStatusLastOutcomeSuccess
		out.LastOutcome = &o
	case "failed":
		o := openapi.SyncDataKindStatusLastOutcomeError
		out.LastOutcome = &o
	}
	if d.LastErrorCode != "" {
		code := d.LastErrorCode
		if mapped, ok := _errorCodes[code]; ok {
			code = mapped
		}
		out.LastErrorCode = &code
	}
	if d.LagSeconds != nil {
		lag := int(min(*d.LagSeconds, math.MaxInt32))
		out.LagSeconds = &lag
	}
	return out
}

func (h *connections) RequestSyncRun(ctx context.Context, req openapi.RequestSyncRunRequestObject) (openapi.RequestSyncRunResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/kind", Code: entity.FieldRequired}}}
	}
	op, err := h.operations.RequestSyncRun(ctx, s, entity.ID(req.ConnectionId), entity.SyncRunRequest{
		Kind: entity.SyncRunKind(req.Body.Kind), DataKind: string(req.Body.DataKind),
	})
	if err != nil {
		return nil, err
	}
	location := "/api/v1/operations/" + op.ID.String()
	return openapi.RequestSyncRun202JSONResponse{Body: operationBody(op), Headers: openapi.RequestSyncRun202ResponseHeaders{Location: &location}}, nil
}

func (h *session) GetBackgroundOperation(ctx context.Context, req openapi.GetBackgroundOperationRequestObject) (openapi.GetBackgroundOperationResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	op, err := h.operations.Operation(ctx, s, entity.ID(req.OperationId))
	if err != nil {
		return nil, err
	}
	return openapi.GetBackgroundOperation200JSONResponse(operationBody(op)), nil
}

func operationBody(op entity.Operation) openapi.BackgroundOperation {
	out := openapi.BackgroundOperation{
		Id: uuid.UUID(op.ID), Kind: openapi.BackgroundOperationKind(op.Kind), Status: openapi.BackgroundOperationStatus(op.Status),
		CreatedAt: op.CreatedAt, UpdatedAt: op.UpdatedAt, ResultUri: optional(op.ResultURI),
	}
	if !op.Done() {
		after := _pollAfter
		out.RetryAfterSeconds = &after
	}
	if op.Status == entity.OperationFailed && op.CorrelationID != nil {
		out.Error = alloc(out.Error)
		out.Error.Code, out.Error.CorrelationId = op.ErrorCode, uuid.UUID(*op.CorrelationID)
	}
	return out
}

// alloc makes a value of the anonymous type of a field of the generated contract.
func alloc[T any](*T) *T { return new(T) }

// withLastSuccess fills last_success_at of connections from the state of their collection.
func (h *connections) withLastSuccess(ctx context.Context, orgID entity.ID, items []openapi.Connection) error {
	last, err := h.sync.LastSuccess(ctx, orgID)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].LastSuccessAt = last[entity.ID(items[i].Id)]
	}
	return nil
}
