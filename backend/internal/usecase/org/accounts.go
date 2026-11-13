package org

import (
	"context"
	"crypto/sha256"
	"slices"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _operationAssign = "createAccountAssignments"

// ListPersons pages the people, bots and service participants accounts are linked to (UC-08).
func (u *UseCase) ListPersons(ctx context.Context, s entity.Subject, f entity.PersonFilter) (entity.PersonPage, error) {
	limit := f.Limit
	f.Limit++
	var page entity.PersonPage
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.orgs.PagePersons(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			next := items[limit-1].ID
			page.Next = &next
		}
		page.Items = items
		return nil
	})
	return page, err
}

// ListSourceAccounts pages the accounts of sources with their current link and the proposals for them: a
// login or an email shared with an account linked to a person; a display name proposes nothing (FR-018).
func (u *UseCase) ListSourceAccounts(ctx context.Context, s entity.Subject, f entity.SourceAccountFilter) (entity.SourceAccountPage, error) {
	limit := f.Limit
	f.Limit++
	var page entity.SourceAccountPage
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.orgs.PageSourceAccounts(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			next := items[limit-1].ID
			page.Next = &next
		}
		ids := make([]entity.ID, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		proposals, err := u.orgs.AccountProposals(ctx, s.OrganizationID, ids)
		if err != nil {
			return err
		}
		for i := range items {
			items[i].Proposals = proposalsFor(&items[i], proposals[items[i].ID])
		}
		page.Items = items
		return nil
	})
	return page, err
}

// proposalsFor drops a proposal of the person the account is already linked to.
func proposalsFor(v *entity.AccountView, all []entity.AccountProposal) []entity.AccountProposal {
	out := make([]entity.AccountProposal, 0, len(all))
	for _, p := range all {
		if v.Assignment != nil && v.Assignment.PersonID != nil && *v.Assignment.PersonID == p.PersonID {
			continue
		}
		out = append(out, p)
	}
	return out
}

// AssignAccounts confirms a batch of decisions under one idempotency key (UC-08, FR-018). Every decision has
// its own outcome: a link the form did not see is a version conflict, a decision equal to the current link
// changes nothing, a new link or kind is a new version with the next knowledge number. A repeat of the key
// returns the versions it saved; the same key with another decision is refused (TZ 5.2).
func (u *UseCase) AssignAccounts(ctx context.Context, s entity.Subject, key entity.ID, items []entity.AssignmentRequest) ([]entity.AssignmentOutcome, error) {
	if err := validAssignments(items); err != nil {
		return nil, err
	}
	org := s.OrganizationID
	ids := make([]entity.ID, len(items))
	confirmations := make([]entity.ID, len(items))
	var persons []entity.ID
	for i := range items {
		ids[i], confirmations[i] = items[i].SourceAccountID, confirmationID(key, items[i].SourceAccountID)
		if items[i].PersonID != nil {
			persons = append(persons, *items[i].PersonID)
		}
	}
	var out []entity.AssignmentOutcome
	err := u.tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		seq, err := u.tenants.NextSeq(ctx, org)
		if err != nil {
			return err
		}
		saved, err := u.orgs.AssignmentsByConfirmation(ctx, org, confirmations)
		if err != nil {
			return err
		}
		accounts, err := u.orgs.SourceAccountsByID(ctx, org, ids)
		if err != nil {
			return err
		}
		current, err := u.orgs.LockCurrentAssignments(ctx, org, ids)
		if err != nil {
			return err
		}
		known, err := u.orgs.ExistingPersons(ctx, org, persons)
		if err != nil {
			return err
		}
		b := &assignBatch{u: u, s: s, seq: seq, accounts: accounts, current: current, known: known}
		out = make([]entity.AssignmentOutcome, 0, len(items))
		for i := range items {
			if prev, ok := saved[confirmations[i]]; ok {
				if prev.ConfirmedBy != s.UserID || !sameDecision(&items[i], &prev) {
					return entity.ErrIdempotencyKeyReused
				}
				id := prev.ID
				out = append(out, entity.AssignmentOutcome{SourceAccountID: items[i].SourceAccountID, Outcome: entity.AssignmentSaved, AssignmentID: &id})
				continue
			}
			res, err := b.apply(ctx, &items[i], confirmations[i])
			if err != nil {
				return err
			}
			out = append(out, res)
		}
		return nil
	})
	return out, err
}

func validAssignments(items []entity.AssignmentRequest) error {
	if len(items) == 0 || len(items) > entity.MaxAssignmentBatch {
		return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/items", Code: entity.FieldOutOfRange}}}
	}
	var fields []entity.FieldError
	seen := make(map[entity.ID]bool, len(items))
	for i := range items {
		it := &items[i]
		at := "/items/" + strconv.Itoa(i)
		if seen[it.SourceAccountID] {
			fields = append(fields, entity.FieldError{Field: at + "/source_account_id", Code: entity.FieldDuplicate})
		}
		seen[it.SourceAccountID] = true
		switch it.ActorKind {
		case entity.PersonHuman, entity.PersonBot, entity.PersonSystem, entity.PersonUnknown:
		default:
			fields = append(fields, entity.FieldError{Field: at + "/actor_kind", Code: entity.FieldNotAllowedValue})
		}
		if it.CreatePerson && it.PersonID != nil {
			fields = append(fields, entity.FieldError{Field: at + "/person_id", Code: entity.FieldNotAllowedValue})
		}
	}
	if len(fields) > 0 {
		return &entity.ValidationError{Fields: fields}
	}
	return nil
}

// confirmationID derives the confirmation of one decision of a batch from its key, so a repeat of the key
// finds what each decision saved.
func confirmationID(key, account entity.ID) entity.ID {
	sum := sha256.Sum256(append(key[:], account[:]...))
	var id entity.ID
	copy(id[:], sum[:16])
	id[6] = id[6]&0x0f | 0x80 // version 8: a UUID made by the application
	id[8] = id[8]&0x3f | 0x80 // RFC 9562 variant
	return id
}

// sameDecision tells a repeated decision from another one sent with the same key.
func sameDecision(it *entity.AssignmentRequest, saved *entity.AccountAssignment) bool {
	if it.ActorKind != saved.ActorKind {
		return false
	}
	if it.CreatePerson {
		return saved.PersonID != nil
	}
	return equalID(it.PersonID, saved.PersonID)
}

func equalID(a, b *entity.ID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// assignBatch is what a batch read under the knowledge number before its decisions.
type assignBatch struct {
	u        *UseCase
	s        entity.Subject
	seq      int64
	accounts map[entity.ID]entity.AccountView
	current  map[entity.ID]entity.AccountAssignment
	known    []entity.ID
}

func (b *assignBatch) apply(ctx context.Context, it *entity.AssignmentRequest, confirmation entity.ID) (entity.AssignmentOutcome, error) {
	org := b.s.OrganizationID
	out := entity.AssignmentOutcome{SourceAccountID: it.SourceAccountID}
	account, ok := b.accounts[it.SourceAccountID]
	switch {
	case !ok:
		out.Outcome, out.ErrorCode = entity.AssignmentRejected, entity.AssignmentAccountNotFound
		return out, nil
	case it.ActorKind == entity.PersonHuman && it.PersonID == nil && !it.CreatePerson:
		out.Outcome, out.ErrorCode = entity.AssignmentRejected, entity.AssignmentPersonRequired
		return out, nil
	case it.PersonID != nil && !slices.Contains(b.known, *it.PersonID):
		out.Outcome, out.ErrorCode = entity.AssignmentRejected, entity.AssignmentPersonNotFound
		return out, nil
	}
	cur, has := b.current[it.SourceAccountID]
	if has != (it.BaseAssignmentID != nil) || (has && cur.ID != *it.BaseAssignmentID) {
		out.Outcome = entity.AssignmentVersionConflict
		return out, nil
	}
	if has && !it.CreatePerson && cur.ActorKind == it.ActorKind && equalID(cur.PersonID, it.PersonID) {
		id := cur.ID
		out.Outcome, out.AssignmentID = entity.AssignmentUnchanged, &id
		return out, nil
	}
	now := b.u.clock.Now()
	person := it.PersonID
	if it.CreatePerson {
		id, err := b.u.orgs.CreatePerson(ctx, org, account.Account.DisplayName, it.ActorKind, now)
		if err != nil {
			return out, err
		}
		person = &id
	}
	base := ""
	if has {
		base = cur.ID.String()
		if err := b.u.orgs.SupersedeAssignments(ctx, org, []entity.ID{cur.ID}, b.seq, now); err != nil {
			return out, err
		}
	}
	id, err := b.u.orgs.InsertAssignment(ctx, org, entity.AccountAssignment{
		SourceAccountID: it.SourceAccountID, PersonID: person, ActorKind: it.ActorKind, Knowledge: entity.Knowledge{KnownSeq: b.seq},
		ConfirmedBy: b.s.UserID, ConfirmedAt: now, ConfirmationID: confirmation,
	})
	if err != nil {
		return out, err
	}
	details := map[string]any{"source_account_id": it.SourceAccountID.String(), "actor_kind": string(it.ActorKind), "known_seq": b.seq}
	if person != nil {
		details["person_id"] = person.String()
	}
	err = b.u.auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: org, OccurredAt: now, ActorKind: entity.ActorUser, ActorID: b.s.UserID, Operation: _operationAssign,
		ObjectKind: "account_assignment", ObjectID: id, Outcome: entity.AuditSucceeded, BaseVersion: base, NewVersion: id.String(),
		CorrelationID: entity.ID(logger.CorrelationID(ctx)), Details: details,
	})
	out.Outcome, out.AssignmentID = entity.AssignmentSaved, &id
	return out, err
}

// UserSourceAccount is the account of a user of craftPulse in an instance of a source by the confirmed link
// of the user's person; false without one. access checks rights in the source with it.
func (u *UseCase) UserSourceAccount(ctx context.Context, orgID, userID, instanceID entity.ID) (entity.UserAccount, bool, error) {
	var (
		out   entity.UserAccount
		found bool
	)
	err := u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, found, err = u.orgs.UserSourceAccount(ctx, orgID, userID, instanceID)
		return err
	})
	return out, found, err
}

// AssignmentAt is the link of an account that holds for the slice seq; calc reads links at its slice.
func (u *UseCase) AssignmentAt(ctx context.Context, orgID, accountID entity.ID, seq int64) (entity.AccountAssignment, bool, error) {
	var (
		out   entity.AccountAssignment
		found bool
	)
	err := u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, found, err = u.orgs.AssignmentAt(ctx, orgID, accountID, seq)
		return err
	})
	return out, found, err
}
