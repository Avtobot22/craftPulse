package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/objstore"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	ingestrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/ingest"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var (
	_seaweedOnce sync.Once
	_seaweedCfg  objstore.Config
	_seaweedErr  error
)

// sharedStore is one SeaweedFS for the package; keys carry the organization, so tests do not meet.
func sharedStore(t *testing.T) *objstore.Store {
	t.Helper()
	_seaweedOnce.Do(func() {
		ctx := context.Background()
		c, err := testcontainers.Run(ctx, "chrislusf/seaweedfs:4.53",
			testcontainers.WithCmd("server", "-s3", "-dir=/data"),
			testcontainers.WithExposedPorts("8333/tcp"),
			testcontainers.WithWaitStrategy(wait.ForListeningPort("8333/tcp").WithStartupTimeout(time.Minute)),
		)
		if err != nil {
			_seaweedErr = err
			return
		}
		endpoint, err := c.PortEndpoint(ctx, "8333/tcp", "http")
		if err != nil {
			_seaweedErr = err
			return
		}
		_seaweedCfg = objstore.Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "packages", AccessKeyID: "test", SecretAccessKey: "test"}
		admin := s3.New(s3.Options{
			BaseEndpoint: aws.String(endpoint), Region: _seaweedCfg.Region, UsePathStyle: true,
			Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
		})
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err = admin.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(_seaweedCfg.Bucket)}); err == nil || time.Now().After(deadline) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		_seaweedErr = err
	})
	require.NoError(t, _seaweedErr)
	return objstore.New(_seaweedCfg)
}

type ingestEnv struct {
	*loginEnv
	store   *objstore.Store
	connect *connect.UseCase
	uc      *ingest.UseCase
	reg     *prometheus.Registry
	admin   entity.Subject
	member  entity.Subject
	project entity.ID
}

func newIngestEnv(t *testing.T) *ingestEnv {
	t.Helper()
	e := &ingestEnv{loginEnv: newLoginEnv(t), store: sharedStore(t), reg: prometheus.NewRegistry()}
	e.admin = entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.loginAs(t, _adminSub))}
	e.member = entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.loginAs(t, _otherSub))}
	e.project = entity.ID(e.tokenProject(t, e.orgID))
	caller, err := adapterapi.NewCaller(adapterapi.Config{Resolver: mapResolver{}})
	require.NoError(t, err)
	e.connect, err = newConnect(e.app, e.keyring, caller, e.clock)
	require.NoError(t, err)
	_, rights := newOrgAccess(e.app, e.clock)
	e.uc, err = newIngest(e.app, e.store, e.connect, rights, e.clock, e.reg)
	require.NoError(t, err)
	return e
}

func (e *ingestEnv) loginAs(t *testing.T, subject string) uuid.UUID {
	t.Helper()
	e.login(t, newBrowser(), subject)
	return e.userID(t, e.orgID, subject)
}

func importManifest(version string) *envelope.Manifest {
	return &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaIdTasks, SchemaVersion: version,
		Source:       envelope.Source{Provider: envelope.SourceProviderJiraCloud, InstanceKey: _jiraInstance},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindTrackerProject, NativeId: "10000"},
		Completeness: envelope.ManifestCompletenessComplete,
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		Producer:     envelope.Producer{Name: "ingest-test", Version: "1"},
		CreatedAt:    time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC),
	}
}

// task is a task record from the schema example; title changes its content, revision sets source_revision.
func task(t *testing.T, id, title, revision string) *envelope.Record {
	t.Helper()
	var schema envelope.RecordSchema
	for _, s := range envelope.RecordSchemas() {
		if s.Kind == "task" {
			schema = s
		}
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal(schema.Example, &payload))
	payload["title"] = title
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	rec := &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/" + schema.Version, NativeId: id, Payload: raw}
	if revision != "" {
		rec.SourceRevision = &revision
	}
	return rec
}

func pack(t *testing.T, m *envelope.Manifest, recs ...*envelope.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	for _, r := range recs {
		require.NoError(t, w.AddRecord(r))
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// imported submits a package and runs its job as the worker does.
func (e *ingestEnv) imported(t *testing.T, s entity.Subject, data []byte) entity.Receipt {
	t.Helper()
	rc, err := e.uc.SubmitImport(t.Context(), s, e.project, bytes.NewReader(data))
	require.NoError(t, err)
	if rc.Status == entity.ReceiptQuarantined {
		require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, rc.ID))
	}
	return e.receipt(t, rc.ID)
}

func (e *ingestEnv) receipt(t *testing.T, id entity.ID) entity.Receipt {
	t.Helper()
	var rc entity.Receipt
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		var err error
		rc, err = ingestrepo.NewRepo(e.app).Receipt(ctx, e.orgID, id)
		return err
	}))
	return rc
}

func (e *ingestEnv) records(t *testing.T, id entity.ID) []entity.IngestRecord {
	t.Helper()
	var out []entity.IngestRecord
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		var err error
		out, err = ingestrepo.NewRepo(e.app).Records(ctx, e.orgID, id)
		return err
	}))
	return out
}

func (e *ingestEnv) accepted(t *testing.T) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'accepted'")
}

func (e *ingestEnv) keys(t *testing.T, prefix string) []string {
	t.Helper()
	keys, err := e.store.List(t.Context(), prefix+e.orgID.String()+"/")
	require.NoError(t, err)
	return keys
}

type rawEntry struct {
	name    string
	flag    byte
	content []byte
	size    int64
	link    string
}

// raw builds an archive by hand: the writer of the package refuses what these tests send.
func raw(t *testing.T, entries ...rawEntry) []byte {
	t.Helper()
	manifest, err := json.Marshal(importManifest("1"))
	require.NoError(t, err)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, en := range append([]rawEntry{{name: envelope.ManifestName, content: manifest}}, entries...) {
		h := &tar.Header{Name: en.name, Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(len(en.content)), Linkname: en.link}
		if en.flag != 0 {
			h.Typeflag = en.flag
		}
		if en.size > 0 {
			h.Size = en.size
		}
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
		}
		require.NoError(t, tw.WriteHeader(h))
		switch {
		case en.size > 0:
			_, err = tw.Write(make([]byte, en.size))
		case h.Typeflag == tar.TypeReg:
			_, err = tw.Write(en.content)
		}
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestIngest_RefusesUnsafePackagesAtOnce(t *testing.T) {
	e := newIngestEnv(t)
	large := make([]byte, envelope.DefaultLimits.MaxPackageBytes+1)
	_, err := rand.Read(large)
	require.NoError(t, err)
	many := make([]rawEntry, 1000)
	for i := range many {
		many[i] = rawEntry{name: "files/" + uuid.NewString()}
	}
	cases := map[string]struct {
		data   []byte
		reason string
	}{
		"package over 25 MiB":   {large, entity.PackageLimitExceeded},
		"unpacked over 100 MiB": {raw(t, rawEntry{name: "files/big", size: envelope.DefaultLimits.MaxUnpackedBytes + 1}), entity.PackageLimitExceeded},
		"1 001 files":           {raw(t, many...), entity.PackageLimitExceeded},
		"parent path":           {raw(t, rawEntry{name: "../x", content: []byte("x")}), entity.PackagePathUnsafe},
		"absolute path":         {raw(t, rawEntry{name: "/x", content: []byte("x")}), entity.PackagePathUnsafe},
		"symbolic link":         {raw(t, rawEntry{name: "files/link", flag: tar.TypeSymlink, link: "/etc/passwd"}), entity.PackagePathUnsafe},
		"external XML entity": {raw(t, rawEntry{
			name:    "files/report.xml",
			content: []byte(`<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]><r>&x;</r>`),
		}), entity.PackageEnvelopeInvalid},
	}
	for name, c := range cases {
		rc, err := e.uc.SubmitImport(t.Context(), e.admin, e.project, bytes.NewReader(c.data))
		require.NoError(t, err, name)
		assert.Equal(t, entity.ReceiptRejected, rc.Status, name)
		assert.Equal(t, c.reason, rc.ReasonCode, name)
		assert.Equal(t, entity.ReceiptRejected, e.receipt(t, rc.ID).Status, "%s: the refusal is in the journal", name)
	}
	assert.Empty(t, e.keys(t, "quarantine/"), "no quarantine object is left")
	assert.Empty(t, e.keys(t, "packages/"), "nothing refused is stored")
	assert.InDelta(t, 3, gathered(t, e.reg, "craftpulse_ingest_rejected_packages_total", "limit_exceeded"), 0, "rejected packages are counted by reason")
}

func gathered(t *testing.T, reg *prometheus.Registry, name, label string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == label {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func TestIngest_JournalOfImports(t *testing.T) {
	e := newIngestEnv(t)

	partial := e.imported(t, e.admin, pack(t, importManifest("1"), task(t, "IMP-1", "A", ""), task(t, "", "B", "")))
	assert.Equal(t, entity.ReceiptPartial, partial.Status)
	assert.Equal(t, 1, partial.AcceptedCount)
	assert.Equal(t, 1, partial.RejectedCount)
	recs := e.records(t, partial.ID)
	require.Len(t, recs, 2)
	assert.Equal(t, entity.RecordAccepted, recs[0].Outcome)
	assert.Equal(t, entity.RecordRejected, recs[1].Outcome)
	assert.Equal(t, entity.RecordMissingSourceID, recs[1].ReasonCode)
	assert.Equal(t, 1, e.jobs(t, entity.KindNormalizeBatch), "accepted records go to normalize in the same transaction")
	assert.Empty(t, e.keys(t, "quarantine/"), "the quarantine object goes after the commit")
	assert.Equal(t, []string{"packages/" + e.orgID.String() + "/" + partial.ID.String() + ".tar.gz"}, e.keys(t, "packages/"))

	unsupported := e.imported(t, e.admin, pack(t, importManifest("9"), task(t, "IMP-1", "changed", "")))
	assert.Equal(t, entity.ReceiptRejected, unsupported.Status)
	assert.Equal(t, entity.PackageSchemaUnsupported, unsupported.ReasonCode)
	assert.Equal(t, 1, e.accepted(t), "the refused package does not touch the accepted version")

	first := pack(t, importManifest("1"), task(t, "IMP-2", "C", ""))
	original := e.imported(t, e.admin, first)
	require.Equal(t, entity.ReceiptAccepted, original.Status)
	for range 3 {
		again := e.imported(t, e.admin, first)
		assert.Equal(t, entity.ReceiptDuplicate, again.Status)
		require.NotNil(t, again.DuplicateOf)
		assert.Equal(t, original.ID, *again.DuplicateOf)
	}
	assert.Equal(t, 2, e.accepted(t), "repeats add no accepted record")

	replay := e.imported(t, e.admin, pack(t, importManifest("1"), task(t, "IMP-2", "C", ""), task(t, "IMP-3", "D", "")))
	recs = e.records(t, replay.ID)
	require.Len(t, recs, 2)
	assert.Equal(t, entity.RecordReplay, recs[0].Outcome, "the same record in a new package is a replay")
	require.NotNil(t, recs[0].FirstReceiptID)
	assert.Equal(t, original.ID, *recs[0].FirstReceiptID)
	assert.Equal(t, entity.RecordAccepted, recs[1].Outcome)

	before := e.imported(t, e.admin, pack(t, importManifest("1"), task(t, "IMP-4", "E", "7")))
	conflict := e.imported(t, e.admin, pack(t, importManifest("1"), task(t, "IMP-4", "E changed", "7")))
	recs = e.records(t, conflict.ID)
	require.Len(t, recs, 1)
	assert.Equal(t, entity.RecordConflict, recs[0].Outcome, "the same source_revision with another content")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.conflict WHERE status = 'open' AND existing_receipt_id = '"+before.ID.String()+"'"))
	assert.Equal(t, entity.RecordAccepted, e.records(t, before.ID)[0].Outcome, "the accepted fact is left as it is")

	refused := e.imported(t, e.member, pack(t, importManifest("1"), task(t, "IMP-5", "F", "")))
	assert.Equal(t, entity.ReceiptPartial, refused.Status)
	recs = e.records(t, refused.ID)
	require.Len(t, recs, 1)
	assert.Equal(t, entity.RecordOutsideScope, recs[0].ReasonCode, "a user without the right to import")
}

// page writes a page of sync in its own transaction, as sync does with its checkpoint.
func (e *ingestEnv) page(t *testing.T, conn, scope entity.ID, data []byte) entity.Receipt {
	t.Helper()
	p, err := e.uc.PreparePage(t.Context(), e.orgID, conn, scope, bytes.NewReader(data))
	require.NoError(t, err)
	var rc entity.Receipt
	require.NoError(t, persistent.NewTransactor(e.app).InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		rc, err = e.uc.RecordPage(ctx, p)
		return err
	}))
	return rc
}

func (e *ingestEnv) scope(t *testing.T, native string) (conn, scope entity.ID) {
	t.Helper()
	inst, err := e.connect.EnsureInstance(t.Context(), e.orgID, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	conn = entity.ID(uuid.Must(uuid.NewV7()))
	r := connectrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		err := r.CreateConnection(ctx, entity.Connection{
			ID: conn, OrganizationID: e.orgID, InstanceID: inst.ID, Name: "Jira", AdapterKind: entity.AdapterBuiltin,
			ContractVersion: "1", AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionActive, RowVersion: 1,
			CreatedBy: e.admin.UserID, CreatedAt: e.clock.Now(),
		})
		if err != nil {
			return err
		}
		scope, err = r.AddScope(ctx, e.orgID, conn, inst.ID, entity.NewScope{Kind: entity.ScopeTrackerProject, NativeScopeID: native, DisplayName: native}, e.clock.Now())
		return err
	}))
	return conn, scope
}

func TestIngest_PagesOfSync(t *testing.T) {
	e := newIngestEnv(t)
	conn, scope := e.scope(t, "10000")

	first := e.page(t, conn, scope, pack(t, importManifest("1"), task(t, "J-1", "A", "")))
	require.Equal(t, entity.ReceiptAccepted, first.Status)
	whole := pack(t, importManifest("1"), task(t, "J-2", "B", ""))
	cut := e.page(t, conn, scope, whole[:len(whole)/2])
	assert.Equal(t, entity.ReceiptRejected, cut.Status, "a truncated page")
	assert.Equal(t, entity.PackageEnvelopeInvalid, cut.ReasonCode)
	require.NotNil(t, cut.ScopeID)
	assert.Equal(t, scope, *cut.ScopeID, "the receipt names the scope the page was for")
	assert.Equal(t, entity.RecordAccepted, e.records(t, first.ID)[0].Outcome, "the journal of earlier pages is unchanged")
	assert.Equal(t, 1, e.accepted(t))

	other := importManifest("1")
	other.Area.NativeId = "20000"
	outside := e.page(t, conn, scope, pack(t, other, task(t, "J-3", "C", "")))
	assert.Equal(t, entity.ReceiptPartial, outside.Status)
	assert.Equal(t, entity.RecordOutsideScope, e.records(t, outside.ID)[0].ReasonCode, "a page of a scope the connection does not collect")
}

func TestIngest_NewPayloadSchemaIsNoConflict(t *testing.T) {
	e := newIngestEnv(t)
	conn, scope := e.scope(t, "10000")
	inst, err := e.connect.EnsureInstance(t.Context(), e.orgID, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	event := func(schema string, hash byte) *entity.PreparedPackage {
		id := entity.ID(uuid.Must(uuid.NewV7()))
		sum := bytes.Repeat([]byte{hash}, 32)
		return &entity.PreparedPackage{
			Receipt: entity.Receipt{
				ID: id, OrganizationID: e.orgID, Channel: entity.ChannelSync, ConnectionID: &conn, ScopeID: &scope,
				ReceivedAt: e.clock.Now(), SourceInstanceID: &inst.ID, PackageHash: sum, ObjectKey: "packages/" + id.String(), RecordCount: 1,
			},
			Records: []entity.IngestRecord{{
				No: 1, Class: "event", Kind: "task_event", PayloadSchema: schema, InstanceID: inst.ID,
				NativeNS: "20001", NativeID: "30010/status", SourceRevision: "30010", ContentHash: sum,
			}},
		}
	}
	for i, schema := range []string{"task_event/1", "task_event/2"} {
		var rc entity.Receipt
		require.NoError(t, persistent.NewTransactor(e.app).InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
			rc, err = e.uc.RecordPage(ctx, event(schema, byte(i+1)))
			return err
		}))
		assert.Equal(t, entity.RecordAccepted, e.records(t, rc.ID)[0].Outcome, "%s with the earlier revision is accepted", schema)
	}
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.conflict"))
}

func TestIngest_ConcurrentDeliveriesOfOneRecord(t *testing.T) {
	e := newIngestEnv(t)
	ids := make([]entity.ID, 2)
	for i := range ids {
		rc, err := e.uc.SubmitImport(t.Context(), e.admin, e.project, bytes.NewReader(
			pack(t, importManifest("1"), task(t, "IMP-1", "same", ""), task(t, "IMP-"+uuid.NewString(), "own", ""))))
		require.NoError(t, err)
		ids[i] = rc.ID
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() { assert.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, id)) })
	}
	wg.Wait()
	outcomes := make([]entity.RecordOutcome, 0, len(ids))
	for _, id := range ids {
		outcomes = append(outcomes, e.records(t, id)[0].Outcome)
	}
	assert.ElementsMatch(t, []entity.RecordOutcome{entity.RecordAccepted, entity.RecordReplay}, outcomes)
}

// failingQueue stands for a worker stopped after the cleaned object was stored and before the commit.
type failingQueue struct {
	repo.JobQueue
	fail bool
}

func (q *failingQueue) Enqueue(ctx context.Context, job entity.Job, name string) error {
	if q.fail && job.Kind() == entity.KindNormalizeBatch {
		return errors.New("worker stopped")
	}
	return q.JobQueue.Enqueue(ctx, job, name)
}

func TestIngest_InterruptedJobGivesTheSameJournal(t *testing.T) {
	e := newIngestEnv(t)
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	q := &failingQueue{JobQueue: queue.New(jobs), fail: true}
	_, rights := newOrgAccess(e.app, e.clock)
	uc := ingest.New(ingest.Deps{
		Repo: ingestrepo.NewRepo(e.app), Objects: e.store, Queue: q, Tx: persistent.NewTransactor(e.app),
		Connect: e.connect, Access: rights, Metrics: ingestMetrics{rejected: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x"}, []string{"channel", "reason"})},
		Clock: e.clock, NewID: newID, Auditor: audit.New(auditrepo.NewRepo(e.app), persistent.NewTransactor(e.app), e.clock),
	})
	data := pack(t, importManifest("1"), task(t, "IMP-1", "A", ""), task(t, "", "B", ""), task(t, "IMP-2", "C", ""))
	rc, err := uc.SubmitImport(t.Context(), e.admin, e.project, bytes.NewReader(data))
	require.NoError(t, err)

	require.Error(t, uc.ProcessPackage(t.Context(), e.orgID, rc.ID))
	assert.Equal(t, entity.ReceiptQuarantined, e.receipt(t, rc.ID).Status, "nothing of the interrupted run is committed")
	assert.Empty(t, e.records(t, rc.ID))
	assert.Len(t, e.keys(t, "quarantine/"), 1, "the package waits in quarantine")

	q.fail = false
	require.NoError(t, uc.ProcessPackage(t.Context(), e.orgID, rc.ID))
	require.NoError(t, uc.ProcessPackage(t.Context(), e.orgID, rc.ID), "a repeat after the commit finds the receipt done")
	done := e.receipt(t, rc.ID)
	assert.Equal(t, entity.ReceiptPartial, done.Status)
	recs := e.records(t, rc.ID)
	outcomes := make([]entity.RecordOutcome, 0, len(recs))
	for _, r := range recs {
		outcomes = append(outcomes, r.Outcome)
	}
	assert.Equal(t, []entity.RecordOutcome{entity.RecordAccepted, entity.RecordRejected, entity.RecordAccepted}, outcomes,
		"the journal of one uninterrupted run")
	assert.Equal(t, 1, e.jobs(t, entity.KindNormalizeBatch))
	assert.Empty(t, e.keys(t, "quarantine/"))
}

func (e *ingestEnv) jobs(t *testing.T, kind string) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = '"+kind+"'")
}
