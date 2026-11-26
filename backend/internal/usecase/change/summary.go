package change

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// SummaryStore persists the immutable inputs and history of each MR summary.
type SummaryStore interface {
	LastGoodSummary(context.Context, entity.ID, entity.ID) (entity.SummaryRevision, error)
	SummaryProfileAt(context.Context, entity.ID, entity.ID, time.Time, int64) (entity.CheckProfileVersion, error)
	StaleSummaries(context.Context, entity.ID, entity.ID, string) error
	SaveSummary(context.Context, entity.ID, *entity.SummaryRevision) (bool, error)
}

// SummaryFacts reads normalize facts and locks their knowledge counter at finalization.
type SummaryFacts interface {
	SummaryMR(context.Context, entity.ID, entity.ID, bool) (entity.SummaryMR, error)
	SummarySeverities(context.Context, entity.ID, []entity.ID) (map[entity.ID]string, error)
	SummaryReportReadyAt(context.Context, entity.ID, entity.ID) (time.Time, error)
	SummaryReports(context.Context, entity.ID, entity.SummaryMR, entity.ProfileEntry, []entity.ID) ([]entity.TechnicalReport, error)
}

// SummarySource reads the live head through connect.
type SummarySource interface {
	AreaSources(context.Context, entity.ID, []entity.ID) ([]entity.AreaSource, error)
	Fetch(context.Context, entity.ID, entity.ID, entity.FetchInput) (io.ReadCloser, error)
}

// SummaryCalculations applies the reusable calc definitions to selected normalize facts.
type SummaryCalculations interface {
	SummaryPipeline(context.Context, entity.ID, entity.SummaryMR) (entity.CIRun, error)
	SummarizeMetrics(context.Context, entity.ID, entity.SummaryMR, entity.CIRun, SummaryCheckJobs, *entity.SummaryRevision) error
	AnalysisFacts(context.Context, entity.ID, entity.AnalysisScope, int64, time.Time) (entity.AnalysisFacts, error)
	SummarizeFindings(entity.SummaryFindingComparison, []entity.ExceptionVersion) ([]entity.SummaryFindingValues, []entity.RevisionInput)
}

// SummaryDeps supplies the worker-only dependencies of change.summary.
type SummaryDeps struct {
	Store          SummaryStore
	Publication    PublicationStore
	Facts          SummaryFacts
	Source         SummarySource
	Calculations   SummaryCalculations
	Queue          repo.JobQueue
	ObserveLatency func(float64)
}

// RunSummary checks the live head before building and again before queuing publication. The final check of
// normalize's watermark and River completion share a transaction with the revision and publication intent.
func (u *UseCase) RunSummary(ctx context.Context, job entity.ChangeSummaryJob, complete Complete) error {
	org, mr := job.OrganizationID, job.MRID
	var state entity.SummaryMR
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		state, err = u.d.Summary.Facts.SummaryMR(ctx, org, mr, false)
		if errors.Is(err, entity.ErrNotFound) {
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	if state.MRID.IsZero() {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	head, err := u.fetchSummaryHead(ctx, org, state)
	if errors.Is(err, entity.ErrNotFound) {
		return u.finishSummary(ctx, job, state, state.HeadSHA, nil, time.Time{}, complete)
	}
	if err != nil {
		return err
	}
	if head != state.HeadSHA {
		return u.finishSummary(ctx, job, state, head, nil, time.Time{}, complete)
	}
	var v *entity.SummaryRevision
	var last time.Time
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		current, err := u.d.Summary.Facts.SummaryMR(ctx, org, mr, true)
		if errors.Is(err, entity.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.FactsSeq != state.FactsSeq {
			return nil
		}
		v, last, err = u.buildSummary(ctx, org, current)
		return err
	})
	if err != nil {
		return err
	}
	if v != nil {
		head, err = u.fetchSummaryHead(ctx, org, state)
		if err != nil {
			return err
		}
	}
	return u.finishSummary(ctx, job, state, head, v, last, complete)
}

func (u *UseCase) finishSummary(ctx context.Context, job entity.ChangeSummaryJob, state entity.SummaryMR,
	head string, v *entity.SummaryRevision, last time.Time, complete Complete,
) error {
	d := u.d.Summary
	observe := false
	err := u.d.Tx.InOrgTx(ctx, job.OrganizationID, func(ctx context.Context) error {
		observe = false
		org, mr := job.OrganizationID, job.MRID
		current, err := d.Facts.SummaryMR(ctx, org, mr, true)
		if errors.Is(err, entity.ErrNotFound) {
			return complete(ctx)
		}
		if err != nil {
			return err
		}
		// A head change seen by either read invalidates the snapshot. Prefer an explicit
		// live change when normalize has advanced to an intermediate head.
		if head == state.HeadSHA && current.HeadSHA != state.HeadSHA {
			head = current.HeadSHA
		}
		if err := d.Store.StaleSummaries(ctx, org, mr, head); err != nil {
			return err
		}
		if err := complete(ctx); err != nil {
			return err
		}
		if current.HeadSHA != state.HeadSHA || current.FactsSeq != state.FactsSeq {
			return d.Queue.Enqueue(ctx, job, entity.QueueInteractive)
		}
		if v == nil || head != v.HeadSHA {
			return nil
		}
		v.BuiltAt, v.ReportsReadyAt = u.d.Clock.Now(), last
		saved, err := d.Store.SaveSummary(ctx, org, v)
		if err != nil || !saved {
			return err
		}
		if err := d.Queue.Enqueue(ctx, entity.CalcRecountJob{OrganizationID: org, AreaID: current.AreaID}, entity.QueueBulk); err != nil {
			return err
		}
		if v.PipelineState != "running" {
			if err := d.Queue.Enqueue(ctx, entity.OutboundPublishJob{OrganizationID: org, MRID: mr, SummaryRevisionID: v.ID}, entity.QueueInteractive); err != nil {
				return err
			}
		}
		observe = v.PipelineState == "complete" && !last.IsZero()
		return nil
	})
	if err == nil && observe && d.ObserveLatency != nil {
		d.ObserveLatency(max(0, v.BuiltAt.Sub(last).Seconds()))
	}
	return err
}

func (u *UseCase) fetchSummaryHead(ctx context.Context, org entity.ID, mr entity.SummaryMR) (string, error) {
	source, err := u.summaryAreaSource(ctx, org, mr)
	if err != nil {
		return "", err
	}
	return u.readPublicationHead(ctx, org, source, mr.Source)
}

func (u *UseCase) summaryAreaSource(ctx context.Context, org entity.ID, mr entity.SummaryMR) (entity.AreaSource, error) {
	sources, err := u.d.Summary.Source.AreaSources(ctx, org, []entity.ID{mr.AreaID})
	if err != nil {
		return entity.AreaSource{}, err
	}
	for i := range sources {
		s := &sources[i]
		if s.Active && s.ConnectionStatus == entity.ConnectionActive && s.InstanceID == mr.InstanceID && s.Kind == entity.ScopeRepository {
			return *s, nil
		}
	}
	return entity.AreaSource{}, entity.ErrNotFound
}

func (u *UseCase) readPublicationHead(ctx context.Context, org entity.ID, source entity.AreaSource, object entity.ObjectKey) (string, error) {
	body, err := u.d.Summary.Source.Fetch(ctx, org, source.ConnectionID, entity.FetchInput{
		SchemaID: "merge_requests", Limit: 1, ObjectKeys: []entity.ObjectKey{object},
		Scope: entity.ScopeRef{Kind: source.Kind, NativeScopeID: source.NativeScopeID},
	})
	if err != nil {
		return "", err
	}
	return summaryHead(body, object)
}

func summaryHead(body io.ReadCloser, key entity.ObjectKey) (string, error) {
	defer body.Close()
	var archive bytes.Buffer
	result, err := envelope.CleanPageWithRedactor(body, envelope.DefaultLimits, &archive, nil)
	if err != nil {
		return "", err
	}
	if result.Envelope != nil {
		return "", errorf("MR fetch rejected: %s", result.Envelope.Code)
	}
	pkg, err := envelope.ReadCleaned(&archive, envelope.DefaultLimits)
	if err != nil {
		return "", err
	}
	for _, r := range result.Records {
		if r.Rejection != nil || r.Kind != key.ObjectKind || r.NativeID != key.NativeID || r.NativeNS != key.NativeNS {
			continue
		}
		rec, ok, err := pkg.Record(r.No)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		var mr envelope.MergeRequestV1
		if err := json.Unmarshal(rec.Payload, &mr); err != nil {
			return "", err
		}
		if entity.ValidGitSHA(mr.HeadSha) {
			return mr.HeadSha, nil
		}
	}
	return "", errorf("current MR head absent from source page")
}

func (u *UseCase) buildSummary(ctx context.Context, org entity.ID, mr entity.SummaryMR) (*entity.SummaryRevision, time.Time, error) {
	run, err := u.d.Summary.Calculations.SummaryPipeline(ctx, org, mr)
	if err != nil {
		return nil, time.Time{}, err
	}
	at := mr.ObservedAt
	if run.Run.StartedAt != nil {
		at = *run.Run.StartedAt
	}
	profile, entries, err := u.summaryProfile(ctx, org, mr.AreaID, at)
	if err != nil || profile.IsZero() {
		return nil, time.Time{}, err
	}
	mr.BaseSHA = cmp.Or(mr.MergeBaseSHA, mr.BaseSHA)
	v := &entity.SummaryRevision{
		MRID: mr.MRID, HeadSHA: mr.HeadSHA, BaseSHA: mr.BaseSHA,
		ProfileVersionID: profile, KnowledgeSeq: mr.KnowledgeSeq, PipelineState: "complete",
	}
	v.Technical.EffectiveAt = mr.ObservedAt
	if mr.SourceUpdatedAt != nil {
		v.Technical.EffectiveAt = *mr.SourceUpdatedAt
	}
	for _, stamp := range []*time.Time{run.Run.StartedAt, run.Run.FinishedAt, run.Run.SourceUpdatedAt} {
		if stamp != nil && stamp.After(v.Technical.EffectiveAt) {
			v.Technical.EffectiveAt = *stamp
		}
	}
	if !entity.ValidGitSHA(v.BaseSHA) {
		v.BaseSHA, mr.BaseSHA = "", ""
	}
	last, err := u.summarizeReports(ctx, org, mr, entries, v)
	if err != nil {
		return nil, time.Time{}, err
	}
	if v.PipelineState != "complete" {
		v.PipelineState = "running"
		switch run.Run.Outcome {
		case entity.CISuccess, entity.CIFailed, entity.CICanceled, entity.CISkipped:
			v.PipelineState = "reports_missing"
		case entity.CIRunning, entity.CIUnknown:
			// Keep the pipeline running while awaiting reports or a terminal outcome.
		}
	}
	if err := u.d.Summary.Calculations.SummarizeMetrics(ctx, org, mr, run, u.summaryCheckJobs(entries), v); err != nil {
		return nil, time.Time{}, err
	}
	if !v.ComparableBase {
		v.BaseSHA = ""
	}
	entity.RankSummary(v.Items)
	if err := hashSummary(v); err != nil {
		return nil, time.Time{}, err
	}
	return v, last, nil
}

func (u *UseCase) summaryProfile(ctx context.Context, org, area entity.ID, at time.Time) (entity.ID, []entity.ProfileEntry, error) {
	profile, entries, err := u.profileAtArea(ctx, org, area, 0, at)
	return profile.ID, entries, err
}

func (u *UseCase) reportProfile(ctx context.Context, org, area entity.ID, seq int64, at time.Time) (entity.ID, []entity.ProfileEntry, error) {
	profile, entries, err := u.profileAtArea(ctx, org, area, seq, at)
	return profile.ProjectID, entries, err
}

func (u *UseCase) profileAtArea(ctx context.Context, org, area entity.ID, seq int64, at time.Time) (entity.CheckProfileVersion, []entity.ProfileEntry, error) {
	links, err := u.d.Components.ProjectAreas(ctx, org, at)
	if err != nil {
		return entity.CheckProfileVersion{}, nil, err
	}
	var projects []entity.ID
	for _, link := range links {
		if link.To == area && !slices.Contains(projects, link.From) {
			projects = append(projects, link.From)
		}
	}
	slices.SortFunc(projects, compareIDs)
	for _, id := range projects {
		profile, err := u.d.Summary.Store.SummaryProfileAt(ctx, org, id, at, seq)
		if errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return entity.CheckProfileVersion{}, nil, err
		}
		components, err := u.d.Components.ProjectComponents(ctx, org, id, at)
		if err != nil {
			return entity.CheckProfileVersion{}, nil, err
		}
		entries := slices.DeleteFunc(profile.Entries, func(e entity.ProfileEntry) bool {
			return !slices.ContainsFunc(components, func(c entity.Component) bool { return c.ID == e.ComponentID && c.RepositoryAreaID == area })
		})
		if len(entries) > 0 {
			return profile, entries, nil
		}
	}
	return entity.CheckProfileVersion{}, nil, nil
}

func hashSummary(v *entity.SummaryRevision) error {
	slices.SortFunc(v.Inputs, func(a, b entity.SummaryInput) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), compareIDs(a.ID, b.ID))
	})
	v.Inputs = slices.Compact(v.Inputs)
	body, err := json.Marshal(struct {
		Head, Base, Pipeline string
		Profile              entity.ID
		Inputs               []entity.SummaryInput
		Items                []entity.SummaryItem
		Definitions          []entity.RevisionInput
		Findings             []entity.SummaryFindingValues
		Technical            entity.TechnicalSummary
	}{v.HeadSHA, v.BaseSHA, v.PipelineState, v.ProfileVersionID, v.Inputs, v.Items, v.Definitions, v.FindingValues, v.Technical})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	v.InputHash = sum[:]
	return nil
}
