package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (h *intake) UploadPackage(ctx context.Context, req openapi.UploadPackageRequestObject) (openapi.UploadPackageResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok || s.UploadTokenID.IsZero() {
		return nil, entity.ErrAuthRequired
	}
	rc, err := h.uc.SubmitCI(ctx, s, req.Body)
	if err != nil {
		return nil, err
	}
	if rc.Status == entity.ReceiptRejected {
		if rc.ReasonCode == entity.PackageLimitExceeded {
			return nil, errPayloadTooLarge
		}
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/body", Code: entity.FieldInvalidFormat, Detail: rc.ReasonCode}}}
	}
	location := "/api/v1/receipts/" + rc.ID.String()
	body := openapi.ReceiptAccepted{ReceiptId: uuid.UUID(rc.ID), Status: openapi.ReceiptAcceptedStatus(rc.Status), DuplicateOf: uuidPtr(rc.DuplicateOf)}
	if rc.Status == entity.ReceiptQuarantined {
		n := _receiptPoll
		body.RetryAfterSeconds = &n
	}
	return openapi.UploadPackage202JSONResponse{Body: body, Headers: openapi.UploadPackage202ResponseHeaders{Location: &location}}, nil
}
