package health_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

func TestCheckLocalLiveness(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		live   bool
	}{
		{"live", http.StatusOK, true},
		{"unhealthy", http.StatusServiceUnavailable, false},
		{"redirect", http.StatusFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/livez", r.URL.Path)
				if tc.status == http.StatusFound {
					w.Header().Set("Location", "/readyz")
				}
				w.WriteHeader(tc.status)
			}))
			defer s.Close()
			u, err := url.Parse(s.URL)
			require.NoError(t, err)
			assert.Equal(t, tc.live, health.CheckLocalLiveness(":"+u.Port()))
		})
	}
	assert.False(t, health.CheckLocalLiveness("missing-port"))
	assert.False(t, health.CheckLocalLiveness(":0"))
}
