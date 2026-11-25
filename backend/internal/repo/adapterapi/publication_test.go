package adapterapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type publicationRoundTripper func(*http.Request) (*http.Response, error)

func (f publicationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestFindPublishedRequiresExplicitBoolean(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, wantID string
		invalid            bool
	}{
		{name: "missing", body: `{}`, invalid: true},
		{name: "null", body: `{"found":null}`, invalid: true},
		{name: "string", body: `{"found":"false"}`, invalid: true},
		{name: "inconsistent ID", body: `{"found":false,"external_id":"123"}`, invalid: true},
		{name: "absent", body: `{"found":false,"external_id":null}`},
		{name: "found", body: `{"found":true,"external_id":"123"}`, wantID: "123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := NewCaller(Config{Timeout: time.Second})
			require.NoError(t, err)
			c.clients = &http.Client{Transport: publicationRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			target := entity.AdapterTarget{Kind: entity.AdapterClient, URL: "https://adapter.example"}
			id, err := c.FindPublished(t.Context(), target, entity.PublicationTarget{Kind: entity.MRComment, Marker: "mr"})
			require.Equal(t, tc.wantID, id)
			if tc.invalid {
				var ae *entity.AdapterError
				require.ErrorAs(t, err, &ae)
				assert.Equal(t, entity.AdapterContractViolation, ae.Code)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
