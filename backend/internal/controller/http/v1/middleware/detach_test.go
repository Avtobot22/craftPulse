package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetach_HandlerContextIsNotReused: every request hands the handler its own context, which a goroutine may read
// after the request, and the errors the handler records reach the request.
func TestDetach_HandlerContextIsNotReused(t *testing.T) {
	t.Parallel()

	var (
		seen   []*gin.Context
		logged []int
	)
	handler := Detach(func(c *gin.Context, _ any) (any, error) {
		seen = append(seen, c)
		_ = c.Error(errors.New("recorded by the handler"))
		return nil, nil
	}, "op")
	e := gin.New()
	e.ContextWithFallback = true
	e.GET("/", func(c *gin.Context) {
		_, _ = handler(c, nil)
		logged = append(logged, len(c.Errors))
		c.Status(http.StatusNoContent)
	})
	for range 2 {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
	}

	require.Len(t, seen, 2)
	assert.NotSame(t, seen[0], seen[1], "the context of a finished request is not the one gin hands the next")
	assert.Equal(t, []int{1, 1}, logged)
	assert.NotNil(t, seen[0].Done(), "the copy still reads the context of its request")
}
