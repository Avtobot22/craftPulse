package normalize

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Links is the link module, which owns links and decisions on candidates (TZ 10.3). normalize keeps the objects
// at their ends and checks them here: a check inside link through normalize or access would close a cycle of
// modules (TZ 10.3, rule 7).
type Links interface {
	Edge(ctx context.Context, orgID, id entity.ID) (entity.LinkEdge, error)
	Decide(ctx context.Context, orgID, actor, linkID entity.ID, d entity.LinkDecision, key entity.ID) (entity.LinkEdge, error)
}

// Rights is access: the rights of the user in the sources narrow the scope of the roles (TZ 13.5).
type Rights interface {
	Decide(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.SourceQuery) (entity.Scope, error)
}

const _decideLink = "decideLink"

// DecideLink confirms or rejects a candidate for a user who sees both of its ends, including an end with its own
// restriction in the source; otherwise the link is not found (UC-19, TZ 7.7).
func (u *UseCase) DecideLink(ctx context.Context, s entity.Subject, scope entity.Scope, linkID entity.ID, d entity.LinkDecision,
	key entity.ID,
) (entity.LinkResult, error) {
	if d != entity.LinkConfirm && d != entity.LinkReject {
		return entity.LinkResult{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/decision", Code: entity.FieldNotAllowedValue}}}
	}
	org := s.OrganizationID
	e, err := u.d.Links.Edge(ctx, org, linkID)
	if err != nil {
		return entity.LinkResult{}, err
	}
	ends, err := u.visibleEnds(ctx, s, scope, &e)
	if err != nil {
		return entity.LinkResult{}, err
	}
	var out entity.LinkEdge
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Links.Decide(ctx, org, s.UserID, linkID, d, key)
		return err
	})
	if err != nil {
		return entity.LinkResult{}, err
	}
	return entity.LinkResult{Edge: out, To: ends[out.To]}, nil
}

func (u *UseCase) visibleEnds(ctx context.Context, s entity.Subject, scope entity.Scope, e *entity.LinkEdge) (map[entity.ID]entity.LinkEnd, error) {
	var ends []entity.LinkEnd
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		ends, err = u.d.Repo.LinkEnds(ctx, s.OrganizationID, []entity.ID{e.From, e.To})
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make(map[entity.ID]entity.LinkEnd, len(ends))
	areas := make([]entity.ID, 0, len(ends))
	var restricted []entity.RestrictedObject
	for i := range ends {
		end := &ends[i]
		if end.AreaID.IsZero() {
			return nil, entity.ErrNotFound
		}
		out[end.ID] = *end
		areas = append(areas, end.AreaID)
		if end.Restricted {
			restricted = append(restricted, entity.RestrictedObject{ID: end.ID, AreaID: end.AreaID, Kind: end.Kind, NativeNS: end.NativeNS, NativeID: end.NativeID})
		}
	}
	if len(out) != 2 {
		return nil, entity.ErrNotFound
	}
	scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _decideLink, Areas: areas})
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(areas, func(a entity.ID) bool { return !scope.Area(a) }) && len(restricted) > 0 {
		scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _decideLink, Objects: restricted})
		if err != nil {
			return nil, err
		}
	}
	for _, a := range areas {
		if !scope.Area(a) {
			return nil, entity.ErrNotFound
		}
	}
	for i := range restricted {
		if !slices.Contains(scope.Objects(), restricted[i].ID) {
			return nil, entity.ErrNotFound
		}
	}
	return out, nil
}
