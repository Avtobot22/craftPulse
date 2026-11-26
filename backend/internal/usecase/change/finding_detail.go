package change

import (
	"context"
	"errors"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _getFinding = "getFinding"

// GetFinding reads a canonical finding only after the caller can see its area and finding class.
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

	var out entity.FindingDetail
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.FindingObservation(ctx, org, id)
		if err != nil {
			return err
		}
		out.FindingIdentity = identity
		out.State = entity.FindingNewlyObserved
		if out.ComponentID != nil && out.Report.SHA != "" {
			state, err := u.CurrentFindingState(ctx, org, *out.ComponentID, id, out.ReportKind)
			if err != nil && !errors.Is(err, entity.ErrNotFound) {
				return err
			}
			if err == nil {
				out.State = state
			}
		}
		return u.findingExceptions(ctx, org, &out)
	})
	return out, err
}

func (u *UseCase) findingExceptions(ctx context.Context, org entity.ID, out *entity.FindingDetail) error {
	seq, _, err := u.d.Tenants.Clock(ctx, org)
	if err != nil {
		return err
	}
	list, err := u.d.Decisions.ActiveAt(ctx, org, out.AreaID, seq, u.d.Clock.Now().UTC())
	if err != nil {
		return err
	}
	coords := entity.FindingCoordinates{
		FindingAccess: entity.FindingAccess{AreaID: out.AreaID, Kind: out.Kind},
		FindingID:     out.ID, RuleID: out.Observation.RuleID, Path: out.Observation.Path, PackagePURL: out.Observation.PackagePURL,
	}
	if out.ComponentID != nil {
		coords.ComponentID = *out.ComponentID
	}
	var matched *entity.ExceptionVersion
	for i := range list {
		v := &list[i]
		if !v.Matches(coords) {
			continue
		}
		out.Exceptions = append(out.Exceptions, *v)
		if matched == nil || (matched.Scope.FindingID.IsZero() && !v.Scope.FindingID.IsZero()) {
			matched = v
		}
	}
	if matched != nil && out.State != entity.FindingResolved {
		out.State = matched.Type
	}
	return nil
}
