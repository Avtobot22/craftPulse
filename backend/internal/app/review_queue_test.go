package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _reviewQueuePath = "/api/v1/merge-requests"

func reviewQueue(t *testing.T, e *loginEnv, b *browser, values url.Values) openapi.MergeRequestPage {
	t.Helper()
	return decode[openapi.MergeRequestPage](t, codeRead(t, e, b, _reviewQueuePath+"?"+values.Encode(), "MergeRequestPage"))
}

func queueMR(t *testing.T, id, area string, opened time.Time, reviewerField *bool, reviewers []string, draft bool) *envelope.Record {
	t.Helper()
	base := mrState(t, id, area, "Queue MR "+id, "v1", _t1, reviewerField)
	var payload envelope.MergeRequestV1
	require.NoError(t, json.Unmarshal(base.Payload, &payload))
	payload.OpenedAt, payload.Draft = opened, draft
	payload.ReviewerAccountNativeIds = nil
	if reviewers != nil {
		payload.ReviewerAccountNativeIds = &reviewers
	}
	base.Payload = mustMarshal(t, &payload)
	return base
}

func TestReviewQueue_V15AndFallback(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	assigned := queueMR(t, "9001", "42", _created, ptr(true), []string{"3"}, false)
	unknown := queueMR(t, "9002", "42", _created, nil, nil, false)
	unassigned := queueMR(t, "9003", "42", _created, ptr(true), []string{}, false)
	draft := queueMR(t, "9004", "42", _created, ptr(true), []string{}, true)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "team/queue", "r1"), assigned, unknown, unassigned, draft,
		reviewEvent(t, "ready", "ready", "1", _t1), accountRecord(t, "2", "Bot"), accountRecord(t, "3", "Reviewer")))
	require.Equal(t, entity.ReceiptAccepted, rc.Status, e.records(t, rc.ID))
	e.drain(t)
	area := mrArea(t, e, rc, "42")
	bindLinkArea(t, e, area)
	r := &reviewEnv{normalizeEnv: e}
	r.confirm(t, "1", entity.PersonHuman)
	r.confirm(t, "2", entity.PersonBot)
	r.confirm(t, "3", entity.PersonHuman)
	ids := map[string]uuid.UUID{}
	for _, native := range []string{"9001", "9002", "9003", "9004"} {
		ids[native] = uuid.UUID(mrID(t, e, native))
	}
	query := url.Values{"project_id": {e.project.String()}, "review_wait": {string(entity.ReviewAwaitingFirst)}}
	assertWaiting := func() {
		page := reviewQueue(t, e.loginEnv, b, query)
		require.Len(t, page.Data, 2)
		assert.Equal(t, ids["9002"], page.Data[0].Id)
		require.NotNil(t, page.Data[0].ReviewWait)
		assert.True(t, page.Data[0].ReviewWait.FallbackOpen)
		assert.True(t, page.Data[0].ReviewWait.Since.Equal(_created))
		assert.Equal(t, int(e.clock.Now().Sub(_created)/time.Second), page.Data[0].ReviewWait.WaitSeconds)
		assert.Equal(t, ids["9001"], page.Data[1].Id)
		detail := decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, b, _reviewQueuePath+"/"+ids["9001"].String(), "MergeRequest"))
		assert.Equal(t, page.Data[1].ReviewWait, detail.ReviewWait)
	}
	assertWaiting()
	e.imported(t, e.admin, pack(t, mrManifest("42"), reviewEvent(t, "bot", "review", "2", _t1.Add(time.Hour)),
		reviewEvent(t, "author", "comment", "1", _t1.Add(2*time.Hour))))
	e.drain(t)
	assertWaiting()
	e.imported(t, e.admin, pack(t, mrManifest("42"), reviewEvent(t, "human", "review", "3", _t1.Add(3*time.Hour))))
	e.drain(t)
	page := reviewQueue(t, e.loginEnv, b, query)
	require.Len(t, page.Data, 1)
	assert.Equal(t, ids["9002"], page.Data[0].Id)
	query.Set("review_wait", string(entity.ReviewNoReviewer))
	page = reviewQueue(t, e.loginEnv, b, query)
	require.Len(t, page.Data, 1)
	assert.Equal(t, ids["9003"], page.Data[0].Id)
	assert.Equal(t, http.StatusBadRequest, e.do(t, b, http.MethodGet, _reviewQueuePath+"?review_wait=awaiting_re_review", nil).Code)
}

func TestReviewQueue_ClosedAndRestrictedReviews(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	assigned := queueMR(t, "9001", "42", _created, ptr(true), []string{"3"}, false)
	restricted := queueMR(t, "9005", "42", _created, ptr(true), []string{"3"}, false)
	archived := queueMR(t, "9006", "42", _created, ptr(true), []string{}, false)
	var payload envelope.MergeRequestV1
	require.NoError(t, json.Unmarshal(archived.Payload, &payload))
	payload.State, payload.ClosedAt = "closed", &_t1
	archived.Payload = mustMarshal(t, &payload)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), assigned, restricted, archived, accountRecord(t, "3", "Reviewer"),
		reviewEvent(t, "comment", "comment", "3", _t1)))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	(&reviewEnv{normalizeEnv: e}).confirm(t, "3", entity.PersonHuman)
	event := githubEvent(t, "9005", prEvent{id: "review", kind: "review", actor: "3"})
	e.imported(t, e.admin, pack(t, mrManifest("42"), event))
	e.drain(t)
	area := mrArea(t, e, rc, "42")
	_, err := e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET restricted=true WHERE organization_id=$1 AND id=$2", uuid.UUID(e.orgID), uuid.UUID(mrID(t, e, "9005")))
	require.NoError(t, err)
	q := url.Values{"area_id": {area.String()}, "review_wait": {"awaiting_first_response"}}
	page := reviewQueue(t, e.loginEnv, b, q)
	require.Len(t, page.Data, 1)
	id := mrID(t, e, "9001")
	assert.Equal(t, uuid.UUID(id), page.Data[0].Id)
	detail := decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, b, _reviewQueuePath+"/"+id.String(), "MergeRequest"))
	assert.Equal(t, page.Data[0].ReviewWait, detail.ReviewWait)
	q.Set("state", "closed")
	q.Del("review_wait")
	require.Len(t, reviewQueue(t, e.loginEnv, b, q).Data, 1)
	q.Set("review_wait", "no_reviewer")
	assert.Empty(t, reviewQueue(t, e.loginEnv, b, q).Data)
}

func TestReviewQueue_CursorPinsTimeAndKnowledge(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	records := make([]*envelope.Record, 0, 4)
	for i, id := range []string{"9201", "9202", "9203", "9204"} {
		records = append(records, queueMR(t, id, "42", _created.Add(time.Duration(i/2)*time.Hour), ptr(true), []string{}, false))
	}
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), records...))
	e.drain(t)
	area := mrArea(t, e, rc, "42")
	filter := url.Values{"area_id": {area.String()}, "review_wait": {"no_reviewer"}, "sort": {"-wait"}}
	expected := reviewQueue(t, e.loginEnv, b, filter)
	require.Len(t, expected.Data, 4)
	filter.Set("limit", "1")
	page := reviewQueue(t, e.loginEnv, b, filter)
	require.NotNil(t, page.NextCursor)
	assert.Equal(t, http.StatusBadRequest, e.do(t, b, http.MethodGet, _reviewQueuePath+"?cursor="+*page.NextCursor, nil).Code)
	got := page.Data
	e.clock.Advance(10 * time.Minute)
	// A late import cannot enter an already-open list even if it has the oldest opening time.
	e.imported(t, e.admin, pack(t, mrManifest("42"), queueMR(t, "9299", "42", _created.Add(-time.Hour), ptr(true), []string{}, false)))
	e.drain(t)
	for range 3 {
		require.NotNil(t, page.NextCursor)
		filter.Set("cursor", *page.NextCursor)
		page = reviewQueue(t, e.loginEnv, b, filter)
		got = append(got, page.Data...)
	}
	assert.Nil(t, page.NextCursor)
	assert.Equal(t, expected.Data, got)
}

func TestReviewQueue_DeniedRepository(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	mr := e.object(t, "merge_request", "queue-visible", false)
	// Use the test adapter's collected area as a repository for this query.
	_, err := e.owner.Exec(t.Context(), "UPDATE normalize.area SET area_kind='repository' WHERE area_id=$1", e.area)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.mr_version
 (organization_id,known_seq,receipt_id,record_no,source_revision,content_hash,observed_at,mr_id,area_id,iid,title,state,draft,source_branch,target_branch,head_sha,reviewer_field,reviewer_account_ids,opened_at)
 VALUES ($1,1,$2,1,'v1',decode(repeat('00',32),'hex'),$3,$4,$5,7,'Visible MR','open',false,'feature','main',$6,false,'{}',$3)`, uuid.UUID(e.orgID), uuid.New(), e.clock.Now(), mr, e.area, _headA)
	require.NoError(t, err)
	filter := url.Values{"project_id": {e.project.String()}}
	require.Len(t, reviewQueue(t, e.loginEnv, e.member, filter).Data, 1)
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{{UserID: e.adminID.String(), Decision: "allowed"}}})
	e.clock.Advance(61 * time.Second)
	page := reviewQueue(t, e.loginEnv, e.member, filter)
	assert.Empty(t, page.Data)
	assert.True(t, page.AccessLimited)
	assert.NotEmpty(t, page.AccessNotes)
	assert.Nil(t, page.NextCursor)
	require.Len(t, reviewQueue(t, e.loginEnv, e.admin, filter).Data, 1)
}
