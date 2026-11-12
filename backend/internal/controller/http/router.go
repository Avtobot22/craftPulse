package http

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	v1 "github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

func NewRouter(log zerolog.Logger, reg prometheus.Registerer, probes health.Probes, h *handler.Handler) *gin.Engine {
	r := gin.New()
	// Strict handlers receive *gin.Context as context.Context; this exposes the request values.
	r.ContextWithFallback = true
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	_ = r.SetTrustedProxies(nil)

	state := gin.WrapH(health.Handler(probes))
	r.GET("/livez", state)
	r.GET("/readyz", state)
	r.GET("/version", state)

	chain := make([]gin.HandlerFunc, 0, 4)
	chain = append(chain, middleware.Correlate(log), middleware.Observe(middleware.NewMetrics(reg)), middleware.Recovery())
	r.NoRoute(append(chain, func(c *gin.Context) { response.Write(c, response.NotFound) })...)

	root := r.Group("", chain...)
	v1.RegisterRoutes(root, root.Group("/api/v1"), h)
	return r
}
