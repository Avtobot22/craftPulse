package decision

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_listOperation = "listExceptions"
	_getOperation  = "getException"
	_scanBatch     = 100
)

func statuses(r *entity.ExceptionRecord, seq int64, at time.Time) map[entity.ID]string {
	latest := map[entity.ID]entity.ExceptionTransition{}
	for i := range r.Transitions {
		t := &r.Transitions[i]
		if t.Seq > seq || t.At.After(at) {
			continue
		}
		prev, ok := latest[t.VersionID]
		if !ok || t.At.After(prev.At) || t.At.Equal(prev.At) && t.Seq > prev.Seq {
			latest[t.VersionID] = *t
		}
	}
	out := make(map[entity.ID]string, len(latest))
	for id, t := range latest {
		out[id] = t.Status
	}
	return out
}

// Effective chooses the newest version approved at t on the knowledge cut. An automatic approval is also an event.
func Effective(r *entity.ExceptionRecord, seq int64, at time.Time) *entity.ExceptionVersion {
	return selectEffective(r.Versions, statuses(r, seq, at), seq, at)
}

func selectEffective(versions []entity.ExceptionVersion, status map[entity.ID]string, seq int64, at time.Time) *entity.ExceptionVersion {
	var found *entity.ExceptionVersion
	for i := range versions {
		v := &versions[i]
		if v.Seq <= seq && status[v.ID] == entity.ExceptionApproved && !at.Before(v.EffectiveFrom) && at.Before(v.ExpiresAt) &&
			(found == nil || v.Number > found.Number) {
			found = v
		}
	}
	return found
}

func viewOf(r *entity.ExceptionRecord, at time.Time, seq int64, visible map[entity.FindingAccess]bool) entity.ExceptionView {
	v := entity.ExceptionView{ID: r.ID}
	status := statuses(r, seq, at)
	var versions []entity.ExceptionVersion
	shown := map[entity.ID]bool{}
	for i := range r.Versions {
		b := &r.Versions[i]
		if b.Seq <= seq && !b.CreatedAt.After(at) && visible[b.Access(false)] {
			item := *b
			item.Status = status[b.ID]
			versions = append(versions, item)
			shown[b.ID] = true
		}
	}
	if len(versions) == 0 {
		return v
	}
	v.Current, v.Versions = versions[0], versions[1:]
	for i := range r.Transitions {
		t := &r.Transitions[i]
		if t.Seq <= seq && !t.At.After(at) && shown[t.VersionID] {
			v.Transitions = append(v.Transitions, *t)
		}
	}
	v.State = stateAt(&v.Current, selectEffective(versions, status, seq, at) != nil, at)
	return v
}

func stateAt(current *entity.ExceptionVersion, anyActive bool, at time.Time) string {
	switch {
	case anyActive:
		return entity.ExceptionActive
	case current.Status == entity.ExceptionRejected || current.Status == entity.ExceptionWithdrawn:
		return current.Status
	case !at.Before(current.ExpiresAt):
		return entity.ExceptionExpired
	default:
		return entity.ExceptionPending
	}
}

// ActiveAt lists one active version of each decision of the area at the date and knowledge cut.
func (u *UseCase) ActiveAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error) {
	return u.areaVersionsAt(ctx, org, area, seq, at, false)
}

// ActiveAndExpiredAt returns one active version per decision, or its latest expired approval when none is active.
func (u *UseCase) ActiveAndExpiredAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error) {
	return u.areaVersionsAt(ctx, org, area, seq, at, true)
}

func (u *UseCase) areaVersionsAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time, includeExpired bool) ([]entity.ExceptionVersion, error) {
	var out []entity.ExceptionVersion
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		ids, err := u.d.Repo.AreaDecisionIDs(ctx, org, area, seq, stamp(at), includeExpired)
		if err != nil {
			return err
		}
		all, err := u.d.Repo.Histories(ctx, org, ids, seq)
		if err != nil {
			return err
		}
		for _, id := range ids {
			v, status := areaVersion(all[id], seq, at, includeExpired)
			if v == nil || v.Scope.AreaID != area {
				continue
			}
			version := *v
			version.Status = status
			out = append(out, version)
		}
		return nil
	})
	return out, err
}

func areaVersion(r *entity.ExceptionRecord, seq int64, at time.Time, includeExpired bool) (version *entity.ExceptionVersion, status string) {
	state := statuses(r, seq, at)
	if v := selectEffective(r.Versions, state, seq, at); v != nil {
		return v, entity.ExceptionApproved
	}
	if !includeExpired {
		return nil, ""
	}
	var expired *entity.ExceptionVersion
	for i := range r.Versions {
		v := &r.Versions[i]
		if v.Seq <= seq && state[v.ID] == entity.ExceptionApproved && !at.Before(v.ExpiresAt) &&
			v.CreatedAt.Before(v.ExpiresAt) && (expired == nil || v.Number > expired.Number) {
			expired = v
		}
	}
	return expired, entity.ExceptionExpired
}

// Get returns the latest version and the visible part of its history.
func (u *UseCase) Get(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.ExceptionView, error) {
	r, cut, err := u.read(ctx, s.OrganizationID, id)
	if err != nil {
		return entity.ExceptionView{}, err
	}
	return u.readView(ctx, s, scope, _getOperation, &r, &cut)
}

func (u *UseCase) read(ctx context.Context, org, id entity.ID) (entity.ExceptionRecord, entity.ExceptionCursor, error) {
	var r entity.ExceptionRecord
	cut := entity.ExceptionCursor{At: stamp(u.d.Clock.Now())}
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		cut.Seq, _, err = u.d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		r, err = u.d.Repo.Decision(ctx, org, id, false)
		return err
	})
	trimVersions(&r, &cut)
	if err == nil && len(r.Versions) == 0 {
		err = entity.ErrNotFound
	}
	return r, cut, err
}

func (u *UseCase) readView(ctx context.Context, s entity.Subject, scope entity.Scope, operation string,
	r *entity.ExceptionRecord, cut *entity.ExceptionCursor,
) (entity.ExceptionView, error) {
	wants := make([]entity.FindingAccess, 0, len(r.Versions))
	for i := range r.Versions {
		wants = append(wants, r.Versions[i].Access(false))
	}
	visible, err := u.d.Access.VisibleFindings(ctx, s, scope, operation, wants)
	if err != nil {
		return entity.ExceptionView{}, err
	}
	if len(r.Versions) == 0 || !visible[r.Versions[0].Access(false)] {
		return entity.ExceptionView{}, entity.ErrNotFound
	}
	return viewOf(r, cut.At, cut.Seq, visible), nil
}

// List pages by the latest version's expiry or the creation time of the decision, on a pinned cut.
func (u *UseCase) List(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.ExceptionQuery) (entity.ExceptionPage, error) {
	org := s.OrganizationID
	cut := entity.ExceptionCursor{At: stamp(u.d.Clock.Now())}
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		seq, oldest, err := u.d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		cut.Seq = seq
		if q.After != nil {
			if q.After.At.After(cut.At) || q.After.Seq > seq || q.After.Seq < oldest {
				return entity.ErrInvalidCursor
			}
			cut = *q.After
		}
		return nil
	})
	if err != nil {
		return entity.ExceptionPage{}, err
	}
	out := entity.ExceptionPage{}
	limit := 50
	if q.Limit > 0 {
		limit = min(q.Limit, _scanBatch)
	}
	for {
		cursors, records, err := u.batch(ctx, org, &q, &cut)
		if err != nil {
			return entity.ExceptionPage{}, err
		}
		visible, err := u.batchVisibility(ctx, s, scope, records)
		if err != nil {
			return entity.ExceptionPage{}, err
		}
		for _, next := range cursors {
			r := records[next.ID]
			if len(r.Versions) == 0 || !visible[r.Versions[0].Access(false)] {
				out.Notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
				continue
			}
			v := viewOf(r, cut.At, cut.Seq, visible)
			if q.State != "" && q.State != v.State {
				continue
			}
			if len(out.Items) == limit {
				return out, nil
			}
			out.Items = append(out.Items, v)
			out.Next = &next
		}
		if len(cursors) < _scanBatch {
			out.Next = nil
			return out, nil
		}
		cut = cursors[len(cursors)-1]
	}
}

func (u *UseCase) batch(ctx context.Context, org entity.ID, q *entity.ExceptionQuery, cut *entity.ExceptionCursor,
) ([]entity.ExceptionCursor, map[entity.ID]*entity.ExceptionRecord, error) {
	var cursors []entity.ExceptionCursor
	var r map[entity.ID]*entity.ExceptionRecord
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		cursors, err = u.d.Repo.PageDecisions(ctx, org, q, cut, _scanBatch)
		if err != nil {
			return err
		}
		ids := make([]entity.ID, len(cursors))
		for i := range cursors {
			ids[i] = cursors[i].ID
		}
		r, err = u.d.Repo.Histories(ctx, org, ids, cut.Seq)
		for _, record := range r {
			trimVersions(record, cut)
		}
		return err
	})
	return cursors, r, err
}

func (u *UseCase) batchVisibility(ctx context.Context, s entity.Subject, scope entity.Scope, records map[entity.ID]*entity.ExceptionRecord,
) (map[entity.FindingAccess]bool, error) {
	var wants []entity.FindingAccess
	for _, r := range records {
		for i := range r.Versions {
			wants = append(wants, r.Versions[i].Access(false))
		}
	}
	return u.d.Access.VisibleFindings(ctx, s, scope, _listOperation, wants)
}

func trimVersions(r *entity.ExceptionRecord, cut *entity.ExceptionCursor) {
	r.Versions = slices.DeleteFunc(r.Versions, func(v entity.ExceptionVersion) bool {
		return v.Seq > cut.Seq || v.CreatedAt.After(cut.At)
	})
}
