package gitlab

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _childPipelineSource = "parent_pipeline"

var errPipelineSelectionFilled = errors.New("GitLab pipeline selection filled")

type pipelineKey struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type pipeline struct {
	pipelineKey
	IID        int64      `json:"iid"`
	Ref        string     `json:"ref"`
	SHA        string     `json:"sha"`
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func pipelinePath(projectID, pipelineID int64) string {
	return projectPath(projectID) + "/pipelines/" + id(pipelineID)
}

func (p pipelineKey) valid(projectID int64) bool {
	return p.ID > 0 && p.ProjectID == projectID && !p.UpdatedAt.IsZero()
}

func comparePipeline(a, b pipelineKey) int {
	if byTime := a.UpdatedAt.Compare(b.UpdatedAt); byTime != 0 {
		return byTime
	}
	return cmp.Compare(a.ID, b.ID)
}

func (s source) selectPipelines(ctx context.Context, projectID int64, period adaptersdk.Period, cursor *adaptersdk.Cursor, limit int) ([]pipelineKey, error) {
	after := pipelineKey{}
	if cursor != nil {
		n, ok := numericID(cursor.NativeId)
		if !ok && cursor.NativeId != "" {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab pipeline cursor")
		}
		after.ID, after.UpdatedAt = n, cursor.ModifiedAt
	}
	from := period.From
	if !after.UpdatedAt.IsZero() && (from == nil || after.UpdatedAt.After(*from)) {
		from = &after.UpdatedAt
	}

	query := url.Values{"order_by": {"updated_at"}, "sort": {"asc"}}
	if from != nil {
		// GitLab filters with a strict lower bound. Keep the entire timestamp for ID tie-breaking.
		query.Set("updated_after", from.Add(-time.Millisecond).UTC().Format(time.RFC3339Nano))
	}
	if period.To != nil {
		query.Set("updated_before", period.To.UTC().Format(time.RFC3339Nano))
	}
	items := make([]pipelineKey, 0, limit)
	visit := func(key pipelineKey) error {
		if !key.valid(projectID) {
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab pipeline has no identity or update time")
		}
		if comparePipeline(key, after) <= 0 || (from != nil && key.UpdatedAt.Before(*from)) ||
			(period.To != nil && !key.UpdatedAt.Before(*period.To)) {
			return nil
		}
		if len(items) == limit && key.UpdatedAt.After(items[len(items)-1].UpdatedAt) {
			return errPipelineSelectionFilled
		}
		at, found := slices.BinarySearchFunc(items, key, comparePipeline)
		if !found && at < limit {
			if len(items) == limit {
				items = items[:limit-1]
			}
			items = slices.Insert(items, at, key)
		}
		return nil
	}
	path := projectPath(projectID) + "/pipelines"
	for _, child := range []bool{false, true} {
		if child {
			query.Set("source", _childPipelineSource)
		}
		for page := 1; page > 0; {
			next, err := readPage(ctx, s, "pipelines", path, query, page, _sourcePageSize, visit)
			if errors.Is(err, errPipelineSelectionFilled) {
				break
			}
			if err != nil {
				return nil, err
			}
			page = next
		}
	}
	return items, nil
}

func selectPipelineIDs(keys []adaptersdk.ObjectKey, cursor *adaptersdk.Cursor) ([]pipelineKey, error) {
	var after int64
	if cursor != nil {
		var ok bool
		after, ok = numericID(cursor.NativeId)
		if !ok {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab pipeline selection cursor")
		}
	}
	items := make([]pipelineKey, 0, len(keys))
	for _, key := range keys {
		n, ok := numericID(key.NativeId)
		if !ok || key.NativeNs != "" || key.ObjectKind != adaptersdk.ObjectKeyObjectKindCiRun {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab object selection requires global pipeline IDs")
		}
		if n > after {
			items = append(items, pipelineKey{ID: n})
		}
	}
	slices.SortFunc(items, comparePipeline)
	return slices.Compact(items), nil
}

func (s source) fetchCIRuns(ctx context.Context, projectID int64, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	// A page carries one pipeline and its attempts, so report limits cannot split an attempt from its run.
	var items []pipelineKey
	var err error
	if len(in.ObjectKeys) == 0 {
		items, err = s.selectPipelines(ctx, projectID, in.Period, in.Cursor, 2)
	} else {
		items, err = selectPipelineIDs(in.ObjectKeys, in.Cursor)
	}
	if err != nil || len(items) == 0 {
		return err
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return err
	}
	m.Area.Key = &p.Path

	key := items[0]
	run, err := s.fetchPipeline(ctx, p, key.ID, m, page)
	if err != nil {
		return err
	}
	if len(items) > 1 {
		stamp := key.UpdatedAt
		if stamp.IsZero() {
			stamp = run.UpdatedAt
		}
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: stamp, NativeId: id(key.ID)}
	}
	return nil
}

func (s source) fetchPipeline(ctx context.Context, project project, pipelineID int64, m *envelope.Manifest, page *adaptersdk.Page) (pipeline, error) {
	var run pipeline
	if err := s.get(ctx, "pipeline", pipelinePath(project.ID, pipelineID), &run); err != nil {
		return pipeline{}, err
	}
	if run.ID != pipelineID || !run.valid(project.ID) || run.IID <= 0 || !validSHA(run.SHA) || run.Source == "" || run.Status == "" {
		return pipeline{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab pipeline has invalid metadata")
	}
	links, err := s.pipelineLinks(ctx, project, run)
	if err != nil {
		return pipeline{}, err
	}
	jobs, truncated, err := s.pipelineJobs(ctx, run)
	if err != nil {
		return pipeline{}, err
	}
	if truncated {
		m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{
			RecordKind: envelope.TruncatedHistoryRecordKindCiRun, NativeId: id(run.ID),
		})
	}
	payload := envelope.CiRunV2{
		ProjectNativeId: id(project.ID), RefName: run.Ref, Sha: run.SHA, Source: optional(run.Source),
		MrNativeId: optional(links.mr), ParentRunNativeId: optional(links.parent), TriggerJobNativeId: optional(links.trigger),
		NativeStatus: run.Status, Outcome: envelope.CiRunV2Outcome(ciOutcome(run.Status)),
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, UpdatedAt: &run.UpdatedAt,
	}
	if truncated {
		payload.AttemptHistoryTruncated = &truncated
	}
	if err := addCIRecord(page, "ci_run", "2", id(run.ID), m.CreatedAt, payload); err != nil {
		return pipeline{}, err
	}
	return run, s.addPipelineAttempts(ctx, run, payload.ParentRunNativeId, jobs, m.CreatedAt, page)
}

func (s source) manifestCIRuns(ctx context.Context, projectID int64, in adaptersdk.ManifestInput, cursor *adaptersdk.Cursor) (adaptersdk.ManifestPage, error) {
	limit := min(in.Limit, 100)
	items, err := s.selectPipelines(ctx, projectID, in.Period, cursor, limit+1)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	out := adaptersdk.ManifestPage{Items: make([]adaptersdk.ManifestItem, 0, min(len(items), limit))}
	for _, item := range items[:min(len(items), limit)] {
		out.Items = append(out.Items, adaptersdk.ManifestItem{NativeId: id(item.ID), RecordKind: "ci_run", UpdatedAt: &item.UpdatedAt})
	}
	if len(items) > limit {
		last := items[limit-1]
		out.NextCursor = optional(last.UpdatedAt.UTC().Format(time.RFC3339Nano) + "/" + id(last.ID))
	}
	return out, nil
}

type ciLinks struct {
	mr, parent, trigger string
}

func (s source) pipelineLinks(ctx context.Context, project project, run pipeline) (ciLinks, error) {
	// The REST pipeline detail has no MR association or incoming trigger. Use GitLab's links.
	if run.Source != "merge_request_event" && run.Source != _childPipelineSource {
		return ciLinks{}, nil
	}
	query := "query($path: ID!, $iid: ID!) { project(fullPath: $path) { pipeline(iid: $iid) { upstream { id project { id } } sourceJob { id } mergeRequest { id project { id } } } } }"
	variables, err := json.Marshal(map[string]string{"path": project.Path, "iid": strconv.FormatInt(run.IID, 10)})
	if err != nil {
		return ciLinks{}, err
	}
	resp, err := s.requestURL(ctx, "pipeline_links", "/api/graphql", url.Values{"query": {query}, "variables": {string(variables)}}, "application/json", _maxSourcePageBytes) //nolint:bodyclose // DecodeJSON owns the body
	if err != nil {
		return ciLinks{}, err
	}
	type linked struct {
		ID      string `json:"id"`
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	var result struct {
		Data struct {
			Project struct {
				Pipeline *struct {
					Upstream     linked `json:"upstream"`
					SourceJob    linked `json:"sourceJob"`    //nolint:tagliatelle // GitLab GraphQL field
					MergeRequest linked `json:"mergeRequest"` //nolint:tagliatelle // GitLab GraphQL field
				} `json:"pipeline"`
			} `json:"project"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := adaptersdk.DecodeJSON(resp, &result); err != nil {
		return ciLinks{}, err
	}
	if len(result.Errors) != 0 {
		return ciLinks{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab pipeline link query failed")
	}
	node := result.Data.Project.Pipeline
	if node == nil {
		return ciLinks{}, nil
	}
	var links ciLinks
	projectGID := "gid://gitlab/Project/" + id(project.ID)
	if node.MergeRequest.Project.ID == projectGID {
		links.mr = gitLabGlobalID(node.MergeRequest.ID, "MergeRequest")
	}
	if node.Upstream.Project.ID == projectGID && node.Upstream.ID != "gid://gitlab/Ci::Pipeline/"+id(run.ID) {
		links.parent = gitLabGlobalID(node.Upstream.ID, "Ci::Pipeline")
	}
	if links.parent != "" {
		links.trigger, err = s.findTrigger(ctx, project.ID, links.parent, run.ID)
		if links.trigger == "" {
			links.trigger = gitLabGlobalID(node.SourceJob.ID, "Ci::Bridge", "Ci::Build")
		}
	}
	return links, err
}

func (s source) findTrigger(ctx context.Context, projectID int64, parent string, childID int64) (string, error) {
	type bridge struct {
		ID         int64       `json:"id"`
		Downstream pipelineKey `json:"downstream_pipeline"`
	}
	query := url.Values{}
	path := projectPath(projectID) + "/pipelines/" + parent + "/bridges"
	for page := 1; page > 0; {
		var found int64
		next, err := readPage(ctx, s, "pipeline_bridges", path, query, page, _sourcePageSize, func(j bridge) error {
			if j.Downstream.ID == childID && j.Downstream.ProjectID == projectID {
				found = j.ID
			}
			return nil
		})
		if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) || hasCode(err, adaptersdk.AdapterErrorCodeAuth) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if found > 0 {
			return id(found), nil
		}
		page = next
	}
	return "", nil
}
