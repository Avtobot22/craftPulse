package view

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type FindingReader interface {
	GetFinding(context.Context, entity.Subject, entity.Scope, entity.ID) (entity.FindingDetail, error)
	Findings(context.Context, entity.ID, entity.FindingSelection) ([]entity.FindingDetail, error)
	ComponentCodeProfiles(context.Context, entity.ID, entity.ID, []entity.ID) ([]entity.ProfileEntry, error)
}

type FindingRights interface {
	VisibleFindings(context.Context, entity.Subject, entity.Scope, string, []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

type FindingStructure interface {
	ConfirmedComponent(context.Context, entity.ID, entity.ID) (entity.Component, error)
	Structure(context.Context, entity.Subject) (entity.Structure, error)
	ProjectAreas(context.Context, entity.ID, time.Time) ([]entity.Link, error)
}

type FindingRepositories interface {
	AllRepositoryAreas(context.Context, entity.ID) ([]entity.ID, error)
}

type ComponentCalculator interface {
	ComponentCodeFacts(context.Context, entity.ID, entity.Component, []entity.ProfileEntry) ([]entity.ComponentCodeFact, error)
}

type FindingDeps struct {
	Reader       FindingReader
	Rights       FindingRights
	Structure    FindingStructure
	Repositories FindingRepositories
	Calculator   ComponentCalculator
}

func (u *UseCase) GetFinding(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.FindingDetail, error) {
	return u.d.Findings.Reader.GetFinding(ctx, s, scope, id)
}

func (u *UseCase) ListFindings(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.FindingQuery) (entity.FindingPage, error) {
	areas, components, err := u.findingCandidates(ctx, s, scope, q)
	if err != nil {
		return entity.FindingPage{}, err
	}
	sel, notes, err := u.findingSelection(ctx, s, scope, "listFindings", areas, q.ProjectID)
	if err != nil {
		return entity.FindingPage{}, err
	}
	sel.Components, sel.Kind = components, q.Kind
	rows, err := u.d.Findings.Reader.Findings(ctx, s.OrganizationID, sel)
	if err != nil {
		return entity.FindingPage{}, err
	}
	rows = slices.DeleteFunc(rows, func(f entity.FindingDetail) bool {
		return (q.State != "" && f.State != q.State) ||
			(q.Severity != "" && q.Severity != cmp.Or(f.Observation.MappedSeverity, "unknown")) ||
			(q.After != nil && compareFindings(findingPosition(&f), *q.After, q.Sort) <= 0)
	})
	slices.SortFunc(rows, func(a, b entity.FindingDetail) int {
		return compareFindings(findingPosition(&a), findingPosition(&b), q.Sort)
	})
	page := entity.FindingPage{Items: rows, Notes: notes}
	if len(rows) > q.Limit {
		page.Items = rows[:q.Limit]
		page.Next = new(findingPosition(&page.Items[q.Limit-1]))
	}
	return page, nil
}

func (u *UseCase) findingCandidates(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.FindingQuery) (areas, components []entity.ID, err error) {
	org := s.OrganizationID
	switch {
	case !q.ComponentID.IsZero():
		c, err := u.d.Findings.Structure.ConfirmedComponent(ctx, org, q.ComponentID)
		if errors.Is(err, entity.ErrNotFound) || (err == nil && !q.ServiceID.IsZero() && c.ServiceID != q.ServiceID) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		components, areas = []entity.ID{c.ID}, []entity.ID{c.RepositoryAreaID}
	case !q.ServiceID.IsZero():
		st, err := u.d.Findings.Structure.Structure(ctx, s)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range st.Components {
			if c.ServiceID == q.ServiceID && c.Status == entity.ComponentConfirmed {
				components = append(components, c.ID)
				areas = append(areas, c.RepositoryAreaID)
			}
		}
		if len(components) == 0 {
			return nil, nil, nil
		}
	default:
		if scope.Whole() && q.ProjectID.IsZero() {
			areas, err := u.d.Findings.Repositories.AllRepositoryAreas(ctx, org)
			return areas, nil, err
		}
	}
	if scope.Whole() && q.ProjectID.IsZero() {
		return areas, components, nil
	}
	links, err := u.findingProjectAreas(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	projects := scope.Projects()
	var result []entity.ID
	for _, l := range links {
		if (q.ProjectID.IsZero() || l.From == q.ProjectID) && (scope.Whole() || slices.Contains(projects, l.From)) &&
			(areas == nil || slices.Contains(areas, l.To)) && !slices.Contains(result, l.To) {
			result = append(result, l.To)
		}
	}
	return result, components, nil
}

func (u *UseCase) findingSelection(ctx context.Context, s entity.Subject, scope entity.Scope, op string, areas []entity.ID,
	project entity.ID,
) (entity.FindingSelection, []entity.AccessNote, error) {
	targets := make([]entity.FindingAccess, 0, 2*len(areas))
	for _, area := range areas {
		targets = append(targets, entity.FindingAccess{AreaID: area, Kind: entity.FindingCode}, entity.FindingAccess{AreaID: area, Kind: entity.FindingSecurity})
	}
	allowed, err := u.d.Findings.Rights.VisibleFindings(ctx, s, scope, op, targets)
	if err != nil {
		return entity.FindingSelection{}, nil, err
	}
	security := project.IsZero() || slices.Contains(scope.SecurityProjects(), project)
	var sel entity.FindingSelection
	var notes []entity.AccessNote
	for i := 0; i < len(targets); i += 2 {
		area := targets[i].AreaID
		if !allowed[targets[i]] {
			notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
			continue
		}
		sel.Areas = append(sel.Areas, area)
		if security && allowed[targets[i+1]] {
			sel.SecurityAreas = append(sel.SecurityAreas, area)
		} else {
			notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
		}
	}
	return sel, notes, nil
}

func findingPosition(f *entity.FindingDetail) entity.FindingPosition {
	return entity.FindingPosition{ID: f.ID, FirstSeenAt: f.FirstSeenAt, Severity: entity.SeverityOrder(f.Observation.MappedSeverity)}
}

func compareFindings(a, b entity.FindingPosition, sort string) int {
	var order int
	switch sort {
	case "first_seen_at":
		order = a.FirstSeenAt.Compare(b.FirstSeenAt)
	case "-first_seen_at":
		order = b.FirstSeenAt.Compare(a.FirstSeenAt)
	default:
		order = cmp.Compare(a.Severity, b.Severity)
		unknown := entity.SeverityOrder("unknown")
		if sort == "severity" && a.Severity != unknown && b.Severity != unknown {
			order = -order
		}
	}
	return cmp.Or(order, bytes.Compare(a.ID[:], b.ID[:]))
}

func (u *UseCase) findingProjectAreas(ctx context.Context, org entity.ID) ([]entity.Link, error) {
	var links []entity.Link
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		links, err = u.d.Findings.Structure.ProjectAreas(ctx, org, u.d.Clock.Now())
		return err
	})
	return links, err
}
