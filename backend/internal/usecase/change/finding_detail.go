package change

import (
	"context"
	"errors"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _getFinding = "getFinding"

func (u *UseCase) GetFinding(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.FindingDetail, error) {
	org := s.OrganizationID
	identity, err := u.FindingIdentity(ctx, org, id)
	if err != nil {
		return entity.FindingDetail{}, err
	}
	target := entity.FindingAccess{AreaID: identity.AreaID, Kind: identity.Kind}
	allowed, err := u.d.FindingRights.VisibleFindings(ctx, s, scope, _getFinding, []entity.FindingAccess{target})
	if err != nil {
		return entity.FindingDetail{}, err
	}
	if !allowed[target] {
		return entity.FindingDetail{}, entity.ErrNotFound
	}

	areas := []entity.ID{identity.AreaID}
	found, err := u.readFindings(ctx, org, id, entity.FindingSelection{Areas: areas, SecurityAreas: areas})
	if err != nil {
		return entity.FindingDetail{}, err
	}
	if len(found) == 0 {
		return entity.FindingDetail{}, entity.ErrNotFound
	}

	out := found[0]
	for _, action := range [...]struct{ permission, name string }{
		{"exceptions.manage", "create_exception"}, {"exceptions.review", "review_exception"},
	} {
		rights, err := u.d.FindingRights.Authorize(ctx, s, entity.AccessRequest{Operation: _getFinding, Action: action.permission, Optional: true})
		if errors.Is(err, entity.ErrForbidden) || errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return entity.FindingDetail{}, err
		}
		visible, err := u.d.FindingRights.VisibleFindings(ctx, s, rights, _getFinding, []entity.FindingAccess{target})
		if err != nil {
			return entity.FindingDetail{}, err
		}
		if visible[target] {
			out.Actions = append(out.Actions, action.name)
		}
	}
	return out, nil
}

func (u *UseCase) Findings(ctx context.Context, org entity.ID, selection entity.FindingSelection) ([]entity.FindingDetail, error) {
	return u.readFindings(ctx, org, entity.ID{}, selection)
}

type findingStates struct {
	byID     map[entity.ID]string
	fallback string
}

func (u *UseCase) readFindings(ctx context.Context, org, id entity.ID, selection entity.FindingSelection) ([]entity.FindingDetail, error) {
	if len(selection.Areas) == 0 {
		return nil, nil
	}
	var out []entity.FindingDetail
	err := u.d.Repo.ReadFindingSnapshot(ctx, org, func(ctx context.Context) error {
		rows, err := u.d.Repo.FindingObservations(ctx, org, id, selection)
		if err != nil || len(rows) == 0 {
			out = nil
			return err
		}
		seq, _, err := u.d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		at := u.d.Clock.Now().UTC()
		decisions := map[entity.ID][]entity.ExceptionVersion{}
		states := map[entity.ID]findingStates{}
		for i := range rows {
			f := &rows[i]
			f.State = entity.FindingNewlyObserved
			if f.ComponentID != nil && f.Report.SHA != "" {
				result, ok := states[f.Report.ID]
				if !ok {
					result.byID, result.fallback, err = u.findingStates(ctx, org, f.AreaID, *f.ComponentID, f.Report.ID, f.ReportKind)
					if err != nil {
						return err
					}
					states[f.Report.ID] = result
				}
				f.State = result.byID[f.ID]
				if f.State == "" {
					f.State = result.fallback
				}
			}
			if _, ok := decisions[f.AreaID]; !ok {
				decisions[f.AreaID], err = u.d.Decisions.ActiveAndExpiredAt(ctx, org, f.AreaID, seq, at)
				if err != nil {
					return err
				}
			}
			applyFindingExceptions(f, decisions[f.AreaID])
		}
		out = rows
		return nil
	})
	return out, err
}

func applyFindingExceptions(out *entity.FindingDetail, list []entity.ExceptionVersion) {
	coords := entity.FindingCoordinates{
		FindingAccess: entity.FindingAccess{AreaID: out.AreaID, Kind: out.Kind},
		FindingID:     out.ID, RuleID: out.Observation.RuleID, Path: out.Observation.Path, PackagePURL: out.Observation.PackagePURL,
	}
	if out.ComponentID != nil {
		coords.ComponentID = *out.ComponentID
	}
	var matched *entity.ExceptionVersion
	expired := false
	for i := range list {
		v := &list[i]
		if !v.Matches(coords) {
			continue
		}
		out.Exceptions = append(out.Exceptions, *v)
		if v.Status == entity.ExceptionExpired {
			expired = true
			continue
		}
		if matched == nil || (matched.Scope.FindingID.IsZero() && !v.Scope.FindingID.IsZero()) {
			matched = v
		}
	}
	switch {
	case out.State == entity.FindingResolved:
	case matched != nil:
		out.State = matched.Type
	case expired && out.State == entity.FindingNewlyObserved:
		out.State = entity.FindingOpen
	}
}

func (u *UseCase) ComponentCodeProfiles(ctx context.Context, org, component entity.ID, projects []entity.ID) ([]entity.ProfileEntry, error) {
	var out []entity.ProfileEntry
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		out = nil
		for _, project := range projects {
			profile, err := u.d.Repo.LatestProfileVersion(ctx, org, project)
			if errors.Is(err, entity.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			for _, e := range profile.Entries {
				if e.ComponentID == component && e.AnalysisKind == entity.AnalysisCode {
					out = append(out, e)
				}
			}
		}
		return nil
	})
	return out, err
}
