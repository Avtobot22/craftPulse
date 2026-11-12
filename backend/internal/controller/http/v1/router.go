package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const _maxBodyBytes = 25 << 20

type Deps struct {
	Handler *handler.Handler
	Logins  usecase.Logins
	Access  usecase.Access
	Metrics *middleware.Metrics
	// SiteOrigin is the only Origin accepted on a changing request with the session cookie.
	SiteOrigin string
}

func RegisterRoutes(root, api *gin.RouterGroup, d Deps) {
	strict := openapi.NewStrictHandlerWithOptions(d.Handler, nil,
		openapi.StrictGinServerOptions{
			RequestErrorHandlerFunc:  handler.RequestError,
			HandlerErrorFunc:         handler.Error,
			ResponseErrorHandlerFunc: handler.ResponseError,
		})
	guard := []gin.HandlerFunc{
		middleware.Operation(openapi.Operations),
		middleware.Authenticate(d.Logins, d.SiteOrigin),
		middleware.Authorize(d.Access, d.Metrics),
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
