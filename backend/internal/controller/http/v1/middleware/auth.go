package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/cookie"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const (
	_actionKey = "craftpulse_action"
	_apiPrefix = "/api/v1"
)

var _areaParams = []struct {
	name string
	kind entity.AreaKind
}{
	{"team_id", entity.AreaTeam},
	{"project_id", entity.AreaProject},
	{"service_id", entity.AreaService},
}

type subjectKey struct{}

func SubjectFrom(ctx context.Context) (entity.Subject, bool) {
	s, ok := ctx.Value(subjectKey{}).(entity.Subject)
	return s, ok
}

// Operation names the request by its contract operation before parameters are read, so a refusal is
// labeled too. A route missing from the contract has no action and is refused by access.
func Operation(ops map[string]openapi.Operation) gin.HandlerFunc {
	return func(c *gin.Context) {
		if op, ok := ops[c.Request.Method+" "+strings.TrimPrefix(c.FullPath(), _apiPrefix)]; ok {
			c.Set(_operationKey, op.ID)
			c.Set(_actionKey, op.Action)
		}
		c.Next()
	}
}

// Authenticate turns the session cookie into the subject before any data is read (TZ 13.4). A changing
// request with the cookie must come from the site itself (TZ 13.2, SEC-09).
func Authenticate(logins usecase.Logins, siteOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(_actionKey) == openapi.ActionPublic {
			c.Next()
			return
		}
		token, err := c.Cookie(cookie.Session)
		if err != nil || token == "" {
			response.Write(c, response.AuthRequired)
			return
		}
		if changes(c.Request.Method) && c.GetHeader("Origin") != siteOrigin {
			response.Write(c, response.OriginRejected)
			return
		}
		s, err := logins.Authenticate(c.Request.Context(), token)
		if errors.Is(err, entity.ErrAuthRequired) {
			cookie.Clear(c.Writer, cookie.Session)
			response.Write(c, response.AuthRequired)
			return
		}
		if err != nil {
			_ = c.Error(err)
			response.Write(c, response.InternalError)
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), subjectKey{}, s))
		c.Next()
	}
}

// Authorize checks the action of the operation against the roles of the subject, read on this request.
func Authorize(access usecase.Access, metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := c.GetString(_actionKey)
		if action == openapi.ActionPublic {
			c.Next()
			return
		}
		s, ok := SubjectFrom(c.Request.Context())
		if !ok {
			response.Write(c, response.AuthRequired)
			return
		}
		err := access.Authorize(c.Request.Context(), s, entity.AccessRequest{
			Operation: c.GetString(_operationKey),
			Action:    action,
			Area:      area(c),
		})
		switch {
		case err == nil:
			c.Next()
		case errors.Is(err, entity.ErrNotFound):
			metrics.denied(s.UserID.String(), "outside_scope")
			response.Write(c, response.NotFound)
		case errors.Is(err, entity.ErrForbidden):
			metrics.denied(s.UserID.String(), "no_permission")
			response.Write(c, response.Forbidden)
		default:
			_ = c.Error(err)
			response.Write(c, response.InternalError)
		}
	}
}

// area takes the first area of the path; a malformed ID is left to parameter binding, which answers 400.
func area(c *gin.Context) entity.AreaRef {
	for _, p := range _areaParams {
		if raw := c.Param(p.name); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return entity.AreaRef{}
			}
			return entity.AreaRef{Kind: p.kind, ID: entity.ID(id)}
		}
	}
	return entity.AreaRef{}
}

func changes(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
