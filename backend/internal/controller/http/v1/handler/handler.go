package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/cookie"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

// Domain handlers are embedded here; the fallback is one level deeper, so their methods win.
type Handler struct {
	fallback
	session
}

type fallback struct {
	openapi.Unimplemented
}

type Deps struct {
	Logins     usecase.Logins
	Access     usecase.Access
	Registerer prometheus.Registerer
}

func New(d Deps) *Handler {
	return &Handler{session: newSession(d.Logins, d.Access, d.Registerer)}
}

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
	switch {
	case errors.Is(err, openapi.ErrNotImplemented):
		response.Write(c, response.NotImplemented)
		return
	case errors.Is(err, entity.ErrAuthRequired):
		cookie.Clear(c.Writer, cookie.Session)
		response.Write(c, response.AuthRequired)
		return
	case errors.Is(err, entity.ErrNotFound):
		response.Write(c, response.NotFound)
		return
	case errors.Is(err, entity.ErrForbidden):
		response.Write(c, response.Forbidden)
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
