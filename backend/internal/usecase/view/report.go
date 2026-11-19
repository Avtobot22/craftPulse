package view

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_operationRevision = "getRevision"
	_operationCohort   = "listRevisionCohort"
	// _jiraIssuePath is where Jira Cloud and Data Center show an issue by its key.
	_jiraIssuePath = "/browse/"
)

// Reports is the calc module as the report reads it: the series of a team, its revisions and their inputs, the
// narrowing of a revision to a reader and the registry of definitions (TZ 10.4, 12.7).
type Reports interface {
	TeamSeries(ctx context.Context, orgID, teamID entity.ID, step entity.Step) (entity.CalcSeries, error)
	Revision(ctx context.Context, orgID, seriesID entity.ID, revisionID *entity.ID) (entity.CalcRevision, error)
	RevisionOf(ctx context.Context, orgID, revisionID entity.ID) (entity.CalcRevision, error)
	RevisionInputs(ctx context.Context, orgID, revisionID entity.ID) ([]entity.RevisionInput, error)
	RequestTeamRevision(ctx context.Context, orgID, teamID, corr entity.ID) error
	VisibleSeries(ctx context.Context, s entity.Subject, scope entity.Scope, rev entity.CalcRevision, teamID entity.ID,
		f entity.ReportFilter) (entity.VisibleSeries, error)
	VisibleCohort(ctx context.Context, s entity.Subject, scope entity.Scope, rev entity.CalcRevision, teamID entity.ID,
		q entity.CohortQuery) (entity.CohortPage, error)
	MetricDefinitions() []entity.MetricDefinition
	ReportSeries(ctx context.Context, orgID, seriesID entity.ID) (entity.CalcSeries, error)
	CreateSavedView(ctx context.Context, v entity.SavedView) error
	SavedView(ctx context.Context, orgID, id entity.ID) (entity.SavedView, error)
	ListSavedViews(ctx context.Context, orgID, ownerID entity.ID, q entity.SavedViewQuery) (entity.SavedViewPage, error)
	DeleteSavedView(ctx context.Context, orgID, ownerID, id entity.ID) error
}

// Instances is the connect module: the source instance an object came from, for the link to the source.
type Instances interface {
	Instance(ctx context.Context, orgID, id entity.ID) (entity.SourceInstance, error)
}

// TeamReport reads the series of the team by the current revision of the series of the step or by the given
// revision of the team, narrowed to what the reader may see, with the points of [From, To) (FR-021, FR-023).
// A series without a published revision gets its first chain and is entity.ErrNotFound until it is published.
func (u *UseCase) TeamReport(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.ReportQuery) (entity.TeamReport, error) {
	if err := checkReport(&q); err != nil {
		return entity.TeamReport{}, err
	}
	org := s.OrganizationID
	rev, err := u.reportRevision(ctx, org, &q)
	if err != nil {
		return entity.TeamReport{}, err
	}
	vs, err := u.d.Reports.VisibleSeries(ctx, s, scope, rev, q.TeamID, q.Filter)
	if err != nil {
		return entity.TeamReport{}, err
	}
	out := entity.TeamReport{Revision: rev, Notes: vs.Notes, Series: make([]entity.Series, 0, len(vs.Series))}
	for i := range vs.Series {
		series := vs.Series[i]
		series.Points = slices.DeleteFunc(slices.Clone(series.Points), func(p entity.Point) bool {
			return p.Start.Before(q.From) || !p.Start.Before(q.To)
		})
		out.Series = append(out.Series, series)
	}
	return out, nil
}

func (u *UseCase) reportRevision(ctx context.Context, org entity.ID, q *entity.ReportQuery) (entity.CalcRevision, error) {
	if q.RevisionID != nil {
		rev, err := u.d.Reports.RevisionOf(ctx, org, *q.RevisionID)
		if err != nil {
			return entity.CalcRevision{}, err
		}
		team, err := u.d.Series.SeriesTeam(ctx, org, rev.SeriesID)
		if err != nil {
			return entity.CalcRevision{}, err
		}
		if team != q.TeamID || rev.PublishedAt == nil {
			return entity.CalcRevision{}, entity.ErrNotFound
		}
		return rev, nil
	}
	series, err := u.d.Reports.TeamSeries(ctx, org, q.TeamID, q.Step)
	if err != nil {
		return entity.CalcRevision{}, err
	}
	rev, err := u.d.Reports.Revision(ctx, org, series.ID, nil)
	if errors.Is(err, entity.ErrNotFound) {
		if err := u.d.Reports.RequestTeamRevision(ctx, org, q.TeamID, entity.ID(logger.CorrelationID(ctx))); err != nil {
			return entity.CalcRevision{}, err
		}
	}
	return rev, err
}

func checkReport(q *entity.ReportQuery) error {
	v := &entity.ValidationError{}
	if !q.From.Before(q.To) {
		v.Add("to", entity.FieldOutOfRange, "must be later than from")
	}
	if q.Step != entity.StepWeek && q.Step != entity.StepMonth {
		v.Add("step", entity.FieldNotAllowedValue, "the series of a team are weekly or monthly")
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}

// Revision reads the passport of a revision of a team the reader may read: the definitions and the versions of
// the inputs it was computed with (DR-011). Any other revision is entity.ErrNotFound.
func (u *UseCase) Revision(ctx context.Context, s entity.Subject, revisionID entity.ID) (entity.RevisionPassport, error) {
	rev, _, _, err := u.readableRevision(ctx, s, revisionID, _operationRevision)
	if err != nil {
		return entity.RevisionPassport{}, err
	}
	inputs, err := u.d.Reports.RevisionInputs(ctx, s.OrganizationID, rev.ID)
	if err != nil {
		return entity.RevisionPassport{}, err
	}
	out := entity.RevisionPassport{Revision: rev}
	registry := u.d.Reports.MetricDefinitions()
	for _, in := range inputs {
		if in.Kind != entity.InputKindDefinition {
			out.Inputs = append(out.Inputs, in)
			continue
		}
		version, _ := strconv.Atoi(in.Version) // calc writes the version of a definition as a number
		def := entity.MetricDefinition{ID: in.ID, Version: version, Name: in.ID}
		if i := slices.IndexFunc(registry, func(d entity.MetricDefinition) bool { return d.ID == in.ID && d.Version == version }); i >= 0 {
			def = registry[i]
		}
		out.Definitions = append(out.Definitions, def)
	}
	return out, nil
}

// Cohort pages the objects of a point of a revision of a team the reader may see, with links to their sources.
func (u *UseCase) Cohort(ctx context.Context, s entity.Subject, revisionID entity.ID, q entity.CohortQuery) (entity.CohortPage, []string, error) {
	rev, team, scope, err := u.readableRevision(ctx, s, revisionID, _operationCohort)
	if err != nil {
		return entity.CohortPage{}, nil, err
	}
	page, err := u.d.Reports.VisibleCohort(ctx, s, scope, rev, team, q)
	if err != nil {
		return entity.CohortPage{}, nil, err
	}
	urls, err := u.sourceURLs(ctx, s.OrganizationID, page.Items)
	return page, urls, err
}

// MetricDefinitions is the registry of definitions in force.
func (u *UseCase) MetricDefinitions() []entity.MetricDefinition {
	return u.d.Reports.MetricDefinitions()
}

// readableRevision finds a published revision and its team and checks the action again for that team; a
// revision out of the rights looks like a missing one.
func (u *UseCase) readableRevision(ctx context.Context, s entity.Subject, revisionID entity.ID, operation string,
) (entity.CalcRevision, entity.ID, entity.Scope, error) {
	org := s.OrganizationID
	rev, err := u.d.Reports.RevisionOf(ctx, org, revisionID)
	if err != nil {
		return entity.CalcRevision{}, entity.ID{}, entity.Scope{}, err
	}
	if rev.PublishedAt == nil {
		return entity.CalcRevision{}, entity.ID{}, entity.Scope{}, entity.ErrNotFound
	}
	team, err := u.d.Series.SeriesTeam(ctx, org, rev.SeriesID)
	if err != nil {
		return entity.CalcRevision{}, entity.ID{}, entity.Scope{}, err
	}
	scope, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{
		Operation: operation, Action: _actionReadReport, Area: entity.AreaRef{Kind: entity.AreaTeam, ID: team},
	})
	if errors.Is(err, entity.ErrForbidden) {
		return entity.CalcRevision{}, entity.ID{}, entity.Scope{}, entity.ErrNotFound
	}
	return rev, team, scope, err
}

// sourceURLs are the addresses of the objects of a page in their sources, empty when unknown: a task of Jira
// is shown by its key under the base address of the instance.
func (u *UseCase) sourceURLs(ctx context.Context, org entity.ID, items []entity.CohortItem) ([]string, error) {
	out := make([]string, len(items))
	instances := map[entity.ID]entity.SourceInstance{}
	for i := range items {
		o := &items[i].Object
		if o.Deleted || o.InstanceID.IsZero() || o.DisplayKey == nil {
			continue
		}
		inst, ok := instances[o.InstanceID]
		if !ok {
			var err error
			if inst, err = u.d.Instances.Instance(ctx, org, o.InstanceID); err != nil && !errors.Is(err, entity.ErrNotFound) {
				return nil, err
			}
			instances[o.InstanceID] = inst
		}
		if inst.Provider == entity.ProviderJiraCloud && inst.BaseURL != "" && o.Kind == entity.ObjectTask {
			out[i] = inst.BaseURL + _jiraIssuePath + *o.DisplayKey
		}
	}
	return out, nil
}
