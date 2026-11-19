package view

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _savedPeriodDays = 90

// savedFilters describes the supported UI-20 parameters. Other UI state stays opaque; no field can
// supply a subject, scope, access grant or a different revision.
type savedFilters struct {
	From       *time.Time     `json:"from"`
	To         *time.Time     `json:"to"`
	PeriodDays *int           `json:"period_days"`
	Step       entity.Step    `json:"step"`
	Projects   []entity.ID    `json:"project_id"`
	TaskType   *string        `json:"task_type"`
	SizeLabel  *string        `json:"size_label"`
	GroupBy    entity.GroupBy `json:"group_by"`
}

func parseSavedFilters(raw json.RawMessage, step entity.Step) (savedFilters, error) {
	f := savedFilters{Step: step}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) > 50 {
		return f, savedField("filters", entity.FieldInvalidFormat)
	}
	for _, key := range []string{"from", "to", "period_days", "step", "project_id", "task_type", "size_label", "group_by"} {
		if value, ok := fields[key]; ok && string(value) == "null" {
			return f, savedField("filters."+key, entity.FieldInvalidFormat)
		}
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, savedField("filters", entity.FieldInvalidFormat)
	}
	if f.Step != step || (f.Step != entity.StepWeek && f.Step != entity.StepMonth) {
		return f, savedField("filters.step", entity.FieldNotAllowedValue)
	}
	if (f.From == nil) != (f.To == nil) || (f.From != nil && !f.From.Before(*f.To)) {
		return f, savedField("filters.to", entity.FieldOutOfRange)
	}
	if f.PeriodDays != nil && (*f.PeriodDays < 1 || *f.PeriodDays > 3660 || f.From != nil) {
		return f, savedField("filters.period_days", entity.FieldOutOfRange)
	}
	if f.GroupBy != "" && f.GroupBy != entity.GroupTaskType && f.GroupBy != entity.GroupSizeLabel {
		return f, savedField("filters.group_by", entity.FieldNotAllowedValue)
	}
	return f, nil
}

func savedField(field, code string) error {
	return &entity.ValidationError{Fields: []entity.FieldError{{Field: field, Code: code}}}
}

func (u *UseCase) savedSeries(ctx context.Context, s entity.Subject, seriesID entity.ID, operation string,
) (entity.CalcSeries, entity.ID, entity.Scope, error) {
	series, err := u.d.Reports.ReportSeries(ctx, s.OrganizationID, seriesID)
	if err != nil {
		return entity.CalcSeries{}, entity.ID{}, entity.Scope{}, err
	}
	team, err := u.d.Series.SeriesTeam(ctx, s.OrganizationID, seriesID)
	if err != nil {
		return entity.CalcSeries{}, entity.ID{}, entity.Scope{}, err
	}
	scope, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{
		Operation: operation, Action: _actionReadReport, Area: entity.AreaRef{Kind: entity.AreaTeam, ID: team},
	})
	if errors.Is(err, entity.ErrForbidden) {
		err = entity.ErrNotFound
	}
	return series, team, scope, err
}

func (u *UseCase) CreateSavedView(ctx context.Context, s entity.Subject, v entity.SavedView) (entity.SavedView, error) {
	if strings.TrimSpace(v.Name) == "" || utf8.RuneCountInString(v.Name) > 200 {
		return entity.SavedView{}, savedField("name", entity.FieldOutOfRange)
	}
	series, _, _, err := u.savedSeries(ctx, s, v.SeriesID, "createSavedView")
	if err != nil {
		return entity.SavedView{}, err
	}
	if _, err := parseSavedFilters(v.Filters, series.Step); err != nil {
		return entity.SavedView{}, err
	}
	if v.PinnedRevisionID != nil {
		rev, err := u.d.Reports.Revision(ctx, s.OrganizationID, series.ID, v.PinnedRevisionID)
		if err != nil {
			return entity.SavedView{}, err
		}
		if rev.PublishedAt == nil {
			return entity.SavedView{}, entity.ErrNotFound
		}
	}
	if v.ID, err = u.d.NewID(); err != nil {
		return entity.SavedView{}, err
	}
	v.OrganizationID, v.OwnerUserID, v.CreatedAt = s.OrganizationID, s.UserID, u.d.Clock.Now()
	if err := u.d.Reports.CreateSavedView(ctx, v); err != nil {
		return entity.SavedView{}, err
	}
	return v, nil
}

func (u *UseCase) ListSavedViews(ctx context.Context, s entity.Subject, q entity.SavedViewQuery) (entity.SavedViewPage, error) {
	return u.d.Reports.ListSavedViews(ctx, s.OrganizationID, s.UserID, q)
}

// OpenSavedView checks the team and cohort again for the opener, then uses the same TeamReport path as
// an ordinary report. A pinned query always resolves against its own revision, including relative dates.
func (u *UseCase) OpenSavedView(ctx context.Context, s entity.Subject, id entity.ID) (entity.OpenSavedView, error) {
	v, err := u.d.Reports.SavedView(ctx, s.OrganizationID, id)
	if err != nil {
		return entity.OpenSavedView{}, err
	}
	series, team, scope, err := u.savedSeries(ctx, s, v.SeriesID, "getSavedView")
	if err != nil {
		return entity.OpenSavedView{}, err
	}
	f, err := parseSavedFilters(v.Filters, series.Step)
	if err != nil {
		return entity.OpenSavedView{}, err
	}
	rev, err := u.d.Reports.Revision(ctx, s.OrganizationID, series.ID, v.PinnedRevisionID)
	if err != nil {
		return entity.OpenSavedView{}, err
	}
	q := f.query(team, &rev)
	r, err := u.TeamReport(ctx, s, scope, q)
	if err != nil {
		return entity.OpenSavedView{}, err
	}
	return entity.OpenSavedView{View: v, Query: q, Report: r}, nil
}

func (f *savedFilters) query(team entity.ID, rev *entity.CalcRevision) entity.ReportQuery {
	q := entity.ReportQuery{
		TeamID: team, Step: f.Step, RevisionID: &rev.ID,
		Filter: entity.ReportFilter{Projects: f.Projects, TaskType: f.TaskType, SizeLabel: f.SizeLabel, GroupBy: f.GroupBy},
	}
	if f.SizeLabel != nil && *f.SizeLabel == "unknown" {
		q.Filter.UnknownSize, q.Filter.SizeLabel = true, nil
	}
	if f.From != nil {
		q.From, q.To = *f.From, *f.To
		return q
	}
	days := _savedPeriodDays
	if f.PeriodDays != nil {
		days = *f.PeriodDays
	}
	q.To = rev.AsOf
	q.From = q.To.AddDate(0, 0, -days)
	return q
}

func (u *UseCase) DeleteSavedView(ctx context.Context, s entity.Subject, id entity.ID) error {
	return u.d.Reports.DeleteSavedView(ctx, s.OrganizationID, s.UserID, id)
}
