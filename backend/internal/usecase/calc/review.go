package calc

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_eventReady    = "ready"
	_eventAssigned = "reviewer_assigned"
	_eventRemoved  = "reviewer_removed"
	_eventReview   = "review"
	_eventApproval = "approval"
	_eventComment  = "comment"
)

// ReviewWaits are the waits of the MRs of an area at the slice seq and the date asOf by ReviewWaitOf, for the queue
// and the card of a task; the history of the area is incomplete while a connection of it has not reconciled
// without differences. The caller checks the access of its reader to the area.
func (u *UseCase) ReviewWaits(ctx context.Context, orgID, areaID entity.ID, seq int64, asOf time.Time) (map[entity.ID]entity.ReviewWait, error) {
	facts, err := u.d.Facts.ReviewFacts(ctx, orgID, areaID, seq)
	if err != nil {
		return nil, err
	}
	collection, err := u.collectionOf(ctx, orgID, []entity.ID{areaID})
	if err != nil {
		return nil, err
	}
	mrs := reviewInputOf([]entity.ReviewFacts{facts}, incompleteAreas(collection))
	out := make(map[entity.ID]entity.ReviewWait, len(mrs))
	for i := range mrs {
		out[mrs[i].version.MRID] = mrs[i].wait(asOf)
	}
	return out, nil
}

// ReviewWaitOf is the rule of the review queue (FR-031, BR-014): the wait of an MR at the date at, from its version
// and its events known at one knowledge number. kinds are the confirmed kinds of accounts at that number; an
// account without one is not a human. A response is a review or an approval of a human who is not the author,
// or such a comment when the source gives no reviews (reviewsGiven false). Without a response the MR waits for a
// reviewer when the field of reviewers is available and none is assigned, otherwise for the first response; with
// an incomplete history (historyComplete false) whether it was answered is unknown.
func ReviewWaitOf(mr *entity.MRVersion, events []entity.MREvent, kinds map[entity.ID]entity.PersonKind, reviewsGiven, historyComplete bool,
	at time.Time,
) entity.ReviewWait {
	since, fallback := readyOf(mr, events)
	out := entity.ReviewWait{Since: since, FallbackOpen: fallback}
	if e := firstResponse(mr, events, kinds, reviewsGiven, since, at); e != nil {
		out.Kind, out.Since, out.ResponseKind = entity.ReviewResponded, e.EventTime, e.Kind
		out.WaitSeconds = seconds(since, e.EventTime)
		return out
	}
	out.WaitSeconds = seconds(since, at)
	switch {
	case !historyComplete:
		out.Kind = entity.ReviewUnknown
	case mr.ReviewerField && !reviewerAssigned(mr, events, at):
		out.Kind = entity.ReviewNoReviewer
	default:
		out.Kind = entity.ReviewAwaitingFirst
	}
	return out
}

// readyOf is when the MR became ready: the time of the version, else its first ready event, else its opening as
// a signed fallback (FR-031 A1).
func readyOf(mr *entity.MRVersion, events []entity.MREvent) (time.Time, bool) {
	if mr.ReadyAt != nil {
		return mr.ReadyAt.UTC(), false
	}
	for i := range events {
		if events[i].Kind == _eventReady {
			return events[i].EventTime.UTC(), false
		}
	}
	return mr.OpenedAt.UTC(), true
}

func firstResponse(mr *entity.MRVersion, events []entity.MREvent, kinds map[entity.ID]entity.PersonKind, reviewsGiven bool, since, at time.Time,
) *entity.MREvent {
	for i := range events {
		e := &events[i]
		if e.EventTime.Before(since) || e.EventTime.After(at) || !responds(e.Kind, reviewsGiven) || e.ActorAccountID == nil {
			continue
		}
		actor := *e.ActorAccountID
		if kinds[actor] != entity.PersonHuman || (mr.AuthorAccountID != nil && *mr.AuthorAccountID == actor) {
			continue
		}
		return e
	}
	return nil
}

func responds(kind string, reviewsGiven bool) bool {
	if reviewsGiven {
		return kind == _eventReview || kind == _eventApproval
	}
	return kind == _eventComment
}

// reviewerAssigned replays the assignments of reviewers up to at; without such events the reviewers of the
// version decide.
func reviewerAssigned(mr *entity.MRVersion, events []entity.MREvent, at time.Time) bool {
	assigned := map[entity.ID]bool{}
	seen := false
	for i := range events {
		e := &events[i]
		if e.EventTime.After(at) || (e.Kind != _eventAssigned && e.Kind != _eventRemoved) {
			continue
		}
		seen = true
		var target entity.ID
		if e.TargetAccountID != nil {
			target = *e.TargetAccountID
		}
		if e.Kind == _eventAssigned {
			assigned[target] = true
		} else {
			delete(assigned, target)
		}
	}
	if !seen {
		return len(mr.ReviewerAccountIDs) > 0
	}
	return len(assigned) > 0
}

// readyOpen tells whether the MR is open and out of draft at t by its known times.
func readyOpen(mr *entity.MRVersion, events []entity.MREvent, t time.Time) bool {
	if mr.OpenedAt.After(t) || (mr.MergedAt != nil && !mr.MergedAt.After(t)) || (mr.ClosedAt != nil && !mr.ClosedAt.After(t)) {
		return false
	}
	if mr.State != "open" && mr.MergedAt == nil && mr.ClosedAt == nil {
		return false
	}
	ready, fallback := readyOf(mr, events)
	if fallback {
		return !mr.Draft
	}
	return !ready.After(t)
}

func seconds(from, to time.Time) int64 {
	if to.Before(from) {
		return 0
	}
	return int64(to.Sub(from) / time.Second)
}

// MRReviewsAt applies the shared waiting rule to visible MRs on one time and knowledge cut.
func (u *UseCase) MRReviewsAt(ctx context.Context, scope entity.Scope, org, area entity.ID, seq int64, at time.Time) ([]entity.MRReview, error) {
	facts, err := u.d.Facts.MRReviewFactsAt(ctx, scope, org, area, seq)
	if err != nil {
		return nil, err
	}
	var collection []entity.AreaCollection
	if u.d.Collection != nil {
		collection, err = u.d.Collection.MRCollection(ctx, org, []entity.ID{area})
		if err != nil {
			return nil, err
		}
	}
	mrs := reviewInputOf([]entity.ReviewFacts{facts}, incompleteAreas(collection))
	out := make([]entity.MRReview, len(mrs))
	for i := range mrs {
		m := &mrs[i]
		row := entity.MRReview{Version: m.version, AuthorKind: entity.PersonUnknown}
		if id := m.version.AuthorAccountID; id != nil && m.kinds[*id] != "" {
			row.AuthorKind = m.kinds[*id]
		}
		if !m.version.Draft {
			since, _ := readyOf(&m.version, m.events)
			end := at
			for _, finished := range []*time.Time{m.version.MergedAt, m.version.ClosedAt} {
				if finished != nil && finished.Before(end) {
					end = *finished
				}
			}
			if !since.After(end) {
				wait := m.wait(end)
				row.Wait = &wait
			}
		}
		out[i] = row
	}
	return out, nil
}
