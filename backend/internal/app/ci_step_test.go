package app

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

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
