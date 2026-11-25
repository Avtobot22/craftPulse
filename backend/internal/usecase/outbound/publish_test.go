package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type publicationFixture struct {
	summary                  entity.SummaryPublication
	result                   entity.Publication
	actions                  []entity.Publication
	audit                    []entity.AuditEvent
	calls                    []string
	note                     string
	last                     entity.PublishInput
	creates, completed       int
	loseReply, denied, stale bool
}

func (*publicationFixture) InOrgTx(ctx context.Context, _ entity.ID, fn func(context.Context) error) error {
	return fn(ctx)
}

func (f *publicationFixture) PublicationSummary(context.Context, entity.ID, entity.ID) (entity.SummaryPublication, error) {
	return f.summary, nil
}

func (f *publicationFixture) CheckPublicationHead(context.Context, entity.ID, *entity.SummaryPublication) (bool, error) {
	f.calls = append(f.calls, "head")
	return !f.stale, nil
}

func (f *publicationFixture) FinishPublication(ctx context.Context, _, _, _ entity.ID, p entity.Publication, complete func(context.Context) error) error {
	if p.Status != "" {
		f.result, f.summary.Revision.Publication = p, p
	}
	if complete != nil {
		return complete(ctx)
	}
	return nil
}

func (f *publicationFixture) FindPublished(_ context.Context, _, _ entity.ID, in entity.PublicationTarget) (string, error) {
	f.calls = append(f.calls, "find")
	if in.Marker != f.summary.Revision.MRID.String() || in.Kind != entity.MRComment {
		return "", errors.New("unexpected target")
	}
	return f.note, nil
}

func (f *publicationFixture) Publish(_ context.Context, _, _ entity.ID, in entity.PublishInput) (entity.Publication, error) {
	f.calls = append(f.calls, "publish")
	f.last = in
	if f.denied {
		return entity.Publication{Status: entity.PublicationFailed, ErrorCode: entity.WritePermissionMissing}, nil
	}
	if in.ExternalID == "" {
		f.creates++
		f.note = strconv.Itoa(f.creates)
	}
	if f.loseReply {
		f.loseReply = false
		return entity.Publication{}, errors.New("response lost")
	}
	return entity.Publication{Status: entity.PublicationSucceeded, ExternalID: f.note}, nil
}

func (f *publicationFixture) Start(context.Context, entity.ID, entity.ID, entity.ID, entity.PublicationTarget, time.Time) (entity.ID, error) {
	f.actions = append(f.actions, entity.Publication{Status: "intent"})
	return entity.ID{1}, nil
}

func (f *publicationFixture) Finish(_ context.Context, _, _, _, _ entity.ID, _ entity.PublicationTarget, p entity.Publication, _ time.Time) error {
	f.actions[len(f.actions)-1] = p
	return nil
}

func (f *publicationFixture) Record(_ context.Context, e entity.AuditEvent) error {
	f.audit = append(f.audit, e)
	return nil
}

func newPublicationFixture() *publicationFixture {
	return &publicationFixture{
		summary: entity.SummaryPublication{
			MR: entity.SummaryMR{MRVersion: entity.MRVersion{HeadSHA: "current-head"}},
			Revision: entity.SummaryRevision{
				MRID:          entity.ID{2},
				ID:            entity.ID{3},
				No:            1,
				HeadSHA:       "current-head",
				Status:        "ready",
				PipelineState: "complete",
			},
			ConnectionID: entity.ID{4},
		},
	}
}

func (f *publicationFixture) run(t *testing.T, finalAttempt bool) error {
	u := New(Deps{Changes: f, Adapter: f, Store: f, Audit: f, Tx: f, Clock: clock.System{}, SiteURL: "https://craftpulse.example"})
	job := entity.OutboundPublishJob{OrganizationID: entity.ID{5}, MRID: f.summary.Revision.MRID, SummaryRevisionID: entity.ID{3}}
	return u.RunPublish(t.Context(), job, func(context.Context) error { f.completed++; return nil }, finalAttempt)
}

func TestPublicationRepeatedDelivery(t *testing.T) {
	t.Parallel()
	f := newPublicationFixture()
	for i := range 3 {
		f.summary.Revision.No = i + 1
		f.summary.Revision.ID = entity.ID{byte(3 + i)}
		f.summary.Revision.Publication = entity.Publication{}
		if err := f.run(t, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.run(t, false); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.result.ExternalID != "1" || len(f.actions) != 3 || f.completed != 4 ||
		!strings.Contains(f.last.Markdown, "revision_no=3") || f.last.HeadSHA != "current-head" || f.result.PublishedAt == nil {
		t.Fatalf("unexpected publication: creates=%d, actions=%v, result=%+v, body=%s", f.creates, f.actions, f.result, f.last.Markdown)
	}
	if !slices.Equal(f.calls, []string{"find", "head", "publish", "find", "head", "publish", "find", "head", "publish"}) {
		t.Fatalf("unexpected adapter calls: %v", f.calls)
	}
}

func TestPublicationRecoversLostReply(t *testing.T) {
	t.Parallel()
	f := newPublicationFixture()
	f.loseReply = true
	if err := f.run(t, false); !errors.Is(err, errUnknown) || f.result.Status != entity.PublicationUnknown || f.completed != 0 {
		t.Fatalf("lost reply: err=%v, result=%+v, completed=%d", err, f.result, f.completed)
	}
	if err := f.run(t, false); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.last.ExternalID != "1" || f.result.Status != entity.PublicationSucceeded ||
		f.actions[0].Status != entity.PublicationUnknown || f.audit[1].Details["status"] != entity.PublicationUnknown {
		t.Fatalf("reconciliation: creates=%d, result=%+v, actions=%+v", f.creates, f.result, f.actions)
	}
}

func TestPublicationCompletesFinalUnknownAttempt(t *testing.T) {
	t.Parallel()
	f := newPublicationFixture()
	f.loseReply = true
	if err := f.run(t, true); err != nil || f.completed != 1 || f.result.Status != entity.PublicationUnknown {
		t.Fatalf("final attempt: err=%v, completed=%d, result=%+v", err, f.completed, f.result)
	}
}

func TestPublicationSuppressesStaleHeadAndRecordsDenial(t *testing.T) {
	t.Parallel()
	t.Run("head changed after lookup", func(t *testing.T) {
		f := newPublicationFixture()
		f.stale = true
		if err := f.run(t, false); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.calls, []string{"find", "head"}) || f.result.Status != entity.PublicationStale ||
			f.actions[0].Status != entity.PublicationStale || f.completed != 1 {
			t.Fatalf("stale head: calls=%v, result=%+v", f.calls, f.result)
		}
	})
	t.Run("write denied", func(t *testing.T) {
		f := newPublicationFixture()
		f.denied = true
		if err := f.run(t, false); err != nil {
			t.Fatal(err)
		}
		if f.result.Status != entity.PublicationFailed || f.result.ErrorCode != entity.WritePermissionMissing ||
			f.summary.Revision.Status != "ready" || f.creates != 0 || f.audit[1].Outcome != entity.AuditDenied {
			t.Fatalf("denied: result=%+v, revision=%+v", f.result, f.summary.Revision)
		}
	})
}

func TestPublicationRendersPublicSummary(t *testing.T) {
	t.Parallel()
	f := newPublicationFixture()
	v := &f.summary.Revision
	v.Items = []entity.SummaryItem{
		{Kind: "new", Rank: new(1), RequiredRight: "security", Value: json.RawMessage(`{"Rule":"secret-rule","Path":"secret-path","Reason":"secret-evidence"}`)},
		{Kind: "resolved", RequiredRight: "security"},
		{Kind: "resolved", RequiredRight: "none"},
		{Kind: "not_evaluated", RequiredRight: "none", AnalysisKind: "docs", ReasonCode: "report_missing"},
		{Kind: "tests", RequiredRight: "none", Value: json.RawMessage(`{"State":"ok","Counts":{"Passed":5,"Failed":1}}`)},
		{Kind: "ci_attempts", RequiredRight: "none", Value: json.RawMessage(`{"metrics":[{"MetricID":"I03","State":"ok","Numerator":1,"Denominator":2}]}`)},
		{Kind: "coverage", RequiredRight: "none", Value: json.RawMessage(`{"Patch":{"State":"not_evaluated","ReasonCode":"coverage_line_map_missing","Value":0.5},"Size":{"State":"ok","Total":{"Files":2,"Added":3,"Deleted":4}},"Code":[{"Language":"go","Complexity":{"State":"ok","Deltas":[{"Delta":2}]},"Clones":{"State":"ok","Value":1}}]}`)},
	}
	for i := 7; i > 1; i-- {
		v.Items = append(v.Items, entity.SummaryItem{
			Kind: "new", Rank: new(i), FindingID: &entity.ID{byte(i)}, RequiredRight: "none",
			Value: json.RawMessage(fmt.Sprintf(`{"rule":"rule-%d","path":"main.go","line":12}`, i)),
		})
	}
	if err := f.run(t, false); err != nil {
		t.Fatal(err)
	}
	text := f.last.Markdown
	for _, want := range []string{"Есть закрытые результаты", "Устранено сигналов: 1", "head_sha", "current-head", "revision_no=1",
		"rule-2", "main.go:12", "rule-6", "report_missing", "прошло 5", "fail → pass", "C15", "C16", "C17", "coverage_line_map_missing", "50.00%"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "secret-") || strings.Contains(text, "rule-7") || strings.Contains(text, "flaky") ||
		strings.Count(text, "#finding-") != 5 || strings.Index(text, "rule-2") > strings.Index(text, "rule-6") {
		t.Fatalf("unexpected Markdown: %s", text)
	}
}
