package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (h *findings) ListFindings(ctx context.Context, req openapi.ListFindingsRequestObject) (openapi.ListFindingsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	q, err := findingsQuery(req.Params)
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256([]byte(strings.Join([]string{
		s.OrganizationID.String(), s.UserID.String(), q.ComponentID.String(), q.ProjectID.String(),
		q.ServiceID.String(), q.Kind, q.State, q.Severity, q.Sort,
	}, ":")))
	filter := base64.RawURLEncoding.EncodeToString(digest[:])
	now := time.Now().UTC()
	started := now
	const cursorKind = "findings"
	if raw := req.Params.Cursor; raw != nil {
		if len(*raw) > 512 {
			return nil, errMalformedPage
		}
		c, ok := decodeCursor(*raw)
		if !ok || c.Kind != cursorKind || c.Name != filter || c.ID == uuid.Nil || c.FirstSeenAt.IsZero() ||
			c.Severity < 0 || c.Severity > entity.SeverityOrder("unknown") || c.At.After(now) || now.Sub(c.At) >= 24*time.Hour {
			return nil, errMalformedPage
		}
		started = c.At
		q.After = &entity.FindingPosition{ID: entity.ID(c.ID), FirstSeenAt: c.FirstSeenAt, Severity: c.Severity}
	}
	page, err := h.uc.ListFindings(ctx, s, scope, q)
	if err != nil {
		return nil, err
	}
	out := openapi.FindingPage{Data: make([]openapi.Finding, len(page.Items)), AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes)}
	for i := range page.Items {
		out.Data[i], _, err = findingSummary(&page.Items[i])
		if err != nil {
			return nil, err
		}
	}
	if n := page.Next; n != nil {
		out.NextCursor = new(encodeCursor(cursorData{
			Kind: cursorKind, Name: filter, ID: uuid.UUID(n.ID),
			FirstSeenAt: n.FirstSeenAt, Severity: n.Severity, At: started,
		}))
	}
	return openapi.ListFindings200JSONResponse(out), nil
}

func findingsQuery(p openapi.ListFindingsParams) (entity.FindingQuery, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.FindingQuery{}, err
	}
	if (p.FindingKind != nil && !p.FindingKind.Valid()) || (p.State != nil && !p.State.Valid()) ||
		(p.Severity != nil && !p.Severity.Valid()) || (p.Sort != nil && !p.Sort.Valid()) {
		return entity.FindingQuery{}, errMalformedPage
	}
	for _, id := range []*uuid.UUID{p.ComponentId, p.ProjectId, p.ServiceId} {
		if id != nil && *id == uuid.Nil {
			return entity.FindingQuery{}, errMalformedPage
		}
	}
	q := entity.FindingQuery{Limit: limit, Sort: "-severity"}
	if p.ComponentId != nil {
		q.ComponentID = entity.ID(*p.ComponentId)
	}
	if p.ProjectId != nil {
		q.ProjectID = entity.ID(*p.ProjectId)
	}
	if p.ServiceId != nil {
		q.ServiceID = entity.ID(*p.ServiceId)
	}
	if p.FindingKind != nil {
		q.Kind = string(*p.FindingKind)
	}
	if p.State != nil {
		q.State = string(*p.State)
	}
	if p.Severity != nil {
		q.Severity = string(*p.Severity)
	}
	if p.Sort != nil {
		q.Sort = string(*p.Sort)
	}
	return q, nil
}

func (h *findings) GetComponentOverview(ctx context.Context, req openapi.GetComponentOverviewRequestObject) (openapi.GetComponentOverviewResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	overview, err := h.uc.ComponentOverview(ctx, s, scope, entity.ID(req.ComponentId))
	if err != nil {
		return nil, err
	}
	out := openapi.ComponentOverview{
		Component: response.Component(&overview.Component), AccessLimited: len(overview.Notes) > 0, AccessNotes: accessNotes(overview.Notes),
		Redactions: openapi.Redactions{},
	}
	out.Facts = emptyOf(out.Facts, len(overview.Facts))
	out.FindingsByState = emptyOf(out.FindingsByState, len(overview.Counts))
	out.Hotspots = emptyOf(out.Hotspots, 0)
	for i := range overview.Facts {
		f := &overview.Facts[i]
		item := elemOf(out.Facts)
		item.MetricId, item.Source = f.MetricID, openapi.ComponentOverviewFactsSource(f.Source)
		item.Window = openapi.Period{From: f.Window.From, To: f.Window.To}
		item.Value = metricValue(f.Unit, &f.Point)
		out.Facts = append(out.Facts, *item)
	}
	for _, count := range overview.Counts {
		item := elemOf(out.FindingsByState)
		item.FindingKind, item.State = openapi.FindingKind(count.Kind), openapi.FindingState(count.State)
		p := entity.Point{State: entity.StateAssessed, Value: new(float64(count.Count)), N: &count.Count, LowSample: count.Count < 20}
		item.Count = metricValue("count", &p)
		out.FindingsByState = append(out.FindingsByState, *item)
	}
	return openapi.GetComponentOverview200JSONResponse(out), nil
}
