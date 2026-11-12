package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
)

const _maxBodyBytes = 25 << 20

func RegisterRoutes(root, api *gin.RouterGroup, h *handler.Handler) {
	strict := openapi.NewStrictHandlerWithOptions(h,
		[]openapi.StrictMiddlewareFunc{middleware.Operation},
		openapi.StrictGinServerOptions{
			RequestErrorHandlerFunc:  handler.RequestError,
			HandlerErrorFunc:         handler.Error,
			ResponseErrorHandlerFunc: handler.ResponseError,
		})
	api.Use(middleware.LimitBody(_maxBodyBytes))
	openapi.RegisterHandlersWithOptions(api, strict, openapi.GinServerOptions{ErrorHandler: handler.ParameterError})

	// The contract serves login from the site root; the generated routes also answer under api.
	login := openapi.ServerInterfaceWrapper{Handler: strict, ErrorHandler: handler.ParameterError}
	root.GET("/o/:org_slug/login", login.StartLogin)
	root.GET("/o/:org_slug/callback", login.CompleteLogin)
}
