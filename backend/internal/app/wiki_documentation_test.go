package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	docsrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/docs"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/docs"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/view"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _wikiInstance = "https://wiki.example.test/confluence"

func wikiManifest() *envelope.Manifest {
	m := manifestOf(envelope.ManifestSchemaIdDocuments, envelope.SourceProviderConfluenceDc, _wikiInstance, "ENG")
	m.Area.Kind = envelope.AreaRefKindWikiSpace
	return m
}

func wikiRecord(t *testing.T, id, version, acl string, restriction *string, ancestors []string, hash *string, at time.Time) *envelope.Record {
	t.Helper()
	doc := envelope.DocumentV1{
		DocFormat: envelope.DocumentV1DocFormatConfluence, ContainerNativeId: "ENG", PageNativeId: &id, VersionRef: version,
		Title: new("Page " + id), AclState: envelope.DocumentV1AclState(acl), Restriction: restriction,
		ContentSha256: hash, UpdatedAt: &at, SourceUrl: new(_wikiInstance + "/pages/viewpage.action?pageId=" + id),
	}
	if ancestors != nil {
		doc.AncestorNativeIds = &ancestors
	}
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassObject, RecordKind: envelope.RecordRecordKindDocument,
		PayloadSchema: "document/1", NativeId: id, SourceRevision: &version, Payload: raw,
	}
}

func TestWikiDocumentationEnvelope(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	hash := strings.Repeat("ab", 32)
	data := pack(t, wikiManifest(),
		wikiRecord(t, "100", "1", "known", new("page:100"), nil, &hash, _t1),
		wikiRecord(t, "101", "1", "known", new("ancestor:100"), []string{"100"}, &hash, _t1),
		wikiRecord(t, "102", "1", "unknown", nil, []string{"100"}, &hash, _t1),
		wikiRecord(t, "103", "1", "known", nil, nil, nil, _t1.Add(-time.Hour)),
		wikiRecord(t, "103", "2", "known", nil, nil, &hash, _t2),
		wikiRecord(t, "104", "1", "known", nil, []string{"103"}, &hash, _t2),
	)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	require.Positive(t, e.drain(t))
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE doc_format = 'confluence'"))
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'document' AND restricted"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE page_native_id = '102' AND acl_state = 'unknown'"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE page_native_id = '103' AND version_ref = '1' AND content_hash IS NULL"))
	assert.Positive(t, e.n(t, "SELECT count(*) FROM normalize.area_change WHERE 'documents' = ANY(fact_kinds)"))
	assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
	e.renormalize(t, rc.ID)
	e.drain(t)
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE doc_format = 'confluence'"))

	area, err := e.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectKind(entity.ScopeWikiSpace), "ENG")
	require.NoError(t, err)
	seq := int64(e.n(t, "SELECT max(known_seq) FROM normalize.document_version"))
	all, err := e.norm.WikiDocumentsAt(t.Context(), e.orgID, area, "", seq)
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.True(t, all[0].Restricted)
	assert.True(t, all[1].Restricted)
	assert.Equal(t, "2", all[2].VersionRef)
	assert.Equal(t, "Page 103", all[2].Title)
	assert.Equal(t, new(_wikiInstance+"/pages/viewpage.action?pageId=103"), all[2].SourceURL)
	assert.Equal(t, 32, len(all[2].ContentHash))
	current, err := e.norm.CurrentWikiVersions(t.Context(), e.orgID, area, []string{"100", "101", "102", "103"})
	require.NoError(t, err)
	assert.Len(t, current, 3)
	assert.Equal(t, "2", current[entity.ObjectKey{ObjectKind: "document", NativeID: "103"}].Revision)
	first, err := e.norm.WikiVersionBoundary(t.Context(), e.orgID, area, seq, _t2)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.WithinDuration(t, _t1.Add(-time.Hour), *first, 0)

	service := entity.ID(uuid.New())
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.service (organization_id, id, name, status, created_at) VALUES ($1,$2,'Wiki','active',now())", e.orgID, service)
	require.NoError(t, err)
	//nolint:forbidigo // Test fixture for an already-authorized source scope.
	access := entity.GrantScope(entity.ScopeGrant{Services: []entity.ID{service}, Areas: []entity.ID{area}, Objects: []entity.ID{all[0].DocumentID}})
	store := platform.NewRepo(e.app)
	tx := persistent.NewTransactor(e.app)
	rights := wikiAccess{scope: access, t: t}
	docModule := docs.New(docs.Deps{
		Repo: docsrepo.NewRepo(e.app), Tenants: store, Tx: tx, Access: rights,
		References: wikiReferences{}, Areas: e.norm, Documents: e.norm, Auditor: wikiAuditor{}, Clock: e.clock, NewID: newID,
	})
	roots, err := docModule.ReplaceRoots(t.Context(), e.admin, service, 0, entity.ID(uuid.New()), []entity.DocumentationRoot{
		{AreaID: area, ProfileID: entity.WikiInventoryProfile, Audience: "team"},
		{AreaID: area, ProfileID: entity.WikiInventoryProfile, Audience: "team", PathPrefix: "103"},
	})
	require.NoError(t, err)
	require.Len(t, roots.Roots, 2)
	for i, want := range [][]string{{"100", "103", "104"}, {"103", "104"}} {
		inv, err := docModule.InventoryAt(t.Context(), e.orgID, access, entity.DocumentationScope{
			ServiceID: service, RootID: roots.Roots[i].ID, RepositoryID: area,
		}, roots.KnownSeq)
		require.NoError(t, err)
		require.NotNil(t, inv)
		var ids []string
		for _, doc := range inv.Documents {
			ids = append(ids, doc.PageNativeID)
		}
		assert.Equal(t, want, ids)
	}

	list := view.New(view.Deps{Tx: tx, Documentation: view.DocumentationDeps{
		Roots: docModule, Access: rights, Tenants: store,
	}})
	page, err := list.ListServiceDocuments(t.Context(), e.admin, access, service, entity.ID{}, 20)
	require.NoError(t, err)
	ids := make([]string, 0, len(page.Items))
	for i := range page.Items {
		item := &page.Items[i]
		ids = append(ids, item.PageNativeID)
		assert.Equal(t, new(_wikiInstance+"/pages/viewpage.action?pageId="+item.PageNativeID), item.SourceURL)
	}
	assert.ElementsMatch(t, []string{"100", "103", "104"}, ids)

	t.Run("source manifest and knowledge cutoff", func(t *testing.T) {
		e.checkWikiReconciliation(t, *rc.SourceInstanceID, service, area, roots.Roots[0].ID)
	})

	t.Run("same-version observations", func(t *testing.T) {
		e.checkWikiObservations(t, area, hash)
	})
}

func (e *normalizeEnv) checkWikiObservations(t *testing.T, area entity.ID, hash string) {
	t.Helper()
	m := wikiManifest()
	at := m.CreatedAt
	importPage := func(observedAt time.Time, acl string, content *string) {
		m.CreatedAt = observedAt
		update := e.imported(t, e.admin, pack(t, m, wikiRecord(t, "103", "2", acl, nil, nil, content, _t2)))
		require.Equal(t, entity.ReceiptAccepted, update.Status)
		e.drain(t)
	}
	checkUnread := func() {
		current, err := e.norm.CurrentWikiVersions(t.Context(), e.orgID, area, []string{"103"})
		require.NoError(t, err)
		assert.Empty(t, current)
	}
	// Permissions may change without a new source version.
	importPage(at.Add(time.Minute), "unknown", nil)
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE native_id='103' AND restricted"))
	checkUnread()
	// A delayed package cannot restore the older permissions.
	importPage(at.Add(-time.Minute), "known", &hash)
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE native_id='103' AND restricted"))
	seq := int64(e.n(t, "SELECT max(known_seq) FROM normalize.wiki_page_observation"))
	inventory, err := e.norm.WikiDocumentsAt(t.Context(), e.orgID, area, "103", seq)
	require.NoError(t, err)
	require.Len(t, inventory, 1)
	assert.Equal(t, "104", inventory[0].PageNativeID)
	// Metadata can be visible after a fresh observation while the page body is still unread.
	importPage(at.Add(2*time.Minute), "known", nil)
	checkUnread()
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE doc_format = 'confluence'"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.document_version WHERE page_native_id='103' AND version_ref='2' AND content_hash IS NOT NULL"))
}

type wikiAccess struct {
	scope entity.Scope
	t     *testing.T
}

func (a wikiAccess) Authorize(context.Context, entity.Subject, entity.AccessRequest) (entity.Scope, error) {
	return a.scope, nil
}

func (a wikiAccess) Decide(ctx context.Context, _ entity.Subject, _ entity.Scope, _ entity.SourceQuery) (entity.Scope, error) {
	a.t.Helper()
	_, inTx := postgres.TxFrom(ctx)
	assert.False(a.t, inTx, "source permission checks run outside the inventory transaction")
	return a.scope, nil
}

func (wikiAccess) CurrentUser(context.Context, entity.Subject) (entity.CurrentUser, error) {
	return entity.CurrentUser{}, nil
}

type wikiReferences struct{}

func (wikiReferences) CheckDocumentationReferences(context.Context, entity.ID, entity.ID, []entity.ID) error {
	return nil
}

type wikiAuditor struct{}

func (wikiAuditor) Record(context.Context, entity.AuditEvent) error { return nil }

// wikiManifestAdapter keeps the real connection and scope handling while supplying the source manifest.
type wikiManifestAdapter struct {
	sync.Connect
	items []entity.ManifestItem
	total *int
}

func (a *wikiManifestAdapter) Manifest(_ context.Context, _, _ entity.ID, in entity.ManifestInput) (entity.ManifestPage, error) {
	if in.SchemaID != "documents" || in.Scope.Kind != entity.ScopeWikiSpace {
		panic("unexpected wiki manifest request")
	}
	return entity.ManifestPage{Items: a.items, TotalCount: a.total}, nil
}

func (e *normalizeEnv) checkWikiReconciliation(t *testing.T, source, service, area, root entity.ID) {
	t.Helper()
	ctx := t.Context()
	org := e.orgID
	conn := entity.ID(uuid.New())
	connect := connectrepo.NewRepo(e.app)
	store := syncrepo.NewRepo(e.app)
	tx := persistent.NewTransactor(e.app)
	tenants := platform.NewRepo(e.app)
	require.NoError(t, tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if err := connect.CreateConnection(ctx, entity.Connection{
			ID: conn, OrganizationID: org, InstanceID: source, Name: "Wiki", AdapterKind: entity.AdapterBuiltin,
			ContractVersion: "1", AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionDraft,
			RowVersion: 1, CreatedBy: e.admin.UserID, CreatedAt: e.clock.Now(),
		}); err != nil {
			return err
		}
		_, err := connect.AddScope(ctx, org, conn, source, entity.NewScope{
			Kind: entity.ScopeWikiSpace, NativeScopeID: "ENG", DisplayName: "Engineering",
		}, e.clock.Now())
		return err
	}))
	_, err := e.owner.Exec(ctx, "UPDATE connect.connection SET status = 'active' WHERE id = $1", conn)
	require.NoError(t, err)
	adapter := &wikiManifestAdapter{Connect: e.connect, items: []entity.ManifestItem{
		{RecordKind: "document", NativeID: "100", Version: "1"},
		{RecordKind: "document", NativeID: "101", Version: "1"},
		{RecordKind: "document", NativeID: "102", Version: "1"},
		{RecordKind: "document", NativeID: "103", Version: "2"},
		{RecordKind: "document", NativeID: "103", Version: "2"}, // Repeated by a paginated source.
		{RecordKind: "document", NativeID: "104", Version: "1"},
	}}
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	metrics, err := newSyncMetrics(e.reg)
	require.NoError(t, err)
	collection := sync.New(sync.Deps{
		Repo: store, Tx: tx, Tenants: tenants, Connect: adapter, Normalize: e.norm, Ingest: e.uc,
		Queue: queue.New(jobs), Metrics: metrics, Clock: e.clock,
	})
	reconcile := func() entity.WikiCollection {
		var run entity.ID
		require.NoError(t, tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			id, started, err := store.StartReconcile(ctx, org, conn, "documents", e.clock.Now())
			run = id
			assert.True(t, started)
			return err
		}))
		require.NoError(t, collection.RunReconcile(ctx, entity.SyncReconcileJob{
			OrganizationID: org, ConnectionID: conn, DataKind: "documents", RunID: run, Round: 3,
		}, "wiki-test", func(context.Context) error { return nil }))
		var seq int64
		require.NoError(t, tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			var err error
			seq, _, err = tenants.Clock(ctx, org)
			return err
		}))
		got, err := collection.WikiCollectionAt(ctx, org, area, seq, e.clock.Now())
		require.NoError(t, err)
		require.Equal(t, run, got.RunID)
		return got
	}
	partial := reconcile()
	assert.Equal(t, 5, partial.Expected)
	assert.Equal(t, 4, partial.Read)
	assert.Nil(t, partial.LastSuccess)
	// The source permits the same version on the next fetch.
	hash := strings.Repeat("ab", 32)
	e.imported(t, e.admin, pack(t, wikiManifest(), wikiRecord(t, "102", "1", "known", new("ancestor:100"), []string{"100"}, &hash, _t1)))
	e.drain(t)
	complete := reconcile()
	assert.Equal(t, 5, complete.Expected)
	assert.Equal(t, 5, complete.Read)
	require.NotNil(t, complete.LastSuccess)
	assert.WithinDuration(t, e.clock.Now(), *complete.LastSuccess, 0)
	earlier, err := collection.WikiCollectionAt(ctx, org, area, partial.KnownSeq, e.clock.Now())
	require.NoError(t, err)
	assert.Equal(t, partial, earlier)

	calculator, err := newCalc(e.app, calcModules{facts: e.norm, wiki: collection}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	series, err := calculator.DocumentationSeries(ctx, org, entity.DocumentationScope{ServiceID: service, RootID: root, RepositoryID: area})
	require.NoError(t, err)
	assert.Equal(t, entity.MetricSetDocumentation, series.MetricSet)
	day := e.clock.Now().Truncate(24 * time.Hour)
	job := entity.CalcSliceJob{OrganizationID: org, SeriesID: series.ID, From: day, To: day.AddDate(0, 0, 1), AsOf: e.clock.Now()}
	require.NoError(t, calculator.RunSlice(ctx, job, func(context.Context) error { return nil }))
	var id int64
	var args []byte
	require.NoError(t, e.owner.QueryRow(ctx, "SELECT id, args FROM river_job WHERE kind='calc.slice' AND args->>'series_id'=$1", series.ID.String()).Scan(&id, &args))
	require.NoError(t, json.Unmarshal(args, &job))
	require.NoError(t, calculator.RunSlice(ctx, job, completeJob(id)))
	revision, err := calculator.Revision(ctx, org, series.ID, nil)
	require.NoError(t, err)
	points, err := calculator.RevisionPoints(ctx, org, revision.ID)
	require.NoError(t, err)
	require.Len(t, points, 3)
	for i := range points {
		point := &points[i]
		assert.Equal(t, "D27", point.MetricID)
		assert.Equal(t, entity.StateAssessed, point.State)
		switch point.Estimator {
		case "":
			assert.Equal(t, new(1.0), point.Value)
		case "last_success":
			assert.Equal(t, new(float64(complete.LastSuccess.Unix())), point.Value)
		case "earliest_version":
			require.NotNil(t, complete.EarliestVersion)
			assert.Equal(t, new(float64(complete.EarliestVersion.Unix())), point.Value)
		default:
			t.Errorf("unexpected estimator %q", point.Estimator)
		}
	}

	adapter.total = new(6)
	truncated := reconcile()
	assert.Equal(t, 6, truncated.Expected)
	assert.Equal(t, 5, truncated.Read)
	assert.Equal(t, complete.LastSuccess, truncated.LastSuccess)
	adapter.total = new(0)
	failed := reconcile()
	assert.True(t, failed.Failed)
	assert.Equal(t, complete.LastSuccess, failed.LastSuccess)
}
