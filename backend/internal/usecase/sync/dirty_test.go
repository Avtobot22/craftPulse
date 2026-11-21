package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type dirtySource struct {
	Connect
	pages   []envelope.Selection
	fetches []entity.FetchInput
}

func (s *dirtySource) Fetch(_ context.Context, _, _ entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	index := len(s.fetches)
	s.fetches = append(s.fetches, in)
	body, err := json.Marshal(envelope.Manifest{Selection: s.pages[index]})
	return io.NopCloser(bytes.NewReader(body)), err
}

type dirtyIntake struct{ Ingest }

func (dirtyIntake) PreparePage(_ context.Context, _, _, _ entity.ID, r io.Reader) (*entity.PreparedPackage, error) {
	body, err := io.ReadAll(r)
	return &entity.PreparedPackage{
		Receipt: entity.Receipt{Status: entity.ReceiptAccepted, Manifest: body},
	}, err
}

type dirtyLease struct {
	repo.SyncRepo
	leases int
}

func (l *dirtyLease) AcquireLease(context.Context, entity.ID, entity.ID, string, time.Time, time.Time) (bool, error) {
	l.leases++
	return true, nil
}

func TestDirtyReadFollowsTargetPages(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	next := &envelope.Cursor{NativeId: "10005/100", ModifiedAt: at}
	source := &dirtySource{pages: []envelope.Selection{
		{End: true}, {NextCursor: next}, {End: true},
	}}
	lease := &dirtyLease{}
	u := New(Deps{Connect: source, Ingest: dirtyIntake{}, Repo: lease, Tx: collectionTx{}, Clock: clock.NewManual(at)})
	key := entity.ObjectKey{ObjectKind: "task", NativeID: "10005"}
	group := dirtyGroup{
		scope:    &entity.ConnectionScope{Kind: entity.ScopeTrackerProject, NativeScopeID: "10000"},
		dataKind: "tasks", keys: []entity.DirtyKey{{Object: key}},
	}
	pages, f, err := u.readGroup(t.Context(), entity.SyncDirtyJob{}, "dirty", group)
	require.NoError(t, err)
	require.Nil(t, f)
	require.Len(t, pages, 3)
	require.Len(t, source.fetches, 3)
	assert.Equal(t, "tasks", source.fetches[0].SchemaID)
	assert.Equal(t, "task_events", source.fetches[1].SchemaID)
	assert.Nil(t, source.fetches[1].Cursor)
	assert.Equal(t, sourceCursor(next), source.fetches[2].Cursor)
	assert.Equal(t, []entity.ObjectKey{key}, source.fetches[2].ObjectKeys)
	assert.Equal(t, 3, lease.leases)
	for _, page := range pages {
		assert.Equal(t, entity.ViaReconcile, page.ObservedVia)
	}
}
