// Package link owns the links of tasks, MR, CI runs, findings and documents with their method, basis and
// confirmation (TZ 10.3, BR-010). A key in a branch or a title gives a candidate; a link becomes confirmed by an
// explicit relation of the source or by the decision of a user.
package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

var errProposal = errors.New("link: invalid proposal")

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Repo    repo.LinkRepo
	Tenants repo.TenantRepo
	Tx      repo.Transactor
	Auditor Auditor
	Clock   clock.Clock
	NewID   func() (entity.ID, error)
}

type UseCase struct{ d Deps }

func New(d Deps) *UseCase { return &UseCase{d: d} }

// Record writes an explicit link of a source or a candidate in the caller's organization transaction. A repeated
// proposal with the same basis changes nothing, so a rejected candidate does not come back; a new basis makes a
// new version of the candidate.
func (u *UseCase) Record(ctx context.Context, orgID entity.ID, p entity.LinkProposal) (entity.LinkEdge, error) {
	if p.From == p.To || p.From.IsZero() || p.To.IsZero() || (p.Method != entity.LinkSourceExplicit && !p.Method.Proposed()) {
		return entity.LinkEdge{}, fmt.Errorf("%w: %s %s", errProposal, p.Type, p.Method)
	}
	if err := u.d.Repo.LockEdgeWrites(ctx, orgID); err != nil {
		return entity.LinkEdge{}, err
	}
	current, err := u.d.Repo.CurrentEdge(ctx, orgID, p.Type, p.From, p.To, p.Method)
	found := err == nil
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return entity.LinkEdge{}, err
	}
	if found {
		same, err := sameEvidence(current.Evidence, p.Evidence)
		if err != nil || p.Method == entity.LinkSourceExplicit || same {
			return current, err
		}
	}
	seq, err := u.d.Tenants.NextSeq(ctx, orgID)
	if err != nil {
		return entity.LinkEdge{}, err
	}
	if found {
		if err := u.d.Repo.SupersedeEdge(ctx, orgID, current.ID, seq, u.d.Clock.Now()); err != nil {
			return entity.LinkEdge{}, err
		}
	}
	status := entity.LinkCandidate
	if p.Method == entity.LinkSourceExplicit {
		status = entity.LinkConfirmed
	}
	e := entity.LinkEdge{
		Type: p.Type, From: p.From, To: p.To, Method: p.Method, Status: status,
		Knowledge: entity.Knowledge{KnownSeq: seq}, Record: p.Record, Evidence: p.Evidence,
	}
	if err := u.insert(ctx, orgID, &e); err != nil {
		return entity.LinkEdge{}, err
	}
	return e, nil
}

// Edge reads a version of a link; entity.ErrNotFound without it.
func (u *UseCase) Edge(ctx context.Context, orgID, id entity.ID) (entity.LinkEdge, error) {
	var out entity.LinkEdge
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.Edge(ctx, orgID, id)
		return err
	})
	return out, err
}

// EdgesAt reads the links of an object that hold for the slice seq, from it or to it, with status and method.
func (u *UseCase) EdgesAt(ctx context.Context, orgID, objectID entity.ID, dir entity.LinkDirection, seq int64) ([]entity.LinkEdge, error) {
	var out []entity.LinkEdge
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.EdgesAt(ctx, orgID, objectID, dir, seq)
		return err
	})
	return out, err
}

// Decide confirms or rejects a candidate in the caller's organization transaction; the caller has checked that
// the user sees both ends. A repeat with the same key returns the saved link, and a confirmation of a pair that
// is already confirmed returns the existing link instead of a second one.
func (u *UseCase) Decide(ctx context.Context, orgID, actor, linkID entity.ID, d entity.LinkDecision, key entity.ID) (entity.LinkEdge, error) {
	if err := u.d.Repo.LockEdgeWrites(ctx, orgID); err != nil {
		return entity.LinkEdge{}, err
	}
	e, err := u.current(ctx, orgID, linkID)
	if err != nil {
		return entity.LinkEdge{}, err
	}
	saved, err := u.d.Repo.EdgeByConfirmation(ctx, orgID, key)
	if err == nil {
		if saved.ActorUserID == nil || *saved.ActorUserID != actor || !saved.SameEnds(&e) || saved.Status != outcome(d) {
			return entity.LinkEdge{}, entity.ErrIdempotencyKeyReused
		}
		return saved, nil
	}
	if !errors.Is(err, entity.ErrNotFound) {
		return entity.LinkEdge{}, err
	}
	if d == entity.LinkReject {
		return u.reject(ctx, orgID, actor, &e, key)
	}
	confirmed, err := u.d.Repo.CurrentConfirmedEdge(ctx, orgID, e.Type, e.From, e.To)
	if err == nil {
		return confirmed, nil
	}
	if !errors.Is(err, entity.ErrNotFound) {
		return entity.LinkEdge{}, err
	}
	seq, err := u.d.Tenants.NextSeq(ctx, orgID)
	if err != nil {
		return entity.LinkEdge{}, err
	}
	at := u.d.Clock.Now()
	out := entity.LinkEdge{
		Type: e.Type, From: e.From, To: e.To, Method: entity.LinkUserConfirmed, Status: entity.LinkConfirmed,
		Knowledge: entity.Knowledge{KnownSeq: seq}, Evidence: e.Evidence, ActorUserID: &actor, ConfirmationID: &key, DecidedAt: &at,
	}
	return u.save(ctx, orgID, &e, &out, d)
}

// current is the link or, when it was replaced, the current version of the same pair and method.
func (u *UseCase) current(ctx context.Context, orgID, linkID entity.ID) (entity.LinkEdge, error) {
	e, err := u.d.Repo.Edge(ctx, orgID, linkID)
	if err != nil || e.SupersededSeq == nil {
		return e, err
	}
	return u.d.Repo.CurrentEdge(ctx, orgID, e.Type, e.From, e.To, e.Method)
}

func (u *UseCase) reject(ctx context.Context, orgID, actor entity.ID, e *entity.LinkEdge, key entity.ID) (entity.LinkEdge, error) {
	if e.Status == entity.LinkConfirmed {
		return entity.LinkEdge{}, entity.ErrLinkConfirmedBySource
	}
	if e.Status == entity.LinkRejected {
		return *e, nil
	}
	seq, err := u.d.Tenants.NextSeq(ctx, orgID)
	if err != nil {
		return entity.LinkEdge{}, err
	}
	at := u.d.Clock.Now()
	if err := u.d.Repo.SupersedeEdge(ctx, orgID, e.ID, seq, at); err != nil {
		return entity.LinkEdge{}, err
	}
	out := entity.LinkEdge{
		Type: e.Type, From: e.From, To: e.To, Method: e.Method, Status: entity.LinkRejected,
		Knowledge: entity.Knowledge{KnownSeq: seq}, Record: e.Record, Evidence: e.Evidence, ActorUserID: &actor, ConfirmationID: &key, DecidedAt: &at,
	}
	return u.save(ctx, orgID, e, &out, entity.LinkReject)
}

// save writes the result of a decision with its audit event in the same transaction.
func (u *UseCase) save(ctx context.Context, orgID entity.ID, candidate, result *entity.LinkEdge, d entity.LinkDecision) (entity.LinkEdge, error) {
	if err := u.insert(ctx, orgID, result); err != nil {
		return entity.LinkEdge{}, err
	}
	if err := u.audit(ctx, orgID, candidate, result, d); err != nil {
		return entity.LinkEdge{}, err
	}
	return *result, nil
}

func (u *UseCase) insert(ctx context.Context, orgID entity.ID, e *entity.LinkEdge) error {
	id, err := u.d.NewID()
	if err != nil {
		return fmt.Errorf("link: new edge id: %w", err)
	}
	e.ID = id
	return u.d.Repo.InsertEdge(ctx, orgID, e)
}

func (u *UseCase) audit(ctx context.Context, orgID entity.ID, candidate, result *entity.LinkEdge, d entity.LinkDecision) error {
	return u.d.Auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: orgID, ActorKind: entity.ActorUser, ActorID: *result.ActorUserID, OccurredAt: *result.DecidedAt,
		Operation: "link.decide", ObjectKind: "link", ObjectID: result.ID, Outcome: entity.AuditSucceeded,
		BaseVersion: string(candidate.Status), NewVersion: string(result.Status), CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		Details: map[string]any{
			"decision": string(d), "candidate_id": candidate.ID.String(), "link_type": string(result.Type),
			"confirmation_id": result.ConfirmationID.String(),
		},
	})
}

func outcome(d entity.LinkDecision) entity.LinkStatus {
	if d == entity.LinkReject {
		return entity.LinkRejected
	}
	return entity.LinkConfirmed
}

// sameEvidence compares two bases as JSON values: the database keeps them normalized.
func sameEvidence(a, b json.RawMessage) (bool, error) {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b), nil
	}
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false, fmt.Errorf("link: decode evidence: %w", err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false, fmt.Errorf("%w: evidence is not JSON", errProposal)
	}
	return reflect.DeepEqual(x, y), nil
}
