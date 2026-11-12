package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
)

// Domain handlers are embedded here; the fallback is one level deeper, so their methods win.
type Handler struct {
	fallback
}

type fallback struct {
	openapi.Unimplemented
}

func New() *Handler { return &Handler{} }

func ParameterError(c *gin.Context, err error, _ int) {
	_ = c.Error(err)
	response.Write(c, response.MalformedRequest)
}

func RequestError(c *gin.Context, err error) {
	_ = c.Error(err)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		response.Write(c, response.PayloadTooLarge)
		return
	}
	response.Write(c, response.MalformedRequest)
}

func Error(c *gin.Context, err error) {
	if errors.Is(err, openapi.ErrNotImplemented) {
		response.Write(c, response.NotImplemented)
		return
	}
	_ = c.Error(err)
	response.Write(c, response.InternalError)
}

func ResponseError(c *gin.Context, err error) {
	_ = c.Error(err)
	if !c.Writer.Written() {
		response.Write(c, response.InternalError)
	}
}
