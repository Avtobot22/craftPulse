package ci

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionsAttemptID(t *testing.T) {
	const token = "actions-test-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/demo/app/actions/runs/5001/attempts/2/jobs", r.URL.Path)
		assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		_, _ = fmt.Fprint(w, `{"jobs":[{"id":6800,"name":"build"},{"id":7002,"name":"scan"}]}`)
	}))
	defer server.Close()
	job := Job{ProjectPath: "demo/app", PipelineID: "5001", AttemptNo: 2, JobID: "scan"}
	env := map[string]string{ActionsTokenEnv: token, "GITHUB_API_URL": server.URL}
	id, err := job.resolveActionsAttemptID(t.Context(), func(k string) string { return env[k] }, server.Client().Transport)
	require.NoError(t, err)
	assert.Equal(t, "2/7002", id)
}
