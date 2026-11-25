package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type ciReceiptTransport struct{ router http.Handler }

func (tr ciReceiptTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.RemoteAddr = "10.20.1.2:1234"
	rec := httptest.NewRecorder()
	tr.router.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// awaitCIResult observes at least one server response before making the result available.
func (e *ciEnv) awaitCIResult(t *testing.T, id uuid.UUID, makeAvailable func()) string {
	t.Helper()
	site, err := url.Parse(_site)
	require.NoError(t, err)
	polls := 0
	client, err := ci.NewClient(&ci.Job{Site: site, Token: e.token.Value}, ci.ClientConfig{
		HTTP: &http.Client{Transport: ciReceiptTransport{e.router}},
		Sleep: func(context.Context, time.Duration) error {
			polls++
			if polls == 2 {
				makeAvailable()
			}
			require.LessOrEqual(t, polls, 2, "receipt still pending after its result was built")
			return nil
		},
	})
	require.NoError(t, err)
	result, err := client.Await(t.Context(), &ciapi.ReceiptAccepted{
		ReceiptId: id, Status: ciapi.ReceiptAcceptedStatusQuarantined,
	}, 15*time.Second)
	require.NoError(t, err)
	require.NotNil(t, result.ResultUrl)
	assert.Nil(t, result.RetryAfterSeconds)
	assert.Equal(t, 2, polls)
	return *result.ResultUrl
}

func TestCIUpload_AcceptsPackageOfTheCIStep(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	dir := t.TempDir()
	report := filepath.Join(dir, "gl-sast-report.sarif")
	require.NoError(t, os.WriteFile(report, []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"semgrep"}},"results":[]}]}`), 0o600))
	job := &ci.Job{
		InstanceKey: "gitlab.example", ProjectID: "42", ProjectPath: "demo/billing", PipelineID: "5001", JobID: "7002",
		SHA: "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac", RefName: "main", WorkDir: filepath.Join(dir, "state"),
	}
	store := ci.NewStore(job.WorkDir)
	im := &ci.Importer{Job: job, Profile: &ciapi.CiCheckProfile{}, Store: store, Now: time.Now}
	_, err := im.Add(&ci.Import{
		Reports: []ci.ReportSpec{{Kind: "sast", Format: "sarif", Path: report}}, Producer: "semgrep", ProducerVersion: "1.95.0",
		FormatVersion: "2.1.0", ProducerJob: "7001",
	})
	require.NoError(t, err)
	pkg, err := (&ci.Packer{Job: job, Store: store, Version: "test", Now: time.Now}).Build(envelope.DefaultLimits)
	require.NoError(t, err)

	response := e.uploadCI(t, e.token.Value, pkg.Data)

	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	first := decode[acceptedBody](t, response)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(first.ReceiptID)))
	rc := e.readCI(t, first.ReceiptID, e.token.Value)
	assert.Equal(t, "accepted", rc.Status)
	assert.Equal(t, 1, rc.AcceptedCount)
	assert.Zero(t, rc.RejectedCount)
}
