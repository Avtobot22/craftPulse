package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	webhooks usecase.Webhooks
	sourceLinks
	documentation
	links
	uploads
	accounts
	connections
	intake
	process
	checks
	reports
	codeDetails
	fallback
	session
	organization
	administration
}

type fallback struct {
	openapi.Unimplemented
}

type Deps struct {
	Webhooks      usecase.Webhooks
	SourceLinks   usecase.SourceLinks
	SiteURL       string
	Documentation usecase.Documentation
	Links         usecase.Links
	UploadTokens  usecase.UploadTokens
	Logins        usecase.Logins
	Access        usecase.Access
	Structure     usecase.Structure
	Users         usecase.Users
	Audit         usecase.AuditLog
	Protocols     usecase.Protocols
	Connections   usecase.Connections
	Accounts      usecase.Accounts
	Intake        usecase.Intake
	Sync          usecase.SyncStatus
	Operations    usecase.Operations
	Reports       usecase.TeamReports
	Code          usecase.CodeDetails
	Process       usecase.Process
	Previews      usecase.MappingPreviews
	Targets       usecase.Targets
	Checks        usecase.CheckProfiles
	Registerer    prometheus.Registerer
}

func New(d Deps) *Handler {
	return &Handler{
		webhooks:       d.Webhooks,
		sourceLinks:    sourceLinks{uc: d.SourceLinks, callbackURL: d.SiteURL + "/source-links/callback"},
		documentation:  documentation{uc: d.Documentation},
		links:          links{uc: d.Links},
		uploads:        uploads{tokens: d.UploadTokens},
		connections:    connections{uc: d.Connections, sync: d.Sync, operations: d.Operations, siteURL: d.SiteURL},
		accounts:       accounts{uc: d.Accounts, connections: d.Connections},
		intake:         intake{uc: d.Intake},
		process:        process{uc: d.Process, previews: d.Previews, targets: d.Targets, connections: d.Connections},
		checks:         checks{uc: d.Checks},
		codeDetails:    codeDetails{uc: d.Code},
		reports:        reports{operations: d.Operations, views: d.Reports},
		session:        newSession(d.Logins, d.Access, d.Operations, d.Registerer),
		organization:   organization{structure: d.Structure, access: d.Access, connections: d.Connections},
		administration: administration{users: d.Users, audit: d.Audit, protocols: d.Protocols},
	}
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
	// encoding/json reports a refused field only in the message: json: unknown field "name".
	if quoted, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		name, uerr := strconv.Unquote(quoted)
		if uerr == nil {
			response.WriteDetail(c, response.ValidationFailed, "", []entity.FieldError{{Field: name, Code: entity.FieldUnknownField}})
			return
		}
	}
	response.Write(c, response.MalformedRequest)
}

func Error(c *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrObjectStoreUnavailable):
		_ = c.Error(err)
		c.Header("Retry-After", "3")
		response.Write(c, response.ServiceUnavailable)
		return
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
	case errors.Is(err, entity.ErrTokenSlotsFull), errors.Is(err, entity.ErrScopeTaken), errors.Is(err, entity.ErrStateConflict):
		response.Write(c, response.StateConflict)
		return
	case errors.Is(err, entity.ErrVersionConflict):
		response.Write(c, response.VersionConflict)
		return
	case errors.Is(err, entity.ErrIdempotencyKeyReused):
		response.Write(c, response.IdempotencyKeyReused)
		return
	case errors.Is(err, errMalformedPage), errors.Is(err, errMalformedForm), errors.Is(err, entity.ErrInvalidCursor):
		response.Write(c, response.MalformedRequest)
		return
	case errors.Is(err, errPayloadTooLarge):
		response.Write(c, response.PayloadTooLarge)
		return
	case errors.Is(err, errUnsupportedFile):
		response.Write(c, response.UnsupportedMedia)
		return
	}
	if verr, ok := errors.AsType[*entity.ValidationError](err); ok {
		response.WriteDetail(c, response.ValidationFailed, "", verr.Fields)
		return
	}
	// The adapter refused or did not take the call; its detail stays in the log, the class goes out.
	if ae, ok := errors.AsType[*entity.AdapterError](err); ok {
		_ = c.Error(err)
		if ae.Class == entity.CallSource && ae.Code == "invalid_request" {
			response.WriteDetail(c, response.ValidationFailed, "", []entity.FieldError{{Field: "/credentials", Code: entity.FieldInvalidFormat}})
			return
		}
		c.Header("Retry-After", "30")
		response.WriteDetail(c, response.ServiceUnavailable, "adapter call failed: "+string(ae.Class)+" "+ae.Code, nil)
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
