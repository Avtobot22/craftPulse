// Package sync plans the collection of a connection page by page: one job reads one page, and the page,
// the new checkpoint, the next job and the end of the current one commit in one transaction (TZ 10.5.1,
// 10.6). One attempt of collection runs per connection under a lease (NFR-006).
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_leaseFor    = 120 * time.Second
	_leaseWait   = 2 * time.Second
	_defaultPage = 100
	// _overlap reads a new round back from the cursor, so objects changed while the last round read are not lost.
	_overlap      = 10 * time.Minute
	_defaultPause = time.Minute
	_preparePoll  = 5 * time.Second
)

// _backoff are the pauses after failures in a row (TZ 10.6); the last one repeats.
var _backoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}

// _schemas are the schemas of the envelope a data kind of the contract is read with, in order.
var _schemas = map[string][]string{
	"tasks":          {"tasks", "task_events"},
	"accounts":       {"accounts"},
	"merge_requests": {"merge_requests"},
	"ci":             {"ci_runs"},
	"reports":        {"findings", "junit", "coverage"},
	"documents":      {"documents"},
}

// Connect is the connect module: the scopes of a connection and the one way to call its adapter.
type Connect interface {
	Scopes(ctx context.Context, orgID, connectionID entity.ID) ([]entity.ConnectionScope, error)
	Fetch(ctx context.Context, orgID, connectionID entity.ID, in entity.FetchInput) (io.ReadCloser, error)
}

// Ingest is the intake of a page: cleaned before the transaction, written in it.
type Ingest interface {
	PreparePage(ctx context.Context, orgID, connectionID, scopeID entity.ID, page io.Reader) (*entity.PreparedPackage, error)
	RecordPage(ctx context.Context, p *entity.PreparedPackage) (entity.Receipt, error)
}

type Metrics interface {
	CollectionFailed(class string)
}

type Deps struct {
	Repo    repo.SyncRepo
	Connect Connect
	Ingest  Ingest
	Queue   repo.JobQueue
	Tx      repo.Transactor
	Metrics Metrics
	Clock   clock.Clock
	// Rand returns a number in [0, 1) for the jitter of pauses; tests set their own.
	Rand func() float64
	// PageSize is the limit of one Fetch; zero is 100.
	PageSize int
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	if d.PageSize == 0 {
		d.PageSize = _defaultPage
	}
	return &UseCase{d: d}
}

// Complete ends the current job in the transaction of its result; nil leaves the end to the queue.
type Complete = func(ctx context.Context) error

// step is one scope read with one schema; key names it in the cursor.
type step struct {
	scope  entity.ConnectionScope
	schema string
}

func (s step) key() string { return s.scope.ID.String() + "/" + s.schema }

// progress is the cursor of a data kind: the step being read, the cursor of every step and whether the
// next run starts a new round.
type progress struct {
	Current  string                          `json:"current"`
	Cursors  map[string]*entity.SourceCursor `json:"cursors"`
	NewRound bool                            `json:"new_round,omitempty"`
}

// RunPage reads one page of a data kind. holder names the run that takes the lease of the connection.
func (u *UseCase) RunPage(ctx context.Context, job entity.SyncPageJob, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.lease(ctx, org, conn, holder); err != nil {
		return err
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	steps := steps(scopes, _schemas[job.DataKind])
	var st entity.SyncState
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var found bool
		st, found, err = u.d.Repo.State(ctx, org, conn, job.DataKind)
		if !found {
			st = entity.SyncState{ConnectionID: conn, DataKind: job.DataKind, Phase: entity.SyncBackfill, Completeness: "unknown", HistoryFrom: earliest(scopes)}
		}
		return err
	})
	if err != nil {
		return err
	}
	if len(steps) == 0 {
		return u.finish(ctx, org, conn, holder, complete)
	}
	p, err := decode(st.Cursor)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(steps, func(s step) bool { return s.key() == p.Current })
	if i < 0 {
		i = 0
	}
	cur := steps[i]
	cursor := p.Cursors[cur.key()]
	if p.NewRound && cursor != nil {
		back := entity.SourceCursor{ModifiedAt: cursor.ModifiedAt.Add(-_overlap)}
		cursor = &back
	}
	in := entity.FetchInput{
		SchemaID: cur.schema, Scope: entity.ScopeRef{Kind: cur.scope.Kind, NativeScopeID: cur.scope.NativeScopeID},
		Cursor: cursor, Period: entity.Period{From: cur.scope.HistoryFrom}, Limit: u.d.PageSize,
	}
	page, sel, failure, err := u.fetch(ctx, org, conn, cur.scope.ID, in)
	if err != nil {
		return err
	}
	if page != nil {
		page.ObservedVia = entity.ViaBackfill
		if st.Phase == entity.SyncIncremental {
			page.ObservedVia = entity.ViaIncremental
		}
	}
	now := u.d.Clock.Now()
	st.LastAttemptAt = &now
	if failure != nil {
		return u.fail(ctx, job, st, page, failure, holder, complete)
	}
	p.NewRound = false
	p.Cursors[cur.key()] = cursor
	if !sel.End {
		p.Cursors[cur.key()] = sourceCursor(sel.NextCursor)
	}
	var next []entity.Job
	switch {
	case !sel.End:
		p.Current = cur.key()
		next = append(next, entity.SyncPageJob{OrganizationID: org, ConnectionID: conn, DataKind: job.DataKind, CorrelationID: job.CorrelationID})
	case i+1 < len(steps):
		p.Current = steps[i+1].key()
		next = append(next, entity.SyncPageJob{OrganizationID: org, ConnectionID: conn, DataKind: job.DataKind, CorrelationID: job.CorrelationID})
	default:
		p.Current, p.NewRound, st.Phase = steps[0].key(), true, entity.SyncIncremental
		next = append(next, entity.SyncReconcileJob{OrganizationID: org, ConnectionID: conn, DataKind: job.DataKind, CorrelationID: job.CorrelationID})
	}
	next = append(next, histories(job, cur, sel)...)
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	st.Cursor, st.CheckpointAt, st.FailedCursor, st.LastOutcome, st.LastErrorCode = string(raw), &now, "", "succeeded", ""
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if _, err := u.d.Ingest.RecordPage(ctx, page); err != nil {
			return err
		}
		if err := u.d.Repo.SaveState(ctx, org, st); err != nil {
			return err
		}
		for _, j := range next {
			if err := u.d.Queue.Enqueue(ctx, j, entity.QueueBulk); err != nil {
				return err
			}
		}
		return u.end(ctx, org, conn, holder, complete)
	})
}

// fetch calls the adapter and cleans the page before any transaction. A refused call or a page the
// intake refuses is a failure; an error of the core is returned as such.
func (u *UseCase) fetch(ctx context.Context, org, conn, scope entity.ID, in entity.FetchInput) (*entity.PreparedPackage, envelope.Selection, *failure, error) {
	stream, err := u.d.Connect.Fetch(ctx, org, conn, in)
	if err != nil {
		if f, ok := u.classify(err); ok {
			return nil, envelope.Selection{}, f, nil
		}
		return nil, envelope.Selection{}, nil, err
	}
	page, err := u.d.Ingest.PreparePage(ctx, org, conn, scope, stream)
	_ = stream.Close()
	if err != nil {
		return nil, envelope.Selection{}, nil, err
	}
	if page.Receipt.Status == entity.ReceiptRejected {
		return page, envelope.Selection{}, &failure{code: page.Receipt.ReasonCode, class: page.Receipt.ReasonCode, backoff: true}, nil
	}
	var m envelope.Manifest
	if err := json.Unmarshal(page.Receipt.Manifest, &m); err != nil {
		return nil, envelope.Selection{}, nil, err
	}
	return page, m.Selection, nil, nil
}

// failure is what a refused page means for the collection.
type failure struct {
	code, class string
	// at is when to repeat; zero with backoff means after the pause of the failures in a row.
	at      time.Time
	backoff bool
	stop    bool
}

// classify maps a refused call to the reaction of table 10.6.
func (u *UseCase) classify(err error) (*failure, bool) {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	if !ok {
		return nil, false
	}
	now := u.d.Clock.Now()
	f := &failure{code: ae.Code, class: ae.Code}
	switch {
	case ae.Class == entity.CallPaused, ae.Class == entity.CallSource && ae.Code == entity.SourceRateLimited:
		f.code, f.class, f.at = entity.SourceRateLimited, entity.SourceRateLimited, now.Add(_defaultPause)
		if ae.RetryAt != nil {
			f.at = *ae.RetryAt
		}
	case ae.Class == entity.CallSource && ae.Code == entity.SourcePreparing:
		f.at = now.Add(_preparePoll)
		switch {
		case ae.RetryAfter > 0:
			f.at = now.Add(ae.RetryAfter)
		case ae.RetryAt != nil:
			f.at = *ae.RetryAt
		}
	case ae.Class == entity.CallSource && (ae.Code == entity.SourceAuth || ae.Code == "permission_missing"):
		f.stop, f.class = true, "auth"
	case ae.Class == entity.CallSource && ae.Code == "not_supported", ae.Class == entity.CallContractUnsupported:
		f.stop, f.code, f.class = true, string(ae.Class)+":"+ae.Code, string(ae.Class)
		if ae.Class == entity.CallContractUnsupported {
			f.code = string(entity.CallContractUnsupported)
		}
	case ae.Class == entity.CallAdapterUnavailable:
		f.backoff, f.code, f.class = true, string(entity.CallAdapterUnavailable), string(entity.CallAdapterUnavailable)
	default:
		f.backoff = true
	}
	return f, true
}

// Pause is the pause after retry failures in a row: 1, 2, 4, 8, 16, then 30 s, made longer by up to half
// with jitter, so connections do not repeat together after the source comes back (TZ 10.6).
func Pause(retry int, jitter float64) time.Duration {
	base := _backoff[min(retry, len(_backoff)-1)]
	return base + time.Duration(float64(base)/2*jitter)
}

// fail keeps the cursor and the time of the last success, records the failure and repeats the page later
// with a new job, so the repeat spends no attempt; auth and an unsupported adapter stop the collection.
func (u *UseCase) fail(ctx context.Context, job entity.SyncPageJob, st entity.SyncState, page *entity.PreparedPackage, f *failure, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	st.FailedCursor, st.LastOutcome, st.LastErrorCode = st.Cursor, "failed", f.code
	retry := job
	switch {
	case f.stop:
		st.Phase = entity.SyncStopped
	case f.backoff:
		retry.Retry++
		f.at = u.d.Clock.Now().Add(Pause(job.Retry, u.d.Rand()))
	}
	u.d.Metrics.CollectionFailed(f.class)
	zerolog.Ctx(ctx).Warn().Str("connection_id", conn.String()).Str("operation", "fetch").Str("data_kind", job.DataKind).
		Str("reason", f.code).Int("retry", job.Retry).Str("checkpoint", st.Cursor).Time("repeat_at", f.at).Bool("stopped", f.stop).
		Str("correlation_id", job.CorrelationID.String()).Msg("page of collection failed")
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if page != nil {
			if _, err := u.d.Ingest.RecordPage(ctx, page); err != nil {
				return err
			}
		}
		if err := u.d.Repo.SaveState(ctx, org, st); err != nil {
			return err
		}
		if !f.stop {
			if err := u.d.Queue.EnqueueAt(ctx, retry, entity.QueueBulk, f.at); err != nil {
				return err
			}
		}
		return u.end(ctx, org, conn, holder, complete)
	})
}

// lease takes the lease of the connection in a short transaction before the adapter is called; a run that
// does not get it waits a few seconds without spending an attempt.
func (u *UseCase) lease(ctx context.Context, org, conn entity.ID, holder string) error {
	now := u.d.Clock.Now()
	var got bool
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		got, err = u.d.Repo.AcquireLease(ctx, org, conn, holder, now.Add(_leaseFor), now)
		return err
	})
	if err != nil {
		return err
	}
	if !got {
		wait := _leaseWait + time.Duration(float64(3*time.Second)*u.d.Rand())
		return &entity.RetryLaterError{At: now.Add(wait), Err: errors.New("sync: another run holds the lease of the connection")}
	}
	return nil
}

func (u *UseCase) end(ctx context.Context, org, conn entity.ID, holder string, complete Complete) error {
	if err := u.d.Repo.ReleaseLease(ctx, org, conn, holder); err != nil {
		return err
	}
	if complete == nil {
		return nil
	}
	return complete(ctx)
}

// finish ends a run that has nothing to read: the data kind has no schema an adapter serves yet.
func (u *UseCase) finish(ctx context.Context, org, conn entity.ID, holder string, complete Complete) error {
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error { return u.end(ctx, org, conn, holder, complete) })
}

func steps(scopes []entity.ConnectionScope, schemas []string) []step {
	var out []step
	for i := range scopes {
		if !scopes[i].Active {
			continue
		}
		for _, schema := range schemas {
			out = append(out, step{scope: scopes[i], schema: schema})
		}
	}
	return out
}

func earliest(scopes []entity.ConnectionScope) *time.Time {
	var out *time.Time
	for i := range scopes {
		if from := scopes[i].HistoryFrom; scopes[i].Active && from != nil && (out == nil || from.Before(*out)) {
			out = from
		}
	}
	return out
}

func decode(raw string) (progress, error) {
	p := progress{Cursors: map[string]*entity.SourceCursor{}}
	if raw == "" {
		return p, nil
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return progress{}, err
	}
	if p.Cursors == nil {
		p.Cursors = map[string]*entity.SourceCursor{}
	}
	return p, nil
}

// histories are the jobs that read on the cut history of the objects the page names.
func histories(job entity.SyncPageJob, cur step, sel envelope.Selection) []entity.Job {
	out := make([]entity.Job, 0, len(sel.Truncated))
	for _, t := range sel.Truncated {
		out = append(out, entity.SyncHistoryJob{
			OrganizationID: job.OrganizationID, ConnectionID: job.ConnectionID, ScopeID: cur.scope.ID, SchemaID: cur.schema,
			ObjectKind: string(t.RecordKind), NativeNS: t.NativeNs, NativeID: t.NativeId, CorrelationID: job.CorrelationID,
		})
	}
	return out
}

func sourceCursor(c *envelope.Cursor) *entity.SourceCursor {
	if c == nil {
		return nil
	}
	return &entity.SourceCursor{ModifiedAt: c.ModifiedAt, NativeID: c.NativeId}
}
