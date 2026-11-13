package v1

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const _maxBodyBytes = 25 << 20

var _strictJSON sync.Once

type Deps struct {
	UploadTokens usecase.UploadTokens
	Handler      *handler.Handler
	Logins       usecase.Logins
	Access       usecase.Access
	Metrics      *middleware.Metrics
	// SiteOrigin is the only Origin accepted on a changing request with the session cookie.
	SiteOrigin string
}

func RegisterRoutes(root, api *gin.RouterGroup, d Deps) {
	// An unknown field of a body is refused (contract: 422 unknown_field); gin keeps the switch global.
	_strictJSON.Do(func() { binding.EnableDecoderDisallowUnknownFields = true })
	strict := openapi.NewStrictHandlerWithOptions(d.Handler, nil,
		openapi.StrictGinServerOptions{
			RequestErrorHandlerFunc:  handler.RequestError,
			HandlerErrorFunc:         handler.Error,
			ResponseErrorHandlerFunc: handler.ResponseError,
		})
	guard := []gin.HandlerFunc{
		middleware.Operation(openapi.Operations),
		middleware.UploadAuthentication(d.UploadTokens),
		middleware.Authenticate(d.Logins, d.SiteOrigin),
		middleware.Authorize(d.Access, d.Metrics),
		middleware.Contract(),
	}
	api.Use(guard...)
	api.Use(middleware.LimitBody(_maxBodyBytes))
	openapi.RegisterHandlersWithOptions(api, strict, openapi.GinServerOptions{ErrorHandler: handler.ParameterError})

	// The contract serves login from the site root; the generated routes also answer under api.
	login := openapi.ServerInterfaceWrapper{Handler: strict, ErrorHandler: handler.ParameterError}
	site := root.Group("", guard...)
	site.GET("/o/:org_slug/login", login.StartLogin)
	site.GET("/o/:org_slug/callback", login.CompleteLogin)
}
