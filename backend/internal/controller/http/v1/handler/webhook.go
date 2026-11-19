package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _maxWebhookBody = 1 << 20

func (h *Handler) ReceiveWebhookHTTP(c *gin.Context, id uuid.UUID) {
	ctx := c.Request.Context()
	route, err := h.webhooks.WebhookRoute(ctx, entity.ID(id))
	if err != nil {
		Error(c, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, _maxWebhookBody+1))
	if err != nil {
		RequestError(c, err)
		return
	}
	if len(body) > _maxWebhookBody {
		response.Write(c, response.PayloadTooLarge)
		return
	}
	headers := make(map[string]string, len(c.Request.Header))
	for name, values := range c.Request.Header {
		headers[name] = strings.Join(values, ", ")
	}
	if err := h.webhooks.ReceiveWebhook(ctx, route, entity.WebhookInput{Headers: headers, Body: body}, entity.ID(logger.CorrelationID(ctx))); err != nil {
		Error(c, err)
		return
	}
	c.Status(http.StatusOK)
}
