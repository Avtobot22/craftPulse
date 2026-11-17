package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/objstore"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

type ciEnv struct {
	contract *openapi3.T
	*ingestEnv
	adminBrowser *browser
	token        uploadTokenBody
}

func newCIEnv(t *testing.T) *ciEnv { return newCIEnvWithStore(t, nil) }

func newCIEnvWithStore(t *testing.T, store *objstore.Store) *ciEnv {
	t.Helper()
	e := &ciEnv{ingestEnv: newIngestEnv(t)}
	var loadErr error
	e.contract, loadErr = openapi3.NewLoader().LoadFromFile("../../../docs/technical-specification/openapi.yaml")
	require.NoError(t, loadErr)
	orgs, _, err := newOrgAccess(e.app, e.clock, e.connect, nil)
	require.NoError(t, err)
	if store != nil {
		e.store, e.reg = store, prometheus.NewRegistry()
		e.uc, err = newIngest(e.app, store, e.connect, e.rights, orgs, e.clock, e.reg)
		require.NoError(t, err)
	}
	// Reuse the registry of the intake so one scrape includes both HTTP and CI outcomes.
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, e.reg,
		apiModules{orgs: orgs, access: e.rights, connections: e.connect, intake: e.uc})
	api.RunnerNetwork = []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16"), netip.MustParsePrefix("2001:db8:20::/48")}
	api.TrustedProxies = []string{"192.0.2.2"}
	e.router = httpctrl.NewRouter(zerolog.New(zerolog.SyncWriter(e.logs)), e.reg, health.Probes{Ready: &health.Readiness{}}, api)
	e.adminBrowser = e.loginEnv.admin(t, "demo")
	e.token = e.issueUpload(t, e.adminBrowser, uuid.UUID(e.project))
	e.bindRepository(t, e.project, "42")
	return e
}

func (e *ciEnv) bindRepository(t *testing.T, project entity.ID, native string) {
	t.Helper()
	inst, err := e.connect.EnsureInstance(t.Context(), e.orgID, entity.ProviderGitLab, "gitlab.example")
	require.NoError(t, err)
	org, area, conn := uuid.UUID(e.orgID), uuid.New(), uuid.New()
	_, err = e.owner.Exec(t.Context(), `INSERT INTO connect.connection
  (organization_id,id,instance_id,name,adapter_kind,contract_version,access_mode,status,row_version,created_by,created_at,sync_paused)
  VALUES ($1,$2,$3,'GitLab','builtin','1','service_account','active',1,$4,$5,false)`, org, conn, uuid.UUID(inst.ID), uuid.UUID(e.admin.UserID), e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.source_object
  (organization_id,id,instance_id,object_kind,native_ns,native_id,identity_quality,first_known_seq,restricted)
  VALUES ($1,$2,$3,'repository','',$4,'stable',1,false)`, org, area, uuid.UUID(inst.ID), native)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.area (organization_id,area_id,area_kind,last_change_seq) VALUES ($1,$2,'repository',1)`, org, area)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO connect.connection_scope
  (organization_id,connection_id,instance_id,scope_kind,native_scope_id,display_name,area_id,status,added_at)
  VALUES ($1,$2,$3,'repository',$4,'Repository',$5,'active',$6)`, org, conn, uuid.UUID(inst.ID), native, area, e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
  VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, org, uuid.UUID(project), area, e.clock.Now())
	require.NoError(t, err)
}

func (e *ciEnv) request(t *testing.T, method, path, token, remote, forwarded, media string, body io.Reader, chunked bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, body)
	req.RemoteAddr = remote
	req.Header.Set("Authorization", "Bearer "+token)
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
		req.Header.Set("X-Real-IP", forwarded)
	}
	if chunked {
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
	}
	out := httptest.NewRecorder()
	e.router.ServeHTTP(out, req)
	e.validateResponse(t, req, out)
	return out
}

func (e *ciEnv) uploadCI(t *testing.T, token string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	return e.request(t, http.MethodPost, "/api/v1/ingest/packages", token, "10.20.1.9:1234", "", "application/gzip", bytes.NewReader(data), false)
}

func ciPackage(t *testing.T, mutate func(map[string][]byte)) []byte {
	t.Helper()
	root := "../../pkg/envelope/testdata/ci/gitlab-builtin"
	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)], err = os.ReadFile(path)
		return err
	}))
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(files[envelope.ManifestName], &manifest))
	source := manifest["source"].(map[string]any)
	source["provider"], source["instance_key"] = "gitlab", "gitlab.example"
	files[envelope.ManifestName] = mustMarshal(t, manifest)
	if mutate != nil {
		mutate(files)
	}
	names := []string{}
	for name := range files {
		if name != envelope.ManifestName {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if _, ok := files[envelope.ManifestName]; ok {
		names = append([]string{envelope.ManifestName}, names...)
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		data := files[name]
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return out.Bytes()
}

func (e *ciEnv) readCI(t *testing.T, id uuid.UUID, token string) receiptBody {
	t.Helper()
	rec := e.bearer(t, token, "/api/v1/receipts/"+id.String())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[receiptBody](t, rec)
}

func TestCIUpload_AcceptRepeatAndProjectReceipts(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	data := ciPackage(t, nil)
	response := e.uploadCI(t, e.token.Value, data)
	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	first := decode[acceptedBody](t, response)
	assert.Equal(t, "quarantined", first.Status)
	require.NotNil(t, first.RetryAfter)
	assert.Equal(t, "/api/v1/receipts/"+first.ReceiptID.String(), response.Header().Get("Location"))
	rc := e.receipt(t, entity.ID(first.ReceiptID))
	require.NotNil(t, rc.ProjectID)
	require.NotNil(t, rc.UploadTokenID)
	assert.Equal(t, e.project, *rc.ProjectID)
	assert.Equal(t, entity.ID(e.token.ID), *rc.UploadTokenID)
	assert.Nil(t, rc.RequestedBy)
	var lane string
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT queue FROM river_job WHERE kind='ingest.package' AND args->>'receipt_id'=$1`, first.ReceiptID.String()).Scan(&lane))
	assert.Equal(t, entity.QueueInteractive, lane)
	assert.Equal(t, "quarantined", e.readCI(t, first.ReceiptID, e.token.Value).Status)
	// A repeat while the first package is still quarantined is already a duplicate.
	repeat := e.uploadCI(t, e.token.Value, data)
	require.Equal(t, http.StatusAccepted, repeat.Code, repeat.Body.String())
	assert.Equal(t, "duplicate", decode[acceptedBody](t, repeat).Status)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(first.ReceiptID)))
	assert.Equal(t, "accepted", e.readCI(t, first.ReceiptID, e.token.Value).Status)
	for range 3 {
		r := e.uploadCI(t, e.token.Value, data)
		require.Equal(t, http.StatusAccepted, r.Code, r.Body.String())
		dup := decode[acceptedBody](t, r)
		assert.Equal(t, "duplicate", dup.Status)
		require.NotNil(t, dup.DuplicateOf)
		assert.Equal(t, first.ReceiptID, *dup.DuplicateOf)
	}
	assert.Equal(t, 1, e.accepted(t))
	assert.Equal(t, 1.0, ciMetric(t, e.reg, "accepted", ""))
	assert.Equal(t, 4.0, ciMetric(t, e.reg, "duplicate", ""))
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(first.ReceiptID)))
	assert.Equal(t, 1.0, ciMetric(t, e.reg, "accepted", ""), "repeating the worker does not count a second outcome")
	assert.Empty(t, e.keys(t, "quarantine/"))
	replacement := e.issueUpload(t, e.adminBrowser, uuid.UUID(e.project))
	assert.Equal(t, "accepted", e.readCI(t, first.ReceiptID, replacement.Value).Status)
	rows := e.bearer(t, replacement.Value, "/api/v1/receipts/"+first.ReceiptID.String()+"/records")
	require.Equal(t, http.StatusOK, rows.Code, rows.Body.String())
	assert.Contains(t, rows.Body.String(), `"outcome":"accepted"`)
	other := e.tokenProject(t, e.orgID)
	otherToken := e.issueUpload(t, e.adminBrowser, other)
	for _, suffix := range []string{"", "/records"} {
		assert.Equal(t, http.StatusNotFound, e.bearer(t, otherToken.Value, "/api/v1/receipts/"+first.ReceiptID.String()+suffix).Code)
	}
	// Hash lookup is scoped to the project: the other project cannot inherit the first one's acceptance.
	foreign := e.uploadCI(t, otherToken.Value, data)
	require.Equal(t, http.StatusAccepted, foreign.Code, foreign.Body.String())
	f := decode[acceptedBody](t, foreign)
	assert.Equal(t, "quarantined", f.Status)
	assert.Nil(t, f.DuplicateOf)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(f.ReceiptID)))
	assert.Equal(t, 1, e.readCI(t, f.ReceiptID, otherToken.Value).RejectedCount)
	assert.Equal(t, 1, e.accepted(t))
	require.Equal(t, http.StatusNoContent, e.do(t, e.adminBrowser, http.MethodDelete, "/api/v1/upload-tokens/"+e.token.ID.String(), _changing).Code)
	assert.Equal(t, http.StatusUnauthorized, e.uploadCI(t, e.token.Value, data).Code)
	assert.Equal(t, http.StatusUnauthorized, e.bearer(t, e.token.Value, "/api/v1/receipts/"+first.ReceiptID.String()).Code)
	var actor, actorID, area string
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT actor_kind,actor_id::text,area_id::text FROM audit.event WHERE operation='uploadPackage' AND object_id=$1`, first.ReceiptID).Scan(&actor, &actorID, &area))
	assert.Equal(t, "upload_token", actor)
	assert.Equal(t, e.token.ID.String(), actorID)
	assert.Equal(t, e.project.String(), area)
	assert.NotContains(t, e.logs.String(), e.token.Value)
}

func TestCIUpload_PartialChannelAndRepository(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	data := ciPackage(t, func(f map[string][]byte) {
		f[envelope.RecordsName] = append(f[envelope.RecordsName], []byte("\n"+string(mustMarshal(t, task(t, "BAD-1", "private task", ""))))...)
	})
	resp := e.uploadCI(t, e.token.Value, data)
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
	id := decode[acceptedBody](t, resp).ReceiptID
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(id)))
	rc := e.readCI(t, id, e.token.Value)
	assert.Equal(t, "partial", rc.Status)
	assert.Equal(t, 1, rc.AcceptedCount)
	assert.Equal(t, 1, rc.RejectedCount)
	rows := e.records(t, entity.ID(id))
	require.Len(t, rows, 2)
	assert.Equal(t, entity.RecordRejected, rows[1].Outcome)
	assert.NotEmpty(t, rows[1].ReasonCode)
	// A valid CI run naming another repository is rejected beside the valid run.
	m := importManifest("1")
	m.SchemaId = envelope.ManifestSchemaIdCiRuns
	m.Source.Provider, m.Source.InstanceKey = envelope.SourceProviderGitlab, "gitlab.example"
	m.Area.Kind, m.Area.NativeId = envelope.AreaRefKindRepository, "42"
	recs := []*envelope.Record{}
	for _, repo := range []string{"42", "99"} {
		for _, schema := range envelope.RecordSchemas() {
			if schema.Kind == "ci_run" {
				var p map[string]any
				require.NoError(t, json.Unmarshal(schema.Example, &p))
				p["project_native_id"] = repo
				recs = append(recs, &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "ci_run", PayloadSchema: "ci_run/1", NativeId: repo, Payload: mustMarshal(t, p)})
			}
		}
	}
	resp = e.uploadCI(t, e.token.Value, pack(t, m, recs...))
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
	id = decode[acceptedBody](t, resp).ReceiptID
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(id)))
	rc = e.readCI(t, id, e.token.Value)
	assert.Equal(t, "partial", rc.Status)
	assert.Equal(t, 1, rc.AcceptedCount)
	assert.Equal(t, 1, rc.RejectedCount)
	assert.Equal(t, entity.RecordOutsideScope, e.records(t, entity.ID(id))[1].ReasonCode)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return b
}

type unreadCI struct{ reads int }

func (r *unreadCI) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }

func TestCIUpload_NetworkAndMediaBeforeBody(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	for _, tt := range []struct {
		remote, forwarded, media string
		status                   int
	}{
		{"198.51.100.1:42", "", "application/gzip", 403},
		{"198.51.100.1:42", "10.20.1.9", "application/gzip", 403},
		{"192.0.2.2:42", "198.51.100.9", "application/gzip", 403},
		{"10.20.1.9:42", "", "application/json", 415},
	} {
		body := &unreadCI{}
		out := e.request(t, http.MethodPost, "/api/v1/ingest/packages", e.token.Value, tt.remote, tt.forwarded, tt.media, body, true)
		assert.Equal(t, tt.status, out.Code, out.Body.String())
		assert.Zero(t, body.reads)
	}
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt"))
	assert.Empty(t, e.keys(t, "quarantine/"))
	for _, tt := range []struct{ remote, forwarded string }{
		{"192.0.2.2:42", "10.20.1.9"}, {"[2001:db8:20::7]:42", ""},
	} {
		out := e.request(t, http.MethodPost, "/api/v1/ingest/packages", e.token.Value, tt.remote, tt.forwarded, "application/gzip", bytes.NewReader(ciPackage(t, nil)), false)
		require.Equal(t, http.StatusAccepted, out.Code, out.Body.String())
	}
}

func TestCIUpload_LimitsAndInvalidManifest(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	for _, tt := range []struct {
		data    []byte
		chunked bool
		status  int
	}{
		{make([]byte, 26<<20), false, 413},
		{make([]byte, 26<<20), true, 413},
		{ciPackage(t, func(f map[string][]byte) { delete(f, envelope.ManifestName) }), false, 422},
		{ciPackage(t, func(f map[string][]byte) { delete(f, envelope.ScanManifestName) }), false, 422},
	} {
		out := e.request(t, http.MethodPost, "/api/v1/ingest/packages", e.token.Value, "10.20.1.9:42", "", "application/gzip", bytes.NewReader(tt.data), tt.chunked)
		assert.Equal(t, tt.status, out.Code, out.Body.String())
		assert.Empty(t, e.keys(t, "quarantine/"))
	}
	assert.Zero(t, e.accepted(t))
}

func TestCIUpload_ObjectStoreUnavailable(t *testing.T) {
	// Stop a dedicated SeaweedFS so parallel intake tests keep their own store.
	c, err := testcontainers.Run(t.Context(), "chrislusf/seaweedfs:4.53",
		testcontainers.WithCmd("server", "-s3", "-dir=/data"), testcontainers.WithExposedPorts("8333/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("8333/tcp").WithStartupTimeout(time.Minute)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Terminate(context.Background())) })
	endpoint, err := c.PortEndpoint(t.Context(), "8333/tcp", "http")
	require.NoError(t, err)
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint), Region: "us-east-1", UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	})
	require.Eventually(t, func() bool {
		_, err := client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String("ci-packages")})
		return err == nil
	}, 30*time.Second, 200*time.Millisecond)
	store := objstore.New(objstore.Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "ci-packages", AccessKeyID: "test", SecretAccessKey: "test"})
	e := newCIEnvWithStore(t, store)
	require.NoError(t, c.Stop(t.Context(), nil))
	out := e.uploadCI(t, e.token.Value, ciPackage(t, nil))
	require.Equal(t, http.StatusServiceUnavailable, out.Code, out.Body.String())
	assert.NotEmpty(t, out.Header().Get("Retry-After"))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt"))
}

func (e *ciEnv) validateResponse(t *testing.T, req *http.Request, out *httptest.ResponseRecorder) {
	t.Helper()
	path := strings.TrimPrefix(req.URL.Path, "/api/v1")
	item := e.contract.Paths.Value(path)
	if item == nil {
		t.Fatalf("missing contract path %s", path)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, Route: &routers.Route{Spec: e.contract, Path: path, PathItem: item, Method: req.Method, Operation: item.GetOperation(req.Method)}},
		Status:                 out.Code, Header: out.Header(), Body: io.NopCloser(bytes.NewReader(out.Body.Bytes())),
	}
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), input), out.Body.String())
}

func ciMetric(t *testing.T, reg *prometheus.Registry, outcome, reason string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == "craftpulse_ingest_ci_packages_total" {
			for _, metric := range family.Metric {
				labels := map[string]string{}
				for _, l := range metric.Label {
					labels[l.GetName()] = l.GetValue()
				}
				if labels["outcome"] == outcome && labels["reason"] == reason {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func TestCIUpload_ConcurrentRepeat(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	data := ciPackage(t, nil)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 3)
	for range 3 {
		wg.Go(func() { results <- e.uploadCI(t, e.token.Value, data) })
	}
	wg.Wait()
	close(results)
	var first *acceptedBody
	duplicates := []acceptedBody{}
	for result := range results {
		require.Equal(t, http.StatusAccepted, result.Code, result.Body.String())
		body := decode[acceptedBody](t, result)
		if body.Status == "quarantined" {
			require.Nil(t, first)
			first = &body
		} else {
			duplicates = append(duplicates, body)
		}
	}
	require.NotNil(t, first)
	require.Len(t, duplicates, 2)
	for _, dup := range duplicates {
		require.NotNil(t, dup.DuplicateOf)
		assert.Equal(t, first.ReceiptID, *dup.DuplicateOf)
	}
	assert.Equal(t, 1, e.jobs(t, entity.KindIngestPackage))
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(first.ReceiptID)))
	assert.Equal(t, 1, e.accepted(t))
	assert.Empty(t, e.keys(t, "quarantine/"))
}
