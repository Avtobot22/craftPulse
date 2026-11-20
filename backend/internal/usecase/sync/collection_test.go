package sync

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
)

type collectionConnect struct {
	Connect
	sources []entity.AreaSource
	scopes  []entity.ConnectionScope
}

func (c collectionConnect) AreaSources(context.Context, entity.ID, []entity.ID) ([]entity.AreaSource, error) {
	return c.sources, nil
}

func (c collectionConnect) Scopes(context.Context, entity.ID, entity.ID) ([]entity.ConnectionScope, error) {
	return c.scopes, nil
}

type collectionRepo struct {
	repo.SyncRepo
	kinds []string
	state entity.SyncState
	run   entity.ReconcileRun
}

func (r *collectionRepo) State(_ context.Context, _, _ entity.ID, kind string) (entity.SyncState, bool, error) {
	r.kinds = append(r.kinds, kind)
	return r.state, true, nil
}

func (r *collectionRepo) LastReconcile(_ context.Context, _, _ entity.ID, kind string) (entity.ReconcileRun, bool, error) {
	r.kinds = append(r.kinds, kind)
	return r.run, true, nil
}

type collectionTx struct{ repo.Transactor }

func (collectionTx) InOrgTx(ctx context.Context, _ entity.ID, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestCICollection_UsesCIStateAndScopeHistory(t *testing.T) {
	t.Parallel()
	area, conn, org := entity.ID(uuid.New()), entity.ID(uuid.New()), entity.ID(uuid.New())
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	older := start.AddDate(0, -1, 0)
	r := &collectionRepo{state: entity.SyncState{Completeness: completePartial, HistoryFrom: &older}, run: entity.ReconcileRun{DataKind: "ci", Outcome: entity.ReconcileIncomplete}}
	connect := collectionConnect{
		sources: []entity.AreaSource{{AreaID: area, ConnectionID: conn, Kind: entity.ScopeRepository, Active: true}},
		scopes:  []entity.ConnectionScope{{AreaID: &area, ConnectionID: conn, HistoryFrom: &start, Active: true}},
	}
	u := New(Deps{Repo: r, Connect: connect, Tx: collectionTx{}})
	out, err := u.CICollection(t.Context(), org, []entity.ID{area})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, []string{"ci", "ci"}, r.kinds)
	assert.Equal(t, &start, out[0].HistoryFrom)
	assert.Equal(t, completePartial, out[0].Completeness)
	assert.Equal(t, entity.ReconcileIncomplete, out[0].Reconcile.Outcome)
	areas, err := u.reconciledAreas(t.Context(), org, conn, "ci")
	require.NoError(t, err)
	assert.Equal(t, []entity.ID{area}, areas, "CI reconciliation must recount CI series even when no normalized fact changed")

	connect.sources[0].Kind = entity.ScopeTrackerProject
	u = New(Deps{Repo: r, Connect: connect, Tx: collectionTx{}})
	out, err = u.CICollection(t.Context(), org, []entity.ID{area})
	require.NoError(t, err)
	assert.Empty(t, out, "tracker collection cannot establish CI completeness")
}
