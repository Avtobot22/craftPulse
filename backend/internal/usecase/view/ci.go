package view

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type CodeFacts interface {
	MRAt(context.Context, entity.Scope, entity.ID, entity.ID, int64) (entity.MRVersion, error)
	AllRepositoryAreas(context.Context, entity.ID) ([]entity.ID, error)
	RepositoryAreas(context.Context, entity.ID, []entity.ID) ([]entity.ID, error)
	RestrictedObjects(context.Context, entity.Scope, entity.ID, []entity.ID) ([]entity.RestrictedObject, error)
	ReportKnowledge(context.Context, entity.ID, *int64) (int64, error)
	PipelineTestExecutions(context.Context, entity.ID, entity.ReportScope, entity.TestExecutionQuery) (entity.TestExecutionList, error)
	CurrentObjects(context.Context, entity.ID, []entity.ID) ([]entity.LinkEnd, error)
	Repository(context.Context, entity.Scope, entity.ID, entity.ID) (entity.RepositoryVersion, error)
	RunAt(context.Context, entity.Scope, entity.ID, entity.ID, int64) (entity.CIRun, error)
}

type SourceRights interface {
	Decide(context.Context, entity.Subject, entity.Scope, entity.SourceQuery) (entity.Scope, error)
}

type CodeLinks interface {
	TaskMREdgesAt(context.Context, entity.ID, []entity.ID, int64) ([]entity.LinkEdge, error)
}

type RunCalculator interface {
	SummarizeRun(entity.CIRun) entity.RunSummary
	TestCoverageAt(context.Context, entity.ID, entity.ReportScope) (entity.TestCoverageRevision, error)
}

// codeScope resolves only the authorization metadata before consulting the source. Facts are read afterwards.
func (u *UseCase) codeScope(ctx context.Context, s entity.Subject, scope entity.Scope, op string, ids []entity.ID) (entity.Scope, map[entity.ID]entity.LinkEnd, error) {
	ends, err := u.d.Code.CurrentObjects(ctx, s.OrganizationID, ids)
	if err != nil {
		return entity.Scope{}, nil, err
	}
	var areas []entity.ID
	var objects []entity.RestrictedObject
	for _, end := range ends {
		areas = append(areas, end.AreaID)
		if end.Restricted {
			objects = append(objects, entity.RestrictedObject{ID: end.ID, AreaID: end.AreaID, Kind: end.Kind, NativeNS: end.NativeNS, NativeID: end.NativeID})
		}
	}
	scope, err = u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: op, Areas: areas})
	if err != nil {
		return entity.Scope{}, nil, err
	}
	if len(objects) > 0 {
		scope, err = u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: op, Objects: objects})
		if err != nil {
			return entity.Scope{}, nil, err
		}
	}
	visible := make(map[entity.ID]entity.LinkEnd, len(ends))
	for _, end := range ends {
		if scope.Area(end.AreaID) && (!end.Restricted || slices.Contains(scope.Objects(), end.ID)) {
			visible[end.ID] = end
		}
	}
	return scope, visible, nil
}

func (u *UseCase) MergeRequest(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.MRDetail, error) {
	scope, ends, err := u.codeScope(ctx, s, scope, "getMergeRequest", []entity.ID{id})
	if err != nil {
		return entity.MRDetail{}, err
	}
	end, ok := ends[id]
	if !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.MRDetail{}, entity.ErrNotFound
	}
	asOf := u.d.Clock.Now()
	seq, err := u.d.Code.ReportKnowledge(ctx, s.OrganizationID, nil)
	if err != nil {
		return entity.MRDetail{}, err
	}
	v, err := u.d.Code.MRAt(ctx, scope, s.OrganizationID, id, seq)
	if err != nil {
		return entity.MRDetail{}, err
	}
	if !scope.Area(v.AreaID) {
		return entity.MRDetail{}, entity.ErrNotFound
	}
	repository, err := u.d.Code.Repository(ctx, scope, s.OrganizationID, v.AreaID)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return entity.MRDetail{}, err
	}
	out := entity.MRDetail{Version: v, Repository: repository, AuthorKind: entity.PersonUnknown, Links: []entity.LinkResult{}}
	if repository.WebURL != "" && v.IID != nil {
		instance, err := u.d.Instances.Instance(ctx, s.OrganizationID, end.InstanceID)
		if err != nil {
			return entity.MRDetail{}, err
		}
		out.SourceURL = mergeRequestURL(repository.WebURL, instance.Provider, *v.IID)
	}
	reviewScope, _, err := u.reviewScope(ctx, s, scope, "getMergeRequest", []entity.ID{v.AreaID})
	if err != nil {
		return entity.MRDetail{}, err
	}
	reviews, err := u.d.ReviewCalculator.MRReviewsAt(ctx, reviewScope, s.OrganizationID, v.AreaID, seq, asOf)
	if err != nil {
		return entity.MRDetail{}, err
	}
	for i := range reviews {
		if reviews[i].Version.MRID == v.MRID {
			out.ReviewWait, out.AuthorKind = reviews[i].Wait, reviews[i].AuthorKind
			break
		}
	}
	edges, err := u.d.CodeLinks.TaskMREdgesAt(ctx, s.OrganizationID, []entity.ID{id}, math.MaxInt64)
	if err != nil {
		return entity.MRDetail{}, err
	}
	var tasks []entity.ID
	for i := range edges {
		edge := &edges[i]
		if edge.Type == entity.LinkTaskMR && edge.To == id && edge.Status != entity.LinkRejected {
			tasks = append(tasks, edge.From)
		}
	}
	if len(tasks) > 0 {
		_, visible, err := u.codeScope(ctx, s, scope, "getMergeRequest", tasks)
		if err != nil {
			return entity.MRDetail{}, err
		}
		for i := range edges {
			edge := &edges[i]
			if edge.Type != entity.LinkTaskMR || edge.To != id || edge.Status == entity.LinkRejected {
				continue
			}
			end, ok := visible[edge.From]
			if !ok || end.Kind != entity.ObjectTask {
				out.Notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
				continue
			}
			out.Links = append(out.Links, entity.LinkResult{Edge: *edge, To: end})
		}
	}
	return out, nil
}

func mergeRequestURL(base string, provider entity.Provider, iid int) *string {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil
	}
	var route string
	switch provider {
	case entity.ProviderGitLab:
		route = "/-/merge_requests/"
	case entity.ProviderGitHub:
		route = "/pull/"
	default:
		return nil
	}
	u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
	result := strings.TrimRight(u.String(), "/") + route + strconv.Itoa(iid)
	return &result
}

func (u *UseCase) Pipeline(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.PipelineDetail, error) {
	seq, err := u.d.Code.ReportKnowledge(ctx, s.OrganizationID, nil)
	if err != nil {
		return entity.PipelineDetail{}, err
	}
	out, allowed, err := u.readPipeline(ctx, s, scope, id, "getPipeline", seq)
	if err != nil {
		return entity.PipelineDetail{}, err
	}
	out.Metrics, err = u.d.RunCalculator.TestCoverageAt(ctx, s.OrganizationID, pipelineReportScope(&out.Run, seq))
	if err != nil {
		return entity.PipelineDetail{}, err
	}
	r := &out.Run.Run
	at := r.ObservedAt
	if r.StartedAt != nil {
		at = *r.StartedAt
	}
	expected, err := u.d.PipelineChecks.ExpectedPipelineReports(ctx, allowed, s.OrganizationID, r.AreaID, at)
	if err != nil {
		return entity.PipelineDetail{}, err
	}
	out.Reports = pipelineReports(expected, out.Metrics.Reports)
	return out, nil
}

func (u *UseCase) readPipeline(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, op string, seq int64) (entity.PipelineDetail, entity.Scope, error) {
	scope, ends, err := u.codeScope(ctx, s, scope, op, []entity.ID{id})
	if err != nil {
		return entity.PipelineDetail{}, entity.Scope{}, err
	}
	if end, ok := ends[id]; !ok || end.Kind != entity.ObjectCIRun {
		return entity.PipelineDetail{}, entity.Scope{}, entity.ErrNotFound
	}
	run, err := u.d.Code.RunAt(ctx, scope, s.OrganizationID, id, seq)
	if err != nil {
		return entity.PipelineDetail{}, entity.Scope{}, err
	}
	if run.Run.MRID != nil {
		_, visible, err := u.codeScope(ctx, s, scope, op, []entity.ID{*run.Run.MRID})
		if err != nil {
			return entity.PipelineDetail{}, entity.Scope{}, err
		}
		if _, allowed := visible[*run.Run.MRID]; !allowed {
			run.Run.MRID = nil
		}
	}
	return entity.PipelineDetail{Run: run, Summary: u.d.RunCalculator.SummarizeRun(run)}, scope, nil
}

func (u *UseCase) PipelineAttempts(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, q entity.AttemptQuery) (entity.AttemptList, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return entity.AttemptList{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/limit", Code: entity.FieldOutOfRange}}}
	}
	issued := u.d.Clock.Now()
	if q.IssuedAt != nil {
		issued = *q.IssuedAt
		if issued.After(u.d.Clock.Now()) || u.d.Clock.Now().Sub(issued) > 24*time.Hour {
			return entity.AttemptList{}, entity.ErrInvalidCursor
		}
	}
	detail, _, err := u.readPipeline(ctx, s, scope, id, "listPipelineAttempts", math.MaxInt64)
	if err != nil {
		return entity.AttemptList{}, err
	}
	attempts := slices.Clone(detail.Run.Attempts)
	slices.SortFunc(attempts, func(a, b entity.CIAttemptVersion) int { return bytes.Compare(a.AttemptID[:], b.AttemptID[:]) })
	out := entity.AttemptList{IssuedAt: issued, Items: []entity.CIAttemptVersion{}, UsageSeconds: detail.Summary.UsageSeconds}
	for i := range attempts {
		attempt := &attempts[i]
		if q.Outcome != "" && attempt.Outcome != q.Outcome || q.After != nil && bytes.Compare(attempt.AttemptID[:], q.After[:]) <= 0 {
			continue
		}
		if len(out.Items) == q.Limit {
			last := out.Items[len(out.Items)-1].AttemptID
			out.Next = &last
			break
		}
		out.Items = append(out.Items, *attempt)
	}
	return out, nil
}
