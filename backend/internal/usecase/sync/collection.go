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
	sources, err := u.d.Connect.AreaSources(ctx, orgID, areaIDs)
	if err != nil || len(sources) == 0 {
		return nil, err
	}
	var out []entity.AreaCollection
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		for i := range sources {
			src := &sources[i]
			if !src.Active {
				continue
			}
			item := entity.AreaCollection{AreaID: src.AreaID, ConnectionID: src.ConnectionID, Completeness: completeUnknown}
			st, ok, err := u.d.Repo.State(ctx, orgID, src.ConnectionID, _dataTasks)
			if err != nil {
				return err
			}
			if ok {
				item.Completeness, item.Watermark, item.LastSuccessAt, item.Phase = st.Completeness, st.Watermark, st.LastSuccessAt, st.Phase
			}
			last, found, err := u.d.Repo.LastReconcile(ctx, orgID, src.ConnectionID, _dataTasks)
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
