package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	calcrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _savedViewsPath = "/api/v1/saved-views"

type savedViewBody struct {
	ID               uuid.UUID      `json:"id"`
	Name             string         `json:"name"`
	SeriesID         uuid.UUID      `json:"series_id"`
	Filters          map[string]any `json:"filters"`
	PinnedRevisionID *uuid.UUID     `json:"pinned_revision_id"`
	CreatedAt        time.Time      `json:"created_at"`
	Report           *reportBody    `json:"report"`
}

type savedViewPageBody struct {
	Data       []savedViewBody `json:"data"`
	NextCursor *string         `json:"next_cursor"`
}

func (e *reportEnv) savedRequest(t *testing.T, b *browser, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rec *httptest.ResponseRecorder
	if body != nil {
		rec = e.json(t, b, method, path, nil, body)
	} else {
		rec = e.do(t, b, method, path, _changing)
	}
	_contractOnce.Do(func() {
		_contractDoc, _contractErr = openapi3.NewLoader().LoadFromFile("../../../docs/technical-specification/openapi.yaml")
	})
	require.NoError(t, _contractErr)
	pattern := "/saved-views"
	if strings.HasPrefix(path, _savedViewsPath+"/") {
		pattern += "/{saved_view_id}"
	}
	item := _contractDoc.Paths.Value(pattern)
	req := httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, Route: &routers.Route{
			Spec: _contractDoc, Path: pattern, PathItem: item, Method: method, Operation: item.GetOperation(method),
		}},
		Status: rec.Code, Header: rec.Header(), Body: io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), input), rec.Body.String())
	return rec
}

func (e *reportEnv) savedInput(t *testing.T, revision uuid.UUID, pin bool) map[string]any {
	t.Helper()
	var series uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT series_id FROM calc.revision WHERE id = $1", revision).Scan(&series))
	in := map[string]any{
		"name": "Отчёт команды", "series_id": series,
		"filters":            map[string]any{"from": e.week, "to": e.next.AddDate(0, 0, 7), "step": "week"},
		"pinned_revision_id": nil,
	}
	if pin {
		in["pinned_revision_id"] = revision
	}
	return in
}

func (e *reportEnv) save(t *testing.T, in map[string]any) savedViewBody {
	t.Helper()
	rec := e.savedRequest(t, e.admin, http.MethodPost, _savedViewsPath, in)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	v := decode[savedViewBody](t, rec)
	assert.Equal(t, _savedViewsPath+"/"+v.ID.String(), rec.Header().Get("Location"))
	assert.Nil(t, v.Report, "create returns a descriptor")
	return v
}

func (e *reportEnv) openSaved(t *testing.T, b *browser, v savedViewBody) savedViewBody {
	t.Helper()
	rec := e.savedRequest(t, b, http.MethodGet, _savedViewsPath+"/"+v.ID.String(), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	out := decode[savedViewBody](t, rec)
	require.NotNil(t, out.Report)
	return out
}

func TestSavedView_RevisionsRetentionAndDeletion(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	first := e.publish(t)
	pinned := e.save(t, e.savedInput(t, first.Revision.ID, true))
	live := e.save(t, e.savedInput(t, first.Revision.ID, false))
	in := e.savedInput(t, first.Revision.ID, true)
	in["filters"] = map[string]any{"period_days": 90}
	relative := e.save(t, in)
	before := e.openSaved(t, e.admin, relative)
	assert.Equal(t, 2, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision_pin WHERE holder_kind = 'saved_view'"))

	e.collectLate(t)
	e.recount(t)
	current := e.readReport(t, e.admin, nil)
	require.NotEqual(t, first.Revision.ID, current.Revision.ID)
	old := e.openSaved(t, e.admin, pinned)
	assert.Equal(t, first.Revision.ID, old.Report.Revision.ID)
	assert.Equal(t, 1.0, *old.Report.value(t, "T05", "", e.week).Value)
	latest := e.openSaved(t, e.admin, live)
	assert.Equal(t, current.Revision.ID, latest.Report.Revision.ID)
	assert.Equal(t, 2.0, *latest.Report.value(t, "T05", "", e.week).Value)
	assert.Equal(t, before.Report, e.openSaved(t, e.admin, relative).Report, "relative pinned dates stay fixed")

	// An explicit installation cutoff makes the superseded revision eligible, but both views hold it.
	cutoff := e.clock.Now().Add(time.Second)
	require.NoError(t, e.calc.PurgeRevisions(t.Context(), e.orgID, cutoff))
	assert.Equal(t, first.Revision.ID, e.openSaved(t, e.admin, pinned).Report.Revision.ID)
	for _, v := range []savedViewBody{pinned, relative} {
		path := _savedViewsPath + "/" + v.ID.String()
		assert.Equal(t, http.StatusNoContent, e.savedRequest(t, e.admin, http.MethodDelete, path, nil).Code)
		assert.Equal(t, http.StatusNoContent, e.savedRequest(t, e.admin, http.MethodDelete, path, nil).Code, "delete is idempotent")
		assert.Equal(t, http.StatusNotFound, e.savedRequest(t, e.admin, http.MethodGet, path, nil).Code)
		if v.ID == pinned.ID {
			require.NoError(t, e.calc.PurgeRevisions(t.Context(), e.orgID, cutoff))
			assert.Equal(t, first.Revision.ID, e.openSaved(t, e.admin, relative).Report.Revision.ID, "the second pin still holds")
		}
	}
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision_pin WHERE holder_kind = 'saved_view'"))
	// Other modules' pins also keep the revision.
	_, err := e.owner.Exec(t.Context(), "INSERT INTO calc.revision_pin (organization_id,revision_id,holder_kind,holder_id) VALUES ($1,$2,'export',$3)", uuid.UUID(e.orgID), first.Revision.ID, uuid.New())
	require.NoError(t, err)
	require.NoError(t, e.calc.PurgeRevisions(t.Context(), e.orgID, cutoff))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision WHERE id = '"+first.Revision.ID.String()+"'"))
	_, err = e.owner.Exec(t.Context(), "DELETE FROM calc.revision_pin WHERE holder_kind = 'export'")
	require.NoError(t, err)
	require.NoError(t, e.calc.PurgeRevisions(t.Context(), e.orgID, cutoff))
	require.NoError(t, e.calc.PurgeRevisions(t.Context(), e.orgID, cutoff), "purge repeats safely")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision WHERE id = '"+first.Revision.ID.String()+"'"))
	assert.Equal(t, current.Revision.ID, e.openSaved(t, e.admin, live).Report.Revision.ID, "current revision stays")
	assert.Positive(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM audit.deletion WHERE basis = 'retention' AND retention_class = 'SUPERSEDED_REVISIONS' AND status = 'applied'"))
}

func TestSavedView_ReaderRightsAndTenantIsolation(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	r := e.publish(t)
	v := e.save(t, e.savedInput(t, r.Revision.ID, true))
	roles := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, roles.Code)
	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, roles.Header().Get("ETag"), []roleInput{
		{Role: "developer", ProjectID: &e.project}, {Role: "dev_lead", TeamID: &e.team},
	}).Code)
	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	e.link(t, _otherSub, "demo-person-02")
	assert.False(t, e.openSaved(t, e.admin, v).Report.AccessLimited)
	assert.False(t, e.openSaved(t, e.member, v).Report.AccessLimited)

	closed := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO normalize.source_object
  (organization_id,id,instance_id,object_kind,native_ns,native_id,identity_quality,first_known_seq,restricted)
  VALUES ($1,$2,$3,'tracker_project','','10099','stable',1,false)`, uuid.UUID(e.orgID), closed, e.instance)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO normalize.area (organization_id,area_id,area_kind,last_change_seq) VALUES ($1,$2,'tracker_project',1)", uuid.UUID(e.orgID), closed)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET current_area_id = $1 WHERE native_id = '31001' AND object_kind = 'task'", closed)
	require.NoError(t, err)
	e.clock.Advance(61 * time.Second)
	member := e.openSaved(t, e.member, v)
	assert.True(t, member.Report.AccessLimited)
	assert.Equal(t, 0.0, *member.Report.value(t, "T05", "", e.week).Value, "hidden A contributes nothing")
	assert.Zero(t, *member.Report.value(t, "T08", "p50", e.week).N)
	assert.Equal(t, 1.0, *e.openSaved(t, e.admin, v).Report.value(t, "T05", "", e.week).Value)
	cohort := e.cohort(t, e.member, member.Report.Revision.ID, url.Values{"metric_id": {"T05"}, "point_start": {e.week.Format(time.RFC3339)}})
	assert.Equal(t, []string{"R-B:excluded"}, cohort.keys())
	assert.True(t, cohort.AccessLimited)
	path := _savedViewsPath + "/" + v.ID.String()
	assert.Equal(t, http.StatusNotFound, e.savedRequest(t, e.member, http.MethodDelete, path, nil).Code)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision_pin WHERE holder_kind = 'saved_view'"), "foreign delete keeps the pin")

	// A filter cannot turn the opener into the author or grant access to the hidden project.
	in := e.savedInput(t, r.Revision.ID, false)
	in["filters"].(map[string]any)["project_id"] = []uuid.UUID{closed}
	in["filters"].(map[string]any)["scope"] = "organization"
	in["filters"].(map[string]any)["user_id"] = e.adminID
	filtered := e.save(t, in)
	assert.Equal(t, 0.0, *e.openSaved(t, e.member, filtered).Report.value(t, "T05", "", e.week).Value)
	assert.Empty(t, decode[savedViewPageBody](t, e.savedRequest(t, e.member, http.MethodGet, _savedViewsPath, nil)).Data)
	other := e.loginEnv.admin(t, "other")
	assert.Equal(t, http.StatusNotFound, e.savedRequest(t, other, http.MethodGet, path, nil).Code)
	assert.Empty(t, decode[savedViewPageBody](t, e.savedRequest(t, other, http.MethodGet, _savedViewsPath, nil)).Data)
	foreign := uuid.New()
	err = postgres.InOrgTx(t.Context(), e.app, foreign, func(ctx context.Context) error {
		_, err := calcrepo.NewRepo(e.app).SavedView(ctx, e.orgID, entity.ID(v.ID))
		return err
	})
	assert.ErrorIs(t, err, entity.ErrNotFound, "RLS hides even a query using the real organization ID")
}

func TestSavedView_PaginationFiltersAndValidation(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	r := e.publish(t)
	ids := make([]uuid.UUID, 0, 3)
	for range 3 {
		v := e.save(t, e.savedInput(t, r.Revision.ID, false))
		ids = append(ids, v.ID)
	}
	seen := make(map[uuid.UUID]bool)
	path := _savedViewsPath + "?limit=1"
	for {
		rec := e.savedRequest(t, e.admin, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		page := decode[savedViewPageBody](t, rec)
		require.Len(t, page.Data, 1)
		assert.Nil(t, page.Data[0].Report)
		assert.False(t, seen[page.Data[0].ID], "equal timestamps still paginate without duplicates")
		seen[page.Data[0].ID] = true
		if page.NextCursor == nil {
			break
		}
		path = _savedViewsPath + "?limit=1&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	assert.Len(t, seen, len(ids))
	for _, query := range []string{"?limit=0", "?limit=201", "?cursor=broken"} {
		assert.Equal(t, http.StatusBadRequest, e.savedRequest(t, e.admin, http.MethodGet, _savedViewsPath+query, nil).Code)
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{name: "blank name", edit: func(in map[string]any) { in["name"] = "  " }},
		{name: "long name", edit: func(in map[string]any) { in["name"] = strings.Repeat("я", 201) }},
		{name: "step of another series", edit: func(in map[string]any) { in["filters"] = map[string]any{"step": "month"} }},
		{name: "reversed dates", edit: func(in map[string]any) { in["filters"] = map[string]any{"from": e.next, "to": e.week} }},
		{name: "one date", edit: func(in map[string]any) { in["filters"] = map[string]any{"from": e.week} }},
		{name: "zero days", edit: func(in map[string]any) { in["filters"] = map[string]any{"period_days": 0} }},
		{name: "ambiguous period", edit: func(in map[string]any) { in["filters"].(map[string]any)["period_days"] = 90 }},
		{name: "bad group", edit: func(in map[string]any) { in["filters"] = map[string]any{"group_by": "scope"} }},
		{name: "missing filters", edit: func(in map[string]any) { delete(in, "filters") }},
		{name: "null filters", edit: func(in map[string]any) { in["filters"] = nil }},
		{name: "null step", edit: func(in map[string]any) { in["filters"] = map[string]any{"step": nil} }},
		{name: "invalid project", edit: func(in map[string]any) { in["filters"] = map[string]any{"project_id": []string{"broken"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := e.savedInput(t, r.Revision.ID, true)
			tc.edit(in)
			assert.Equal(t, http.StatusUnprocessableEntity, e.savedRequest(t, e.admin, http.MethodPost, _savedViewsPath, in).Code)
		})
	}
	in := e.savedInput(t, r.Revision.ID, true)
	in["pinned_revision_id"] = uuid.New()
	assert.Equal(t, http.StatusNotFound, e.savedRequest(t, e.admin, http.MethodPost, _savedViewsPath, in).Code)
	in["pinned_revision_id"], in["series_id"] = nil, uuid.New()
	assert.Equal(t, http.StatusNotFound, e.savedRequest(t, e.admin, http.MethodPost, _savedViewsPath, in).Code)
	assert.Equal(t, 3, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.saved_view"), "failed creates leave no descriptor")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision_pin WHERE holder_kind = 'saved_view'"))
	month, err := e.calc.TeamSeries(t.Context(), e.orgID, entity.ID(e.team), entity.StepMonth)
	require.NoError(t, err)
	in = e.savedInput(t, r.Revision.ID, true)
	in["series_id"], in["filters"] = month.ID, map[string]any{"step": "month"}
	assert.Equal(t, http.StatusNotFound, e.savedRequest(t, e.admin, http.MethodPost, _savedViewsPath, in).Code, "pin must belong to the saved series")

	in = e.savedInput(t, r.Revision.ID, true)
	in["filters"].(map[string]any)["size_label"] = "unknown"
	unsized := e.openSaved(t, e.admin, e.save(t, in))
	assert.Zero(t, *unsized.Report.value(t, "T05", "", e.week).Value)
	assert.Equal(t, 1, unsized.Report.value(t, "T05", "", e.week).ExcludedCount)
	in = e.savedInput(t, r.Revision.ID, true)
	in["filters"].(map[string]any)["task_type"] = "Bug"
	bugs := e.openSaved(t, e.admin, e.save(t, in))
	assert.Zero(t, *bugs.Report.value(t, "T08", "p50", e.week).N)
	// A direct link to a revision no longer reproducible keeps its identity and explains why.
	_, err = e.owner.Exec(t.Context(), "UPDATE calc.revision SET irreproducible_reason = 'basis_deleted' WHERE id = $1", r.Revision.ID)
	require.NoError(t, err)
	rec := e.savedRequest(t, e.admin, http.MethodGet, _savedViewsPath+"/"+bugs.ID.String(), nil)
	assert.Contains(t, rec.Body.String(), `"irreproducible_reason":"basis_deleted"`)
	assert.Contains(t, rec.Body.String(), r.Revision.ID.String())
}

// Ensure the same source refusal used for ordinary reports narrows a saved report after the access TTL.
func TestSavedView_SourceAccessRevocation(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	r := e.publish(t)
	v := e.save(t, e.savedInput(t, r.Revision.ID, true))
	assert.Equal(t, 1.0, *e.openSaved(t, e.admin, v).Report.value(t, "T05", "", e.week).Value)
	e.rights(t, testadapter.Access{Default: "denied"})
	e.clock.Advance(61 * time.Second)
	opened := e.openSaved(t, e.admin, v)
	assert.True(t, opened.Report.AccessLimited)
	assert.Zero(t, *opened.Report.value(t, "T05", "", e.week).Value)
	assert.Zero(t, opened.Report.value(t, "T05", "", e.week).ExcludedCount)
	assert.Zero(t, *opened.Report.value(t, "T08", "p50", e.week).N)
}
