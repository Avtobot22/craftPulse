package response

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _problemContentType = "application/problem+json"

type Problem struct {
	status int
	code   string
	title  string
}

var (
	StateConflict        = Problem{http.StatusConflict, "state_conflict", "Resource state conflicts with this operation"}
	MalformedRequest     = Problem{http.StatusBadRequest, "malformed_request", "Malformed request"}
	UnknownParameter     = Problem{http.StatusBadRequest, "unknown_parameter", "Unknown query parameter"}
	AuthRequired         = Problem{http.StatusUnauthorized, "auth_required", "Authentication required"}
	Forbidden            = Problem{http.StatusForbidden, "forbidden", "Action is not allowed"}
	OriginRejected       = Problem{http.StatusForbidden, "origin_rejected", "Request origin is not allowed"}
	NotFound             = Problem{http.StatusNotFound, "not_found", "Not found"}
	VersionConflict      = Problem{http.StatusPreconditionFailed, "version_conflict", "Resource version has changed"}
	PayloadTooLarge      = Problem{http.StatusRequestEntityTooLarge, "payload_too_large", "Payload is too large"}
	ValidationFailed     = Problem{http.StatusUnprocessableEntity, "validation_failed", "Validation failed"}
	PreconditionRequired = Problem{http.StatusPreconditionRequired, "precondition_required", "If-Match header is required"}
	InternalError        = Problem{http.StatusInternalServerError, "internal_error", "Internal server error"}
	NotImplemented       = Problem{http.StatusNotImplemented, "not_implemented", "Operation is not implemented"}
)

func Write(c *gin.Context, p Problem) {
	WriteDetail(c, p, "", nil)
}

// WriteDetail adds a detail meant for the developer and the field errors of validation_failed.
func WriteDetail(c *gin.Context, p Problem, detail string, fields []entity.FieldError) {
	problem := openapi.Problem{
		Type:          "urn:craftpulse:problem:" + p.code,
		Title:         p.title,
		Status:        p.status,
		Code:          p.code,
		CorrelationId: logger.CorrelationID(c.Request.Context()),
	}
	if detail != "" {
		problem.Detail = &detail
	}
	if len(fields) > 0 {
		errs := make([]openapi.FieldError, len(fields))
		for i, f := range fields {
			errs[i] = openapi.FieldError{Field: f.Field, Code: openapi.FieldErrorCode(f.Code)}
			if f.Detail != "" {
				errs[i].Detail = &f.Detail
			}
		}
		problem.Errors = &errs
	}
	body, _ := json.Marshal(problem)
	c.Header("Cache-Control", "no-store")
	c.Data(p.status, _problemContentType, body)
	c.Abort()
}
