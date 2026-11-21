package sync

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) recountActivity(ctx context.Context, org, conn, corr entity.ID) error {
	return u.d.Queue.Enqueue(ctx, entity.CalcRecountJob{OrganizationID: org, ConnectionID: conn, CorrelationID: corr}, entity.QueueBulk)
}

func (u *UseCase) ActivityConnectionAreas(ctx context.Context, org, conn entity.ID) ([]entity.ID, error) {
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return nil, err
	}
	var areas []entity.ID
	for i := range scopes {
		scope := &scopes[i]
		if scope.Active && scope.AreaID != nil && !slices.Contains(areas, *scope.AreaID) {
			areas = append(areas, *scope.AreaID)
		}
	}
	slices.SortFunc(areas, compareActivityIDs)
	return areas, nil
}

func compareActivityIDs(a, b entity.ID) int { return slices.Compare(a[:], b[:]) }

// ActivityCollection takes the same state and latest reconciliation as Status.
func (u *UseCase) ActivityCollection(ctx context.Context, org entity.ID, areas []entity.ID, conn entity.ID) ([]entity.AreaCollection, error) {
	connections := make(map[entity.ID][]entity.ID)
	if !conn.IsZero() {
		connections[conn] = areas
	} else if len(areas) > 0 {
		sources, err := u.d.Connect.AreaSources(ctx, org, areas)
		if err != nil {
			return nil, err
		}
		for i := range sources {
			src := &sources[i]
			if src.Active && src.ConnectionStatus != entity.ConnectionDeleted && !slices.Contains(connections[src.ConnectionID], src.AreaID) {
				connections[src.ConnectionID] = append(connections[src.ConnectionID], src.AreaID)
			}
		}
	}
	var out []entity.AreaCollection
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		for connection, scopeAreas := range connections {
			items, err := u.activityStates(ctx, org, connection)
			if err != nil {
				return err
			}
			if len(scopeAreas) == 0 {
				scopeAreas = []entity.ID{{}}
			}
			for _, area := range scopeAreas {
				for i := range items {
					item := items[i]
					item.AreaID = area
					out = append(out, item)
				}
			}
		}
		return nil
	})
	slices.SortFunc(out, func(a, b entity.AreaCollection) int {
		if a.ConnectionID != b.ConnectionID {
			return compareActivityIDs(a.ConnectionID, b.ConnectionID)
		}
		if a.AreaID != b.AreaID {
			return compareActivityIDs(a.AreaID, b.AreaID)
		}
		return cmp.Compare(a.DataKind, b.DataKind)
	})
	return out, err
}

func (u *UseCase) activityStates(ctx context.Context, org, conn entity.ID) ([]entity.AreaCollection, error) {
	states, err := u.d.Repo.States(ctx, org, conn)
	if err != nil {
		return nil, err
	}
	overall, found, err := u.d.Repo.LastReconcile(ctx, org, conn, "")
	if err != nil {
		return nil, err
	}
	base := entity.AreaCollection{ConnectionID: conn}
	if found {
		base.ConnectionReconcile = &overall
	}
	if len(states) == 0 {
		return []entity.AreaCollection{base}, nil
	}
	out := make([]entity.AreaCollection, 0, len(states))
	for i := range states {
		st := &states[i]
		item := base
		item.DataKind, item.LastErrorCode = st.DataKind, st.LastErrorCode
		item.Completeness, item.Phase = st.Completeness, st.Phase
		item.Watermark, item.LastSuccessAt, item.HistoryFrom = st.Watermark, st.LastSuccessAt, st.HistoryFrom
		last, found, err := u.d.Repo.LastReconcile(ctx, org, conn, st.DataKind)
		if err != nil {
			return nil, err
		}
		if found {
			item.Reconcile = &last
		}
		out = append(out, item)
	}
	return out, nil
}

func (u *UseCase) ActivityHistory(ctx context.Context, org entity.ID, areas []entity.ID, conn entity.ID, at time.Time) ([]entity.ActivityHistoryEvidence, error) {
	if len(areas) == 0 {
		return nil, nil
	}
	sources, err := u.d.Connect.AreaSources(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	var active []entity.AreaSource
	byScope := make(map[entity.ID]entity.AreaSource)
	for i := range sources {
		source := &sources[i]
		if source.Active && source.ConnectionStatus == entity.ConnectionActive && (conn.IsZero() || conn == source.ConnectionID) {
			active = append(active, *source)
			byScope[source.ScopeID] = *source
		}
	}
	if len(active) == 0 {
		return nil, nil
	}
	var (
		checks           []entity.ActivityCheckResult
		cuts, recoveries []entity.ActivityHistoryNotice
	)
	ids := slices.Collect(maps.Keys(byScope))
	var complete map[entity.ID]bool
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if checks, err = u.d.Repo.ActivityChecks(ctx, org, active, at); err != nil {
			return err
		}
		if recoveries, err = u.d.Repo.ActivityRecoveries(ctx, org, ids, at); err != nil {
			return err
		}
		if cuts, err = u.d.Ingest.ActivityTruncations(ctx, org, ids, at); err != nil {
			return err
		}
		receipts := make(map[entity.ID]bool)
		for i := range checks {
			check := &checks[i]
			if !check.PaginationReceipt.IsZero() {
				receipts[check.PaginationReceipt] = true
			}
		}
		for _, recovery := range recoveries {
			receipts[recovery.ReceiptID] = true
		}
		complete, err = u.d.Ingest.ActivityCompleteReceipts(ctx, org, slices.Collect(maps.Keys(receipts)))
		return err
	})
	if err != nil {
		return nil, err
	}
	type scopeObject struct {
		scope entity.ID
		key   entity.ObjectKey
	}
	objects := make(map[scopeObject]entity.ActivityHistoryEvidence)
	for i := range checks {
		check := &checks[i]
		if !complete[check.PaginationReceipt] {
			check.PaginationReceipt = entity.ID{}
		}
		objects[scopeObject{check.ScopeID, check.Key}] = entity.ActivityHistoryEvidence{Source: byScope[check.ScopeID], Check: check, Key: check.Key}
	}
	for _, cut := range cuts {
		k := scopeObject{cut.ScopeID, cut.Key}
		evidence := objects[k]
		evidence.Source, evidence.Key, evidence.Cut = byScope[k.scope], k.key, &cut
		objects[k] = evidence
	}
	for _, recovery := range recoveries {
		k := scopeObject{recovery.ScopeID, recovery.Key}
		evidence, ok := objects[k]
		if ok && complete[recovery.ReceiptID] && (evidence.Cut == nil || recovery.ObservedAt.After(evidence.Cut.ObservedAt) ||
			recovery.ObservedAt.Equal(evidence.Cut.ObservedAt) && compareActivityIDs(recovery.ReceiptID, evidence.Cut.ReceiptID) > 0) {
			evidence.Recovery = &recovery
			objects[k] = evidence
		}
	}
	return slices.Collect(maps.Values(objects)), nil
}

func (u *UseCase) ActivityReconciliations(ctx context.Context, org entity.ID, connections []entity.ID, from, at time.Time) ([]entity.ReconcileRun, error) {
	var out []entity.ReconcileRun
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ActivityReconciliations(ctx, org, connections, from, at)
		return err
	})
	return out, err
}

func (u *UseCase) ActivityReceiptConnections(ctx context.Context, org entity.ID, receipts []entity.ID) (map[entity.ID]entity.ID, error) {
	return u.d.Ingest.ActivityReceiptConnections(ctx, org, receipts)
}
