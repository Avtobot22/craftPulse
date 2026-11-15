package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
)

// Detach hands a strict handler a copy of the gin context. Gin reuses the context of a finished request, while
// an outbound call made with it may still read it from a goroutine of net/http. The errors the handler records
// go back to the request for its log.
func Detach(f openapi.StrictHandlerFunc, _ string) openapi.StrictHandlerFunc {
	return func(c *gin.Context, request any) (any, error) {
		cp := c.Copy()
		out, err := f(cp, request)
		c.Errors = append(c.Errors, cp.Errors...)
		return out, err
	}
}
