package sync

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _dataTasks = "tasks"

// AreaCollection is the state of the collection of tasks of the active connections that collect the areas, with
// their last reconciliation and phase, for the completeness of calc (T39, T40, T42) and the initial load of
// history. An area only imported has none.
func (u *UseCase) AreaCollection(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaCollection, error) {
	return u.collectionFor(ctx, orgID, areaIDs, _dataTasks)
}

// CICollection reads CI collection and reconciliation, independently of tracker tasks.
func (u *UseCase) CICollection(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaCollection, error) {
	return u.collectionFor(ctx, orgID, areaIDs, "ci")
}

// MRCollection reads merge request history independently of task and CI collection.
func (u *UseCase) MRCollection(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaCollection, error) {
	return u.collectionFor(ctx, orgID, areaIDs, "merge_requests")
}

func (u *UseCase) collectionFor(ctx context.Context, orgID entity.ID, areaIDs []entity.ID, dataKind string) ([]entity.AreaCollection, error) {
	sources, err := u.d.Connect.AreaSources(ctx, orgID, areaIDs)
	if err != nil || len(sources) == 0 {
		return nil, err
	}
	return u.collectionOf(ctx, orgID, sources, dataKind)
}

func (u *UseCase) collectionOf(ctx context.Context, orgID entity.ID, sources []entity.AreaSource, dataKind string) ([]entity.AreaCollection, error) {
	var ciScopes map[entity.ID][]entity.ConnectionScope
	if dataKind == "ci" {
		ciScopes = make(map[entity.ID][]entity.ConnectionScope)
		for i := range sources {
			src := &sources[i]
			if !src.Active || src.Kind != entity.ScopeRepository && src.Kind != entity.ScopeCIProject {
				continue
			}
			if _, ok := ciScopes[src.ConnectionID]; ok {
				continue
			}
			var err error
			ciScopes[src.ConnectionID], err = u.d.Connect.Scopes(ctx, orgID, src.ConnectionID)
			if err != nil {
				return nil, err
			}
		}
	}
	var out []entity.AreaCollection
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		for i := range sources {
			src := &sources[i]
			if !src.Active || (dataKind == "merge_requests" && src.Kind != entity.ScopeRepository) {
				continue
			}
			if dataKind == "ci" && src.Kind != entity.ScopeRepository && src.Kind != entity.ScopeCIProject {
				continue
			}
			item := entity.AreaCollection{AreaID: src.AreaID, ConnectionID: src.ConnectionID, InstanceID: src.InstanceID, Completeness: completeUnknown}
			st, ok, err := u.d.Repo.State(ctx, orgID, src.ConnectionID, dataKind)
			if err != nil {
				return err
			}
			if ok {
				item.Completeness, item.Watermark, item.LastSuccessAt, item.Phase = st.Completeness, st.Watermark, st.LastSuccessAt, st.Phase
				item.HistoryFrom = st.HistoryFrom
			}
			if dataKind == "ci" {
				item.HistoryFrom = nil
				matched := false
				scopes := ciScopes[src.ConnectionID]
				for j := range scopes {
					scope := &scopes[j]
					if scope.Active && scope.AreaID != nil && *scope.AreaID == src.AreaID {
						item.HistoryFrom = scope.HistoryFrom
						matched = true
					}
				}
				if !matched {
					item.Completeness = completeUnknown
				}
			}
			last, found, err := u.d.Repo.LastReconcile(ctx, orgID, src.ConnectionID, dataKind)
			if err != nil {
				return err
			}
			if found {
				item.Reconcile = &last
			}
			out = append(out, item)
		}
		return nil
	})
	return out, err
}

// TimelineCollection includes active connections before their first successful collection.
func (u *UseCase) TimelineCollection(ctx context.Context, org entity.ID, areas []entity.ID) ([]entity.AreaCollection, error) {
	sources, err := u.d.Connect.AreaSources(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	var out []entity.AreaCollection
	for _, kind := range []string{_dataTasks, "merge_requests", "ci"} {
		var selected []entity.AreaSource
		for i := range sources {
			src := &sources[i]
			switch src.Kind {
			case entity.ScopeTrackerProject:
				if kind != _dataTasks {
					continue
				}
			case entity.ScopeRepository:
				if kind == _dataTasks {
					continue
				}
			case entity.ScopeCIProject:
				if kind != "ci" {
					continue
				}
			default:
				if kind != _dataTasks {
					continue
				}
			}
			selected = append(selected, *src)
		}
		if len(selected) == 0 {
			continue
		}
		items, err := u.collectionOf(ctx, org, selected, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}
