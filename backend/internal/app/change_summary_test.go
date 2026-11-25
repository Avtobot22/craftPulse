package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

type summaryTestAdapter struct {
	source entity.AreaSource
	fetch  func(entity.FetchInput) []byte
}

func (a *summaryTestAdapter) AreaSources(context.Context, entity.ID, []entity.ID) ([]entity.AreaSource, error) {
	return []entity.AreaSource{a.source}, nil
}

func (a *summaryTestAdapter) Fetch(_ context.Context, _, _ entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(a.fetch(in))), nil
}

type summaryDecisions struct {
	change.Decisions
	active []entity.ExceptionVersion
}

func (d *summaryDecisions) ActiveAt(context.Context, entity.ID, entity.ID, int64, time.Time) ([]entity.ExceptionVersion, error) {
	return d.active, nil
}

func TestChangeSummary_ExactHeadAndReportLifecycle(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	project := uuid.UUID(e.project)
	profile := e.confirm(t, project, `"0"`, uuid.New(),
		row(e.goComponent, "go", "code", "builtin", "go-default"),
		row(e.goComponent, "go", "sast", "import", "sarif-import"),
		row(e.goComponent, "", "secrets", "builtin", "gitleaks-default"),
		row(e.goComponent, "", "sca", "builtin", "trivy-default"),
	)
	require.Equal(t, http.StatusCreated, profile.Code, profile.Body.String())
	emptySecurityReports := func(sha string) []byte {
		t.Helper()
		component := &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"}
		// Summary evidence must match the producer versions pinned in the test catalog.
		return reportPackageOf(t, _checksInstance, sha,
			reportSpec{
				name: "gitleaks.sarif", kind: "secrets", format: "sarif", producer: "gitleaks", version: "8.28.0",
				profileID: "gitleaks-default", component: component, exit: "success", completeness: "complete",
				content: []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gitleaks"}},"results":[]}]}`),
			},
			reportSpec{
				name: "trivy.json", kind: "sca", format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.67.2",
				profileID: "trivy-default", component: component, exit: "success", completeness: "complete",
				content: []byte(`{"SchemaVersion":2,"ArtifactName":"services/billing","ArtifactType":"filesystem","Results":[]}`),
			},
		)
	}
	base, h1, h2 := strings.Repeat("b", 40), strings.Repeat("a", 40), strings.Repeat("c", 40)
	mrPage := func(head, version string, at time.Time) []byte {
		t.Helper()
		m := mrManifest("42")
		m.Source.InstanceKey = _checksInstance
		r := mrRecord(t, "merge_request", "9001", "", version, envelope.MergeRequestV1{
			ProjectNativeId: "42", Iid: new(7), State: "open", HeadSha: head, BaseSha: &base,
			SourceBranch: "feature", TargetBranch: "main", OpenedAt: at, UpdatedAt: &at,
		})
		return pack(t, m, r)
	}
	// The CI step can finish before GitLab has collected either the MR or its pipeline.
	archive := ciPackage(t, func(files map[string][]byte) {
		for _, name := range []string{envelope.ScanManifestName, envelope.RecordsName} {
			files[name] = bytes.ReplaceAll(files[name], []byte("d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"), []byte(h1))
		}
	})
	uploaded := e.uploadCI(t, e.token.Value, archive)
	require.Equal(t, http.StatusAccepted, uploaded.Code, uploaded.Body.String())
	receipt := decode[acceptedBody](t, uploaded).ReceiptID
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(receipt)))
	e.drain(t)
	firstReceipt := e.readCI(t, receipt, e.token.Value)
	require.Equal(t, "accepted", firstReceipt.Status)
	assert.Nil(t, firstReceipt.ResultURL)

	// Even a collected pipeline cannot replace an explicit MR target from the package.
	pipelineManifest := ciManifest("42")
	pipelineManifest.Source.InstanceKey = _checksInstance
	e.submit(t, pack(t, pipelineManifest, mrRecord(t, "ci_run", "5001", "", "v1", envelope.CiRunV1{
		ProjectNativeId: "42", RefName: "feature", Sha: h1, MrNativeId: new("9001"),
		NativeStatus: "running", Outcome: "running", StartedAt: new(e.clock.Now()),
	})))
	assert.Nil(t, e.readCI(t, receipt, e.token.Value).ResultURL)

	e.clock.Advance(time.Second)
	first := e.submit(t, mrPage(h1, "v1", e.clock.Now()))
	require.NotNil(t, first.SourceInstanceID)
	mr := mrID(t, e.normalized, "9001")
	assert.Nil(t, e.readCI(t, receipt, e.token.Value).ResultURL)
	job := entity.ChangeSummaryJob{OrganizationID: e.orgID, MRID: mr}
	source := &summaryTestAdapter{source: entity.AreaSource{
		InstanceID: *first.SourceInstanceID, Kind: entity.ScopeRepository, NativeScopeID: "42",
		Active: true, ConnectionStatus: entity.ConnectionActive,
	}}
	page := mrPage(h1, "v1", e.clock.Now())
	onFetch := func() {}
	source.fetch = func(in entity.FetchInput) []byte {
		assert.Equal(t, "merge_requests", in.SchemaID)
		assert.Equal(t, []entity.ObjectKey{{ObjectKind: "merge_request", NativeID: "9001"}}, in.ObjectKeys)
		onFetch()
		return page
	}
	deps, err := newSummaryDeps(e.app, source, e.norm, new(calc.UseCase), e.store, prometheus.NewRegistry())
	require.NoError(t, err)
	decisions := &summaryDecisions{Decisions: e.decisions}
	u := newSummarizer(e.app, e.norm, e.orgs, e.clock, decisions, deps, e.catalog)
	count := e.normalized.n
	run := func() {
		t.Helper()
		var id int64
		require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id FROM river_job
			WHERE kind = 'change.summary' AND state = 'available'`).Scan(&id))
		require.NoError(t, u.RunSummary(t.Context(), job, completeJob(id)))
	}
	enqueue := func() {
		t.Helper()
		tx := persistent.NewTransactor(e.app)
		require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
			return deps.Queue.Enqueue(ctx, job, entity.QueueInteractive)
		}))
	}
	checkReady := func(head, state string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		var gotHead, gotState string
		require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id, head_sha, pipeline_state
			FROM change.summary_revision WHERE mr_id = $1 AND status = 'ready'`, uuid.UUID(mr)).Scan(&id, &gotHead, &gotState))
		assert.Equal(t, head, gotHead)
		assert.Equal(t, state, gotState)
		return id
	}
	assertItem := func(id uuid.UUID, kind, reason string, n int) {
		t.Helper()
		assert.Equal(t, n, count(t, `SELECT count(*) FROM change.summary_item
			WHERE summary_revision_id = $1 AND item_kind = $2 AND COALESCE(reason_code,'') = $3`, id, kind, reason))
	}

	br := e.submit(t, e.codeReport(t, base, "feature", "8101", sarifOf(
		hit{rule: "fixed", path: "src/main.go", line: 2},
		hit{rule: "accepted", path: "src/main.go", line: 3},
	)))
	finds := e.identities(t, br)
	require.Len(t, finds, 2)
	require.Equal(t, "accepted", finds[0].rule)
	decision := entity.ID(uuid.New())
	decisions.active = []entity.ExceptionVersion{{
		ID: decision, FindingKind: entity.FindingCode, Status: entity.ExceptionApproved,
		ExceptionDraft: entity.ExceptionDraft{EffectiveFrom: e.clock.Now(), ExpiresAt: e.clock.Now().Add(time.Hour), Scope: entity.ExceptionScope{
			AreaID: entity.ID(e.areas["42"]), ComponentID: entity.ID(e.goComponent), FindingID: entity.ID(finds[0].identity),
		}},
	}}
	e.submit(t, emptySecurityReports(h1))
	e.submit(t, e.codeReport(t, h1, "feature", "8102", sarifOf(), "success", "complete"))
	run()
	ready := checkReady(h1, "running")
	assertItem(ready, "resolved", "", 1)
	assertItem(ready, "accepted", "", 1)
	assertItem(ready, "not_evaluated", "scan_missing", 1)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM change.summary_input
		WHERE summary_revision_id = $1 AND input_kind = 'decision_version' AND input_id = $2`, ready, uuid.UUID(decision)))

	sast := func(sha, name string, content []byte) []byte {
		t.Helper()
		return reportPackageOf(t, _checksInstance, sha, reportSpec{
			name: name + ".sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1",
			profileID: "sarif-import", mode: envelope.ScanToolRunModeImport,
			component: &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"},
			content:   content, exit: "findings", completeness: "complete",
		})
	}
	e.submit(t, sast(h1, "h1", sarifOf()))
	e.clock.Advance(time.Second)
	fetches := 0
	page2 := mrPage(h2, "v2", e.clock.Now())
	onFetch = func() {
		fetches++
		if fetches == 2 {
			page = page2
		}
	}
	run()
	require.Equal(t, 2, fetches)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM change.summary_revision WHERE status = 'stale' AND stale_reason = 'new_head'`))
	assert.Zero(t, count(t, `SELECT count(*) FROM river_job WHERE kind = 'outbound.publish'`))
	onFetch = func() {}

	// Head A remains the target as the MR advances.
	link := e.readCI(t, receipt, e.token.Value).ResultURL
	require.NotNil(t, link)
	assert.Equal(t, "/merge-requests/"+mr.String()+"/summary?head_sha="+h1, *link)

	e.submit(t, page2)
	e.submit(t, emptySecurityReports(h2))
	manifest := ciManifest("42")
	manifest.Source.InstanceKey = _checksInstance
	e.submit(t, pack(t, manifest, mrRecord(t, "ci_run", "58101", "", "v1", envelope.CiRunV1{
		ProjectNativeId: "42", RefName: "feature", Sha: h2, MrNativeId: new("9001"), NativeStatus: "success",
		Outcome: "success", StartedAt: new(e.clock.Now()), FinishedAt: new(e.clock.Now()),
	}), mrRecord(t, "ci_attempt", "70101", "", "v1", envelope.CiAttemptV1{
		RunNativeId: "58101", JobKey: "craftpulse-go-default", JobName: "craftpulse-go-default",
		ConfigFingerprint: strings.Repeat("a", 64), AttemptNo: 1, NativeStatus: "success", Outcome: "success",
	})))
	run()
	ready = checkReady(h2, "reports_missing")
	assert.Equal(t, link, e.readCI(t, receipt, e.token.Value).ResultURL)
	assertItem(ready, "not_evaluated", "scan_missing", 2)
	assertItem(ready, "resolved", "", 0)

	// A completed child may have its own SHA and no MR link. Its parent still
	// points to the selected pipeline, so it updates CI metrics for the MR.
	e.submit(t, pack(t, manifest, mrRecord(t, "ci_run", "58102", "", "v1", envelope.CiRunV1{
		ProjectNativeId: "42", RefName: "feature", Sha: h1, ParentRunNativeId: new("58101"), NativeStatus: "success",
		Outcome: "success", StartedAt: new(e.clock.Now()), FinishedAt: new(e.clock.Now()),
	})))
	run()
	withChild := checkReady(h2, "reports_missing")
	assert.NotEqual(t, ready, withChild)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM change.summary_item WHERE summary_revision_id = $1
		AND item_kind = 'ci_attempts' AND jsonb_array_length(value->'run_versions') = 2`, withChild))

	e.submit(t, e.codeReport(t, h2, "feature", "8104", sarifOf(), "failed", "partial"))
	e.submit(t, sast(h2, "rejected", []byte(`{"version":"2.0.0","runs":[]}`)))
	run()
	ready = checkReady(h2, "reports_missing")
	assertItem(ready, "not_evaluated", "scan_failed", 1)
	assertItem(ready, "not_evaluated", "import_failed", 1)

	hits := make([]hit, 0, 6)
	for i := range 6 {
		hits = append(hits, hit{rule: fmt.Sprintf("new-%d", i), path: "src/main.go", line: i + 10})
	}
	// A comparable SAST base makes the six findings new. The head report lands while the job is active.
	e.submit(t, sast(base, "base", sarifOf()))
	e.submit(t, e.codeReport(t, h2, "feature", "8105", sarifOf(), "success", "complete"))
	fetches = 0
	onFetch = func() {
		fetches++
		if fetches == 2 {
			e.submit(t, sast(h2, "h2", sarifOf(hits...)))
		}
	}
	run()
	assert.Equal(t, ready, checkReady(h2, "reports_missing"))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM river_job WHERE kind = 'change.summary' AND state = 'available'`))
	onFetch = func() {}
	run()
	ready = checkReady(h2, "complete")
	assert.Equal(t, 6, count(t, `SELECT count(*) FROM change.summary_item WHERE summary_revision_id = $1
		AND item_kind = 'new' AND required_right = 'security'`, ready))
	assert.Equal(t, 5, count(t, `SELECT count(*) FROM change.summary_item WHERE summary_revision_id = $1 AND rank BETWEEN 1 AND 5`, ready))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM river_job WHERE kind = 'outbound.publish'
		AND args->>'summary_revision_id' = $1`, ready.String()))

	// Use the HTTP entry point and the same revisions built from imported facts.
	security := e.exceptionActor(t, "summary-security", []roleInput{
		{Role: "tech_lead", ProjectID: &project}, {Role: "security_access", ProjectID: &project},
	})
	outside := e.exceptionActor(t, "summary-outside", []roleInput{{Role: "tech_lead", ProjectID: &e.other}})
	// These fixtures have no live source; the test still exercises project visibility.
	_, err = e.owner.Exec(t.Context(), "DELETE FROM connect.connection_scope WHERE organization_id=$1 AND area_id=$2", uuid.UUID(e.orgID), e.areas["42"])
	require.NoError(t, err)
	reg := prometheus.NewRegistry()
	views := newView(e.app, viewModules{code: e.norm, rights: e.rights}, e.clock)
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg,
		apiModules{orgs: e.orgs, access: e.rights, code: views})
	e.router = httpctrl.NewRouter(zerolog.New(e.logs), reg, health.Probes{Ready: &health.Readiness{}}, api)
	summaryPath := "/api/v1/merge-requests/" + mr.String() + "/summary"
	read := func(b *browser, suffix string) openapi.MergeRequestSummary {
		t.Helper()
		return decode[openapi.MergeRequestSummary](t, codeRead(t, e.loginEnv, b, summaryPath+suffix, "MergeRequestSummary"))
	}
	full := read(security, "")
	plain := read(e.lead, "")
	assert.Equal(t, ready, full.Id)
	assert.Equal(t, h2, plain.HeadSha)
	require.NotNil(t, full.Expected)
	assert.Equal(t, openapi.DataState("ASSESSED"), full.Expected.MissingChecks.State)
	assert.Equal(t, new(float32(3)), full.Expected.MissingChecks.Numerator)
	assert.Equal(t, new(float32(4)), full.Expected.MissingChecks.Denominator)
	assert.Equal(t, new(float32(0.75)), full.Expected.MissingChecks.Value)
	receiptPath := "/api/v1" + *link
	fromReceipt := decode[openapi.MergeRequestSummary](t, codeRead(t, e.loginEnv, security, receiptPath, "MergeRequestSummary"))
	assert.Equal(t, h1, fromReceipt.HeadSha)
	assert.Equal(t, "stale", string(fromReceipt.Status))
	assert.Equal(t, http.StatusNotFound, e.do(t, security, http.MethodGet,
		fmt.Sprintf("%s&revision_no=%d", receiptPath, full.RevisionNo), nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(t, outside, http.MethodGet, receiptPath, nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(t, security, http.MethodGet, summaryPath+"?head_sha="+strings.Repeat("0", 40), nil).Code)
	assert.Empty(t, full.Redactions)
	assert.Equal(t, []string{"items"}, plain.Redactions)
	assert.True(t, plain.AccessLimited)
	assert.Len(t, full.Items, len(plain.Items)+6)
	require.NotNil(t, full.Ci)
	assert.Equal(t, uuid.UUID(runID(t, e.normalized, "58101")), full.Ci.PipelineId)
	for i, item := range plain.Items {
		assert.Equal(t, i+1, item.ItemNo)
		assert.Nil(t, item.Rank)
		assert.NotEqual(t, new(openapi.AnalysisKind("sast")), item.AnalysisKind)
	}
	assert.Equal(t, "pending", string(full.Publication.Status))
	_, err = e.owner.Exec(t.Context(), `UPDATE change.summary_revision SET publication_status = 'succeeded',
		publication_external_id = 'comment-1', published_at = built_at WHERE id = $1`, ready)
	require.NoError(t, err)
	published := read(security, "").Publication
	assert.Equal(t, "succeeded", string(published.Status))
	assert.Equal(t, new("comment-1"), published.ExternalId)
	assert.NotNil(t, published.PublishedAt)
	old := read(security, "?revision_no=1")
	assert.Equal(t, &base, old.BaseSha)
	assert.Equal(t, h1, old.HeadSha)
	assert.Equal(t, "stale", string(old.Status))
	assert.Equal(t, "not_requested", string(old.Publication.Status))
	assert.Equal(t, http.StatusNotFound, e.do(t, outside, http.MethodGet, summaryPath, nil).Code)

	enqueue()
	run()
	assert.Equal(t, ready, checkReady(h2, "complete"))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM river_job WHERE kind = 'outbound.publish'
		AND args->>'summary_revision_id' = $1`, ready.String()))

	// An unrelated base SHA leaves complete head findings newly observed and keeps the last good revision.
	base = strings.Repeat("e", 40)
	h3 := strings.Repeat("d", 40)
	e.clock.Advance(time.Second)
	page = mrPage(h3, "v3", e.clock.Now())
	e.submit(t, page)
	e.submit(t, e.codeReport(t, h3, "feature", "8106", sarifOf(hit{rule: "observed", path: "src/main.go", line: 3})))
	run()
	assertItem(checkReady(h3, "running"), "newly_observed", "", 1)
	current := read(security, "")
	assert.Nil(t, current.BaseSha)
	// A pre-upgrade revision stored the source base SHA even without comparison evidence.
	db := stdlib.OpenDBFromPool(e.owner)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Core())
	require.NoError(t, err)
	_, err = provider.DownTo(t.Context(), 57)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `UPDATE change.summary_revision SET base_sha = $1, status = 'superseded'
		WHERE organization_id = $2 AND id = $3`, base, uuid.UUID(e.orgID), current.Id)
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	assert.Equal(t, old.BaseSha, read(security, "?revision_no=1").BaseSha)
	legacy := read(security, fmt.Sprintf("?revision_no=%d", current.RevisionNo))
	assert.Equal(t, current.Id, legacy.Id)
	assert.Equal(t, "superseded", string(legacy.Status))
	assert.Nil(t, legacy.BaseSha)
	require.NotNil(t, current.LastGood)
	assert.Equal(t, h2, current.LastGood.HeadSha)
	tx := persistent.NewTransactor(e.app)
	require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		last, err := deps.Store.LastGoodSummary(ctx, e.orgID, mr)
		require.NoError(t, err)
		assert.Equal(t, ready, uuid.UUID(last.ID))
		assert.Equal(t, h2, last.HeadSHA)
		assert.Equal(t, "stale", last.Status)
		assert.False(t, last.BuiltAt.IsZero())
		return nil
	}))
	// Another MR in the same visible repository has facts but has not been summarized.
	manifestMR := mrManifest("42")
	manifestMR.Source.InstanceKey = _checksInstance
	e.submit(t, pack(t, manifestMR, mrRecord(t, "merge_request", "9002", "", "v1", envelope.MergeRequestV1{
		ProjectNativeId: "42", Iid: new(8), State: "open", HeadSha: h3,
		SourceBranch: "other", TargetBranch: "main", OpenedAt: e.clock.Now(),
	})))
	noRevision := "/api/v1/merge-requests/" + mrID(t, e.normalized, "9002").String() + "/summary"
	assert.Equal(t, http.StatusNotFound, e.do(t, e.lead, http.MethodGet, noRevision, nil).Code)
}
