package app

import (
	"crypto/sha256"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
)

// TestRetention_PurgesWhatIsPastItsTerm is the retention table (TZ 12.9): expired login attempts and sessions, probes
// finished a day ago that no connection activated and operations completed a day ago go; the rest stays.
func TestRetention_PurgesWhatIsPastItsTerm(t *testing.T) {
	t.Parallel()
	e := newConnectionsEnv(t, testadapter.Config{})
	id := e.activeConnection(t)
	var activated uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT activated_probe_id FROM connect.connection WHERE id = $1", id).Scan(&activated))
	stale := e.probe(t, id).ID
	rec := e.syncRun(t, e.admin, id, "resume")
	require.Equal(t, http.StatusAccepted, rec.Code)
	_, done := e.operation(t, e.admin, rec.Header().Get("Location"))
	e.settle(t)
	_, done = e.operation(t, e.admin, "/api/v1/operations/"+done.ID.String())
	require.Equal(t, "succeeded", done.Status)

	now := e.clock.Now()
	day := 25 * time.Hour
	for _, q := range []string{
		"UPDATE connect.probe SET started_at = started_at - $2::interval, finished_at = finished_at - $2::interval WHERE id = ANY($1)",
		"UPDATE view.operation SET created_at = created_at - $2::interval, updated_at = updated_at - $2::interval, completed_at = completed_at - $2::interval WHERE id = ANY($1)",
	} {
		_, err := e.owner.Exec(t.Context(), q, []uuid.UUID{stale, activated, done.ID}, day.String())
		require.NoError(t, err)
	}
	var user uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT user_id FROM auth.session WHERE organization_id = $1 LIMIT 1", e.orgID).Scan(&user))
	hash := func(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
	for i, expires := range []time.Time{now.Add(-time.Minute), now.Add(10 * time.Minute)} {
		_, err := e.owner.Exec(t.Context(), `INSERT INTO auth.login_attempt (cookie_hash, organization_id, state, nonce, pkce_verifier, issuer,
			return_path, created_at, expires_at) VALUES ($1, $2, 's', 'n', 'v', 'https://idp', '/', $3, $4)`,
			hash("attempt"+string(rune('a'+i))), e.orgID, expires.Add(-10*time.Minute), expires)
		require.NoError(t, err)
	}
	_, err := e.owner.Exec(t.Context(), `INSERT INTO auth.session (organization_id, token_hash, user_id, created_at, last_activity_at, expires_at)
		VALUES ($1, $2, $3, $4, $4, $5)`, e.orgID, hash("old session"), user, now.Add(-13*time.Hour), now.Add(-time.Hour))
	require.NoError(t, err)
	cut := "'" + now.Format(time.RFC3339Nano) + "'"
	alive := count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.session WHERE expires_at > "+cut)
	require.Positive(t, alive)

	conn, err := newConnect(e.app, e.keyring, nil, e.clock)
	require.NoError(t, err)
	purges := newRetention(e.app, conn, newView(e.app, viewModules{}, e.clock), e.clock)
	require.NoError(t, purges.Purge(t.Context()))
	require.NoError(t, purges.Purge(t.Context()), "a repeat finds nothing more")

	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.login_attempt"), "the attempt that has not expired stays")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.login_attempt WHERE expires_at <= "+cut))
	assert.Equal(t, alive, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.session"), "only the expired sessions go")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM connect.probe WHERE id = '"+stale.String()+"'"), "a stale probe goes")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM connect.probe WHERE id = '"+activated.String()+"'"),
		"the activated probe stays")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM view.operation WHERE id = '"+done.ID.String()+"'"), "a completed operation goes")
}
