package decision

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_createOperation  = "createException"
	_versionOperation = "createExceptionVersion"
	_decideOperation  = "decideException"
)

// Save creates an exception or appends a version after change has resolved its finding scope.
func (u *UseCase) Save(ctx context.Context, s entity.Subject, scope entity.Scope, cmd entity.ExceptionWrite) (entity.ExceptionView, error) {
	if err := u.CheckDraft(&cmd.Draft); err != nil {
		return entity.ExceptionView{}, err
	}
	if !allowedKind(cmd.FindingKind) {
		return entity.ExceptionView{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	if cmd.Key.IsZero() {
		return entity.ExceptionView{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "Idempotency-Key", Code: entity.FieldRequired}}}
	}
	org := s.OrganizationID
	var id entity.ID
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if err := u.d.Repo.LockConfirmation(ctx, org, cmd.Key); err != nil {
			return err
		}
		saved, err := u.d.Repo.VersionByConfirmation(ctx, org, cmd.Key)
		switch {
		case err == nil:
			if !sameWrite(&saved, &cmd, s.UserID) {
				return entity.ErrIdempotencyKeyReused
			}
			id = saved.DecisionID
			return nil
		case !errors.Is(err, entity.ErrNotFound):
			return err
		}
		id, err = u.saveNew(ctx, s, &cmd)
		return err
	})
	if err != nil {
		return entity.ExceptionView{}, err
	}
	return u.Get(ctx, s, scope, id)
}

func sameWrite(v *entity.ExceptionVersion, cmd *entity.ExceptionWrite, author entity.ID) bool {
	return v.AuthorID == author && v.Base == cmd.Base && (cmd.ID.IsZero() || cmd.ID == v.DecisionID) &&
		v.FindingKind == cmd.FindingKind && v.ExceptionDraft == cmd.Draft
}

func (u *UseCase) saveNew(ctx context.Context, s entity.Subject, cmd *entity.ExceptionWrite) (entity.ID, error) {
	org, now := s.OrganizationID, stamp(u.d.Clock.Now())
	rec := entity.ExceptionRecord{}
	op := _createOperation
	var err error
	if cmd.ID.IsZero() {
		rec.ID, err = u.d.Repo.InsertDecision(ctx, org, s.UserID, now)
	} else {
		op = _versionOperation
		rec, err = u.d.Repo.Decision(ctx, org, cmd.ID, true)
	}
	if err != nil {
		return entity.ID{}, err
	}
	base := len(rec.Versions)
	if cmd.Base != base || base == math.MaxInt32 {
		return entity.ID{}, entity.ErrVersionConflict
	}
	if base > 0 && rec.Versions[0].Scope.AreaID != cmd.Draft.Scope.AreaID {
		return entity.ID{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope/area_id", Code: entity.FieldOutsideScope}}}
	}
	seq, err := u.d.Tenants.NextSeq(ctx, org)
	if err != nil {
		return entity.ID{}, err
	}
	v := entity.ExceptionVersion{
		DecisionID: rec.ID, Key: cmd.Key, AuthorID: s.UserID, Number: base + 1, Base: base, CreatedAt: now, Seq: seq,
		ExceptionDraft: cmd.Draft, FindingKind: cmd.FindingKind,
		ApprovalRequired: cmd.Draft.Type == entity.ExceptionAcceptedRisk && entity.SensitiveFinding(cmd.FindingKind),
	}
	v.ID, err = u.d.Repo.InsertVersion(ctx, org, &v)
	if err != nil {
		return entity.ID{}, err
	}
	state := entity.ExceptionApproved
	if v.ApprovalRequired {
		state = entity.ExceptionPending
	}
	t := entity.ExceptionTransition{VersionID: v.ID, ActorID: s.UserID, Version: v.Number, Seq: seq, At: now, Status: state}
	if err := u.d.Repo.InsertTransition(ctx, org, &t); err != nil {
		return entity.ID{}, err
	}
	rec.Versions = append(rec.Versions, v)
	rec.Transitions = append(rec.Transitions, t)
	if err := u.audit(ctx, s, op, &v, state, base); err != nil {
		return entity.ID{}, err
	}
	return rec.ID, u.onChange(ctx, org, &rec, v.Scope.AreaID, now)
}

// Decide approves or rejects the pending version as a reviewer or withdraws it as its author.
func (u *UseCase) Decide(ctx context.Context, s entity.Subject, cmd entity.ExceptionDecision) (entity.ExceptionView, error) {
	state, action, err := transitionRequest(&cmd)
	if err != nil {
		return entity.ExceptionView{}, err
	}
	scope, err := u.reviewAccess(ctx, s, &cmd, action)
	if err != nil {
		return entity.ExceptionView{}, err
	}
	org := s.OrganizationID
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if err := u.d.Repo.LockConfirmation(ctx, org, cmd.Key); err != nil {
			return err
		}
		id, saved, err := u.d.Repo.TransitionByConfirmation(ctx, org, cmd.Key)
		switch {
		case err == nil:
			if id != cmd.ID || saved.Version != cmd.Base || saved.ActorID != s.UserID || saved.Status != state || saved.Comment != cmd.Comment {
				return entity.ErrIdempotencyKeyReused
			}
			return nil
		case !errors.Is(err, entity.ErrNotFound):
			return err
		}
		return u.applyTransition(ctx, s, &cmd, state)
	})
	if err != nil {
		return entity.ExceptionView{}, err
	}
	return u.Get(ctx, s, scope, cmd.ID)
}

func transitionRequest(cmd *entity.ExceptionDecision) (status, action string, err error) {
	cmd.Comment = strings.TrimSpace(cmd.Comment)
	if utf8.RuneCountInString(cmd.Comment) > 2000 || strings.ContainsRune(cmd.Comment, 0) {
		return "", "", &entity.ValidationError{Fields: []entity.FieldError{{Field: "/comment", Code: entity.FieldOutOfRange}}}
	}
	if cmd.Key.IsZero() {
		return "", "", &entity.ValidationError{Fields: []entity.FieldError{{Field: "Idempotency-Key", Code: entity.FieldRequired}}}
	}
	switch cmd.Action {
	case entity.ExceptionApprove:
		return entity.ExceptionApproved, _actionReview, nil
	case entity.ExceptionReject:
		return entity.ExceptionRejected, _actionReview, nil
	case entity.ExceptionWithdraw:
		return entity.ExceptionWithdrawn, _actionManage, nil
	default:
		return "", "", &entity.ValidationError{Fields: []entity.FieldError{{Field: "/decision", Code: entity.FieldNotAllowedValue}}}
	}
}

func (u *UseCase) reviewAccess(ctx context.Context, s entity.Subject, cmd *entity.ExceptionDecision, action string) (entity.Scope, error) {
	readScope, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{Operation: _decideOperation, Action: _actionRead})
	if err != nil {
		return entity.Scope{}, err
	}
	r, cut, err := u.read(ctx, s.OrganizationID, cmd.ID)
	if err != nil {
		return entity.Scope{}, err
	}
	if _, err := u.readView(ctx, s, readScope, _decideOperation, &r, &cut); err != nil {
		return entity.Scope{}, err
	}
	scope, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{Operation: _decideOperation, Action: action})
	if err != nil {
		return entity.Scope{}, err
	}
	current := r.Versions[0].Access(true)
	visible, err := u.d.Access.VisibleFindings(ctx, s, scope, _decideOperation, []entity.FindingAccess{current})
	if err != nil {
		return entity.Scope{}, err
	}
	if !visible[current] {
		return entity.Scope{}, entity.ErrForbidden
	}
	return scope, nil
}

func (u *UseCase) applyTransition(ctx context.Context, s entity.Subject, cmd *entity.ExceptionDecision, status string) error {
	org := s.OrganizationID
	r, err := u.d.Repo.Decision(ctx, org, cmd.ID, true)
	if err != nil {
		return err
	}
	if len(r.Versions) == 0 || r.Versions[0].Number != cmd.Base {
		return entity.ErrVersionConflict
	}
	current := &r.Versions[0]
	now := stamp(u.d.Clock.Now())
	previous := statuses(&r, math.MaxInt64, now)[current.ID]
	if err := checkTransition(current, s.UserID, cmd.Action, previous, now); err != nil {
		return err
	}
	seq, err := u.d.Tenants.NextSeq(ctx, org)
	if err != nil {
		return err
	}
	t := entity.ExceptionTransition{
		VersionID: current.ID, ActorID: s.UserID, Key: cmd.Key, Version: current.Number, Seq: seq, Status: status, Comment: cmd.Comment, At: now,
	}
	if err := u.d.Repo.InsertTransition(ctx, org, &t); err != nil {
		return err
	}
	r.Transitions = append(r.Transitions, t)
	if err := u.audit(ctx, s, _decideOperation, current, status, current.Number); err != nil {
		return err
	}
	return u.onChange(ctx, org, &r, current.Scope.AreaID, now)
}

func checkTransition(v *entity.ExceptionVersion, actor entity.ID, decision, previous string, now time.Time) error {
	if (decision == entity.ExceptionApprove && v.ApprovalRequired && actor == v.AuthorID) ||
		(decision == entity.ExceptionWithdraw && actor != v.AuthorID) {
		return entity.ErrForbidden
	}
	allowed := previous == entity.ExceptionPending || previous == entity.ExceptionApproved && decision == entity.ExceptionWithdraw
	if !now.Before(v.ExpiresAt) || !allowed {
		return entity.ErrStateConflict
	}
	return nil
}

func (u *UseCase) audit(ctx context.Context, s entity.Subject, op string, v *entity.ExceptionVersion, status string, base int) error {
	e := entity.AuditEvent{
		OrganizationID: s.OrganizationID, ActorKind: entity.ActorUser, ActorID: s.UserID, Operation: op,
		ObjectKind: "exception", ObjectID: v.DecisionID, AreaID: v.Scope.AreaID, Outcome: entity.AuditSucceeded,
		NewVersion: strconv.Itoa(v.Number), CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		Details: map[string]any{"decision_type": v.Type, "status": status},
	}
	if base > 0 {
		e.BaseVersion = strconv.Itoa(base)
	}
	if err := u.d.Auditor.Record(ctx, e); err != nil {
		return fmt.Errorf("decision: audit %s: %w", op, err)
	}
	return nil
}
