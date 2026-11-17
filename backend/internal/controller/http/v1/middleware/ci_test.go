package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCIIngress_EmptyRunnerNetworkDeniesUpload(t *testing.T) {
	t.Parallel()
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.POST("/", func(c *gin.Context) { c.Set(_actionKey, "ingest.upload") }, CIIngress(nil), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	req.RemoteAddr = "10.20.1.9:42"
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	assert.Equal(t, http.StatusForbidden, out.Code)
}
