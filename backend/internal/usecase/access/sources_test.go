package access

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

var (
	_org      = entity.ID{0xa}
	_user     = entity.ID{0xb}
	_conn     = entity.ID{0xc}
	_instance = entity.ID{0xd}
)

type fakeTx struct{}

func (fakeTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func (fakeTx) InOrgTx(ctx context.Context, _ entity.ID, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeOrg struct {
	links    []entity.Link
	accounts map[entity.ID]entity.UserAccount
}

func (*fakeOrg) Profile(context.Context, entity.ID, entity.ID, time.Time) (entity.Profile, error) {
	return entity.Profile{}, nil
}

func (f *fakeOrg) ProjectAreas(context.Context, entity.ID, time.Time) ([]entity.Link, error) {
	return f.links, nil
}

func (f *fakeOrg) UserSourceAccount(_ context.Context, _, user, instance entity.ID) (entity.UserAccount, bool, error) {
	a, ok := f.accounts[instance]
	return a, ok && user == _user, nil
}

// fakeSources answers CheckAccess by the native ID of the item; block holds every check until it is closed.
type fakeSources struct {
	areas    []entity.AreaSource
	mu       sync.Mutex
	decision map[string]string
	err      error
	maxAge   *int
	block    chan struct{}
	calls    atomic.Int64
}

func (f *fakeSources) AreaSources(_ context.Context, _ entity.ID, ids []entity.ID) ([]entity.AreaSource, error) {
	var out []entity.AreaSource
	for _, a := range f.areas {
		for _, id := range ids {
			if a.AreaID == id {
				out = append(out, a)
			}
		}
	}
	return out, nil
}

func (f *fakeSources) CheckAccess(ctx context.Context, _, _ entity.ID, in entity.SourceAccessInput) (entity.SourceAccessResult, error) {
	f.calls.Add(1)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return entity.SourceAccessResult{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return entity.SourceAccessResult{}, f.err
	}
	d, ok := f.decision[in.Items[0].NativeID]
	if !ok {
		d = entity.SourceUnknown
	}
	return entity.SourceAccessResult{MaxAgeSeconds: f.maxAge, Decisions: []entity.SourceAccessDecision{{Decision: d}}}, nil
}

func (f *fakeSources) set(native, decision string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.decision[native] = decision
}

type fakeAuditor struct {
	mu     sync.Mutex
	events []entity.AuditEvent
}

func (f *fakeAuditor) RecordSeparately(_ context.Context, e entity.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

type fakeMetrics struct {
	hits, misses, failed atomic.Int64
}

func (m *fakeMetrics) CacheLookup(hit bool) {
	if hit {
		m.hits.Add(1)
		return
	}
	m.misses.Add(1)
}

func (m *fakeMetrics) CheckFailed(entity.ID) { m.failed.Add(1) }

type decideEnv struct {
	uc      *UseCase
	org     *fakeOrg
	sources *fakeSources
	audit   *fakeAuditor
	metrics *fakeMetrics
	clock   *clock.Manual
}

func newDecideEnv(edit ...func(*Deps)) *decideEnv {
	e := &decideEnv{
		org:     &fakeOrg{accounts: map[entity.ID]entity.UserAccount{_instance: {NativeID: "acc-1"}}},
		sources: &fakeSources{decision: map[string]string{}},
		audit:   &fakeAuditor{},
		metrics: &fakeMetrics{},
		clock:   clock.NewManual(time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)),
	}
	d := Deps{
		Tx: fakeTx{}, Profiles: e.org, Structure: e.org, Accounts: e.org, Sources: e.sources, Auditor: e.audit,
		Metrics: e.metrics, Clock: e.clock,
	}
	for _, fn := range edit {
		fn(&d)
	}
	e.uc = New(d)
	return e
}

// area adds a source area of project collected by the connection under the native ID.
func (e *decideEnv) area(project entity.ID, native string, edit ...func(*entity.AreaSource)) entity.ID {
	id := entity.ID{0xe0, byte(len(e.sources.areas) + 1)}
	src := entity.AreaSource{
		AreaID: id, ConnectionID: _conn, InstanceID: _instance, Kind: entity.ScopeTrackerProject, NativeScopeID: native,
		Active: true, ConnectionStatus: entity.ConnectionActive,
	}
	for _, fn := range edit {
		fn(&src)
	}
	e.sources.areas = append(e.sources.areas, src)
	if !project.IsZero() {
		e.org.links = append(e.org.links, entity.Link{From: project, To: id})
	}
	return id
}

func (e *decideEnv) decide(t *testing.T, scope entity.Scope, q entity.SourceQuery) entity.Scope {
	t.Helper()
	out, err := e.uc.Decide(t.Context(), entity.Subject{OrganizationID: _org, UserID: _user}, scope, q)
	require.NoError(t, err)
	return out
}

func roleScope(projects ...entity.ID) entity.Scope {
	return entity.GrantScope(entity.ScopeGrant{Projects: projects})
}

func TestDecide_ProjectsOpenInAtLeastOneSource(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	allowed, deniedP, plain, unchecked, mixed := entity.ID{1}, entity.ID{2}, entity.ID{3}, entity.ID{4}, entity.ID{5}
	e.area(allowed, "P-ALLOWED")
	e.area(deniedP, "P-DENIED")
	e.area(unchecked, "P-UNKNOWN")
	e.area(mixed, "M-DENIED")
	e.area(mixed, "M-ALLOWED")
	e.sources.set("P-ALLOWED", entity.SourceAllowed)
	e.sources.set("P-DENIED", entity.SourceDenied)
	e.sources.set("M-DENIED", entity.SourceDenied)
	e.sources.set("M-ALLOWED", entity.SourceAllowed)

	got := e.decide(t, roleScope(allowed, deniedP, plain, unchecked, mixed), entity.SourceQuery{Projects: true})

	assert.ElementsMatch(t, []entity.ID{allowed, plain, mixed}, got.Projects(), "a project without sources is seen by its role")
	assert.True(t, got.Limited())
	assert.ElementsMatch(t, []entity.AccessNote{
		{Code: entity.NoteRestricted}, {Code: entity.NoteCheckUnavailable, ConnectionID: _conn},
	}, got.Notes(), "the notes tell why, never how many")
	assert.Empty(t, e.audit.events, "a list hides projects without a refusal")
}

func TestDecide_ClosesAreaThatCannotBeConfirmed(t *testing.T) {
	t.Parallel()
	project := entity.ID{1}
	tests := []struct {
		name       string
		giveSource func(*entity.AreaSource)
		giveEnv    func(*decideEnv)
		decision   string
		wantOpen   bool
		wantNote   *entity.AccessNote
		wantReason string
		wantCalls  int64
	}{
		{name: "allowed", decision: entity.SourceAllowed, wantOpen: true, wantCalls: 1},
		{
			name: "denied", decision: entity.SourceDenied, wantCalls: 1,
			wantNote: &entity.AccessNote{Code: entity.NoteRestricted}, wantReason: ReasonSourceDenied,
		},
		{
			name: "unknown", decision: entity.SourceUnknown, wantCalls: 1,
			wantNote: &entity.AccessNote{Code: entity.NoteCheckUnavailable, ConnectionID: _conn}, wantReason: ReasonSourceUnavailable,
		},
		{
			name: "paused connection", decision: entity.SourceAllowed, wantCalls: 1,
			giveEnv: func(e *decideEnv) {
				e.sources.err = &entity.AdapterError{Class: entity.CallPaused, Code: entity.SourceRateLimited}
			},
			wantNote: &entity.AccessNote{Code: entity.NoteCheckUnavailable, ConnectionID: _conn}, wantReason: ReasonSourceUnavailable,
		},
		{
			name: "access lost", decision: entity.SourceAllowed,
			giveSource: func(s *entity.AreaSource) { s.ConnectionStatus = entity.ConnectionAccessLost },
			wantNote:   &entity.AccessNote{Code: entity.NoteAccessLost, ConnectionID: _conn}, wantReason: ReasonSourceAccessLost,
		},
		{
			name: "deleted connection", decision: entity.SourceAllowed,
			giveSource: func(s *entity.AreaSource) { s.ConnectionStatus = entity.ConnectionDeleted },
			wantNote:   &entity.AccessNote{Code: entity.NoteAccessLost, ConnectionID: _conn}, wantReason: ReasonSourceAccessLost,
		},
		{
			name: "scope removed from the connection", decision: entity.SourceAllowed,
			giveSource: func(s *entity.AreaSource) { s.Active = false },
			wantNote:   &entity.AccessNote{Code: entity.NoteAccessLost, ConnectionID: _conn}, wantReason: ReasonSourceAccessLost,
		},
		{
			name: "no account of the user in the instance", decision: entity.SourceAllowed,
			giveEnv:  func(e *decideEnv) { e.org.accounts = nil },
			wantNote: &entity.AccessNote{Code: entity.NoteRestricted}, wantReason: ReasonSourceDenied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newDecideEnv()
			var edit []func(*entity.AreaSource)
			if tt.giveSource != nil {
				edit = append(edit, tt.giveSource)
			}
			area := e.area(project, "A-1", edit...)
			e.sources.set("A-1", tt.decision)
			if tt.giveEnv != nil {
				tt.giveEnv(e)
			}

			got := e.decide(t, roleScope(project), entity.SourceQuery{Operation: "listTasks", Areas: []entity.ID{area}})

			assert.Equal(t, tt.wantOpen, got.Area(area))
			assert.Equal(t, tt.wantCalls, e.sources.calls.Load())
			if tt.wantNote == nil {
				assert.False(t, got.Limited())
				assert.Empty(t, e.audit.events)
				return
			}
			assert.Equal(t, []entity.AccessNote{*tt.wantNote}, got.Notes())
			require.Len(t, e.audit.events, 1)
			ev := e.audit.events[0]
			assert.Equal(t, entity.AuditDenied, ev.Outcome)
			assert.Equal(t, tt.wantReason, ev.ReasonCode)
			assert.Equal(t, "listTasks", ev.Operation)
			assert.Equal(t, area, ev.ObjectID)
			assert.Equal(t, _user, ev.ActorID)
		})
	}
}

func TestDecide_AreaOutsideTheRolesIsNotChecked(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	foreign := e.area(entity.ID{9}, "F-1")
	e.sources.set("F-1", entity.SourceAllowed)

	got := e.decide(t, roleScope(entity.ID{1}), entity.SourceQuery{Areas: []entity.ID{foreign}})

	assert.False(t, got.Area(foreign))
	assert.False(t, got.Limited(), "an area outside the roles looks like a missing one")
	assert.Zero(t, e.sources.calls.Load())
	require.Len(t, e.audit.events, 1)
	assert.Equal(t, ReasonOutsideScope, e.audit.events[0].ReasonCode)
}

func TestDecide_ImportedAreaHasNoSourceRights(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	imported := entity.ID{0xf1}

	got := e.decide(t, entity.GrantScope(entity.ScopeGrant{Whole: true}), entity.SourceQuery{Areas: []entity.ID{imported}})

	assert.True(t, got.Area(imported), "no connection collected the area, the role decides")
	assert.Zero(t, e.sources.calls.Load())
}

func TestDecide_CacheKeepsADecisionAtMost60Seconds(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceAllowed)
	q := entity.SourceQuery{Areas: []entity.ID{area}}

	assert.True(t, e.decide(t, roleScope(project), q).Area(area))
	e.sources.set("A-1", entity.SourceDenied)
	e.clock.Advance(59 * time.Second)
	assert.True(t, e.decide(t, roleScope(project), q).Area(area), "a repeated read within 60 s does not call CheckAccess")
	assert.Equal(t, int64(1), e.sources.calls.Load())
	assert.Equal(t, int64(1), e.metrics.hits.Load())

	e.clock.Advance(2 * time.Second)
	assert.False(t, e.decide(t, roleScope(project), q).Area(area), "after 60 s the source is asked again")
	assert.Equal(t, int64(2), e.sources.calls.Load())
	assert.Equal(t, int64(2), e.metrics.misses.Load())
}

func TestDecide_FreshnessOfTheModeShortensTheCache(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	maxAge := 10
	e.sources.maxAge = &maxAge
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceAllowed)
	q := entity.SourceQuery{Areas: []entity.ID{area}}

	e.decide(t, roleScope(project), q)
	e.clock.Advance(9 * time.Second)
	e.decide(t, roleScope(project), q)
	e.clock.Advance(2 * time.Second)
	e.decide(t, roleScope(project), q)

	assert.Equal(t, int64(2), e.sources.calls.Load())
}

func TestDecide_SlowCheckClosesTheAreaAndIsNotKept(t *testing.T) {
	t.Parallel()
	e := newDecideEnv(func(d *Deps) { d.CheckTimeout = 50 * time.Millisecond })
	e.sources.block = make(chan struct{})
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceAllowed)
	q := entity.SourceQuery{Areas: []entity.ID{area}}

	started := time.Now()
	got := e.decide(t, roleScope(project), q)

	assert.Less(t, time.Since(started), time.Second, "the read does not wait for the source beyond the budget")
	assert.False(t, got.Area(area))
	assert.Equal(t, []entity.AccessNote{{Code: entity.NoteCheckUnavailable, ConnectionID: _conn}}, got.Notes())
	assert.Equal(t, int64(1), e.metrics.failed.Load())
	close(e.sources.block)
	assert.True(t, e.decide(t, roleScope(project), q).Area(area), "a failed check is asked again on the next read")
	assert.Equal(t, int64(2), e.sources.calls.Load())
}

func TestDecide_SimultaneousMissesShareOneCheck(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	e.sources.block = make(chan struct{})
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceAllowed)
	q := entity.SourceQuery{Areas: []entity.ID{area}}

	const readers = 8
	results := make([]bool, readers)
	var wg sync.WaitGroup
	for i := range readers {
		wg.Go(func() {
			results[i] = e.decide(t, roleScope(project), q).Area(area)
		})
	}
	require.Eventually(t, func() bool { return e.metrics.misses.Load() == readers }, time.Second, time.Millisecond)
	close(e.sources.block)
	wg.Wait()

	assert.Equal(t, int64(1), e.sources.calls.Load())
	for _, open := range results {
		assert.True(t, open)
	}
}

func TestDecide_RestrictedObjectIsCheckedByItself(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceAllowed)
	e.sources.set("T-SECRET", entity.SourceDenied)
	e.sources.set("T-SHARED", entity.SourceAllowed)
	secret := entity.RestrictedObject{ID: entity.ID{0x51}, AreaID: area, Kind: entity.ObjectTask, NativeID: "T-SECRET"}
	shared := entity.RestrictedObject{ID: entity.ID{0x52}, AreaID: area, Kind: entity.ObjectTask, NativeID: "T-SHARED"}

	got := e.decide(t, roleScope(project), entity.SourceQuery{Areas: []entity.ID{area}, Objects: []entity.RestrictedObject{secret, shared}})

	assert.True(t, got.Area(area))
	assert.Equal(t, []entity.ID{shared.ID}, got.Objects())
	assert.Equal(t, []entity.AccessNote{{Code: entity.NoteRestricted}}, got.Notes())
	require.Len(t, e.audit.events, 1)
	assert.Equal(t, secret.ID, e.audit.events[0].ObjectID)
	assert.Equal(t, int64(3), e.sources.calls.Load())
}

func TestDecide_ObjectOfAClosedAreaIsClosed(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	project := entity.ID{1}
	area := e.area(project, "A-1")
	e.sources.set("A-1", entity.SourceDenied)
	e.sources.set("T-1", entity.SourceAllowed)
	obj := entity.RestrictedObject{ID: entity.ID{0x51}, AreaID: area, Kind: entity.ObjectTask, NativeID: "T-1"}

	got := e.decide(t, roleScope(project), entity.SourceQuery{Objects: []entity.RestrictedObject{obj}})

	assert.Empty(t, got.Objects())
	assert.Equal(t, int64(1), e.sources.calls.Load(), "the object of a closed area is not asked")
}

func TestDecide_CacheEvictsTheOldestDecisions(t *testing.T) {
	t.Parallel()
	e := newDecideEnv(func(d *Deps) { d.CacheSize = 2 })
	project := entity.ID{1}
	natives := []string{"A-1", "A-2", "A-3"}
	areas := make([]entity.ID, 0, len(natives))
	for _, native := range natives {
		areas = append(areas, e.area(project, native))
		e.sources.set(native, entity.SourceAllowed)
	}
	for _, a := range areas {
		e.decide(t, roleScope(project), entity.SourceQuery{Areas: []entity.ID{a}})
	}
	require.Equal(t, int64(3), e.sources.calls.Load())
	assert.Equal(t, 2, e.uc.checks.cache.Len(), "the cache holds no more than its limit")

	e.decide(t, roleScope(project), entity.SourceQuery{Areas: []entity.ID{areas[2]}})
	assert.Equal(t, int64(3), e.sources.calls.Load(), "the newest decision is kept")
	e.decide(t, roleScope(project), entity.SourceQuery{Areas: []entity.ID{areas[0]}})
	assert.Equal(t, int64(4), e.sources.calls.Load(), "the oldest decision was evicted")
	assert.Equal(t, 2, e.uc.checks.cache.Len())
}

func TestDecide_UploadTokenKeepsItsScope(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	scope := roleScope(entity.ID{1})

	got, err := e.uc.Decide(t.Context(), entity.Subject{OrganizationID: _org, UploadTokenID: entity.ID{2}, ProjectID: entity.ID{1}}, scope,
		entity.SourceQuery{Projects: true})

	require.NoError(t, err)
	assert.Equal(t, scope, got)
	assert.Zero(t, e.sources.calls.Load())
}

func TestDecide_ObjectsAddToTheScopeOfTheirAreas(t *testing.T) {
	t.Parallel()
	e := newDecideEnv()
	project := entity.ID{1}
	open := e.area(project, "A-1")
	closed := e.area(project, "A-2")
	e.sources.set("A-1", entity.SourceAllowed)
	e.sources.set("A-2", entity.SourceDenied)
	e.sources.set("T-1", entity.SourceAllowed)
	areas := e.decide(t, roleScope(project), entity.SourceQuery{Areas: []entity.ID{open, closed}})
	require.Len(t, e.audit.events, 1)

	obj := entity.RestrictedObject{ID: entity.ID{0x51}, AreaID: open, Kind: entity.ObjectTask, NativeID: "T-1"}
	got := e.decide(t, areas, entity.SourceQuery{Objects: []entity.RestrictedObject{obj}})

	assert.True(t, got.Area(open), "the areas opened before are kept")
	assert.False(t, got.Area(closed))
	assert.Equal(t, []entity.ID{obj.ID}, got.Objects())
	assert.Equal(t, []entity.AccessNote{{Code: entity.NoteRestricted}}, got.Notes(), "the note of the first step is kept")
	assert.Len(t, e.audit.events, 1, "a refused area is audited once per read")
}
