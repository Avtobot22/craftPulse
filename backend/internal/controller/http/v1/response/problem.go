package response

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _problemContentType = "application/problem+json"

type Problem struct {
	status int
	code   string
	title  string
}

var (
	MalformedRequest = Problem{http.StatusBadRequest, "malformed_request", "Malformed request"}
	NotFound         = Problem{http.StatusNotFound, "not_found", "Not found"}
	PayloadTooLarge  = Problem{http.StatusRequestEntityTooLarge, "payload_too_large", "Payload is too large"}
	InternalError    = Problem{http.StatusInternalServerError, "internal_error", "Internal server error"}
	NotImplemented   = Problem{http.StatusNotImplemented, "not_implemented", "Operation is not implemented"}
)

func Write(c *gin.Context, p Problem) {
	body, _ := json.Marshal(openapi.Problem{
		Type:          "urn:craftpulse:problem:" + p.code,
		Title:         p.title,
		Status:        p.status,
		Code:          p.code,
		CorrelationId: logger.CorrelationID(c.Request.Context()),
	})
	c.Header("Cache-Control", "no-store")
	c.Data(p.status, _problemContentType, body)
	c.Abort()
}
