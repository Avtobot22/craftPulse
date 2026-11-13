package org

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var errPageLimit = errors.New("org: page limit is out of range")

func rowLimit(limit int) (int32, error) {
	if limit < 1 || limit > math.MaxInt32 {
		return 0, errPageLimit
	}
	return int32(limit), nil
}

// PagePersons reads people by ID after the cursor; the caller asks one more than the page to see the next one.
func (r *Repo) PagePersons(ctx context.Context, orgID entity.ID, f entity.PersonFilter) ([]entity.Person, error) {
	limit, err := rowLimit(f.Limit)
	if err != nil {
		return nil, err
	}
	p := PagePersonsParams{OrganizationID: uuid.UUID(orgID), Kind: text(string(f.Kind)), RowLimit: limit}
	if f.After != nil {
		p.After, p.AfterID = true, uuid.UUID(*f.After)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PagePersons(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("org: page persons: %w", err)
	}
	return convert(rows, func(x PagePersonsRow) entity.Person {
		return entity.Person{ID: entity.ID(x.ID), DisplayName: x.DisplayName.String, Kind: entity.PersonKind(x.Kind)}
	}), nil
}

// CreatePerson adds a person an account is linked to when the administrator creates one with the link.
func (r *Repo) CreatePerson(ctx context.Context, orgID entity.ID, name string, kind entity.PersonKind, at time.Time) (entity.ID, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).InsertPerson(ctx, InsertPersonParams{
		OrganizationID: uuid.UUID(orgID), DisplayName: text(name), Kind: string(kind), CreatedAt: at,
	})
	if err != nil {
		return entity.ID{}, fmt.Errorf("org: create person: %w", err)
	}
	return entity.ID(id), nil
}

// PageSourceAccounts reads accounts with their current link by ID after the cursor.
func (r *Repo) PageSourceAccounts(ctx context.Context, orgID entity.ID, f entity.SourceAccountFilter) ([]entity.AccountView, error) {
	limit, err := rowLimit(f.Limit)
	if err != nil {
		return nil, err
	}
	p := PageSourceAccountsParams{OrganizationID: uuid.UUID(orgID), RowLimit: limit}
	if f.InstanceID != nil {
		p.InstanceID = nullID(*f.InstanceID)
	}
	if f.Assigned != "" {
		p.Assigned = pgtype.Bool{Bool: f.Assigned == entity.AccountsAssigned, Valid: true}
	}
	if f.After != nil {
		p.After, p.AfterID = true, uuid.UUID(*f.After)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageSourceAccounts(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("org: page source accounts: %w", err)
	}
	return convert(rows, func(x PageSourceAccountsRow) entity.AccountView {
		v := accountView(x.ID, x.InstanceID, x.NativeAccountID, x.Login, x.DisplayName, x.Email, x.ProposedKind)
		if x.AssignmentID.Valid {
			v.Assignment = &entity.AccountAssignment{
				ID: entity.ID(x.AssignmentID.UUID), SourceAccountID: v.ID, PersonID: idPtr(x.PersonID), ActorKind: entity.PersonKind(x.ActorKind.String),
				Knowledge: entity.Knowledge{KnownSeq: x.KnownSeq.Int64}, ConfirmedBy: entity.ID(x.ConfirmedBy.UUID), ConfirmedAt: x.ConfirmedAt.Time,
				ConfirmationID: entity.ID(x.ConfirmationID.UUID),
			}
		}
		return v
	}), nil
}

// AccountProposals are the people whose other accounts share the login or the email of an account.
func (r *Repo) AccountProposals(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.AccountProposal, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).AccountProposals(ctx, AccountProposalsParams{OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(ids)})
	if err != nil {
		return nil, fmt.Errorf("org: account proposals: %w", err)
	}
	out := make(map[entity.ID][]entity.AccountProposal, len(rows))
	for _, x := range rows {
		id := entity.ID(x.SourceAccountID)
		out[id] = append(out[id], entity.AccountProposal{PersonID: entity.ID(x.PersonID), Basis: x.Basis})
	}
	return out, nil
}

func (r *Repo) SourceAccountsByID(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID]entity.AccountView, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).SourceAccountsByID(ctx, SourceAccountsByIDParams{OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(ids)})
	if err != nil {
		return nil, fmt.Errorf("org: source accounts: %w", err)
	}
	out := make(map[entity.ID]entity.AccountView, len(rows))
	for i := range rows {
		x := &rows[i]
		out[entity.ID(x.ID)] = accountView(x.ID, x.InstanceID, x.NativeAccountID, x.Login, x.DisplayName, x.Email, x.ProposedKind)
	}
	return out, nil
}

// LockCurrentAssignments locks the current links of accounts by ascending ID and returns them by account.
func (r *Repo) LockCurrentAssignments(ctx context.Context, orgID entity.ID, accountIDs []entity.ID) (map[entity.ID]entity.AccountAssignment, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).LockCurrentAssignments(ctx, LockCurrentAssignmentsParams{
		OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(accountIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("org: lock assignments: %w", err)
	}
	out := make(map[entity.ID]entity.AccountAssignment, len(rows))
	for i := range rows {
		a := assignment(&rows[i])
		out[a.SourceAccountID] = a
	}
	return out, nil
}

// AssignmentsByConfirmation finds the versions saved under confirmation IDs, by confirmation ID.
func (r *Repo) AssignmentsByConfirmation(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID]entity.AccountAssignment, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).AssignmentsByConfirmation(ctx, AssignmentsByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(ids),
	})
	if err != nil {
		return nil, fmt.Errorf("org: assignments by confirmation: %w", err)
	}
	out := make(map[entity.ID]entity.AccountAssignment, len(rows))
	for i := range rows {
		a := assignment(&rows[i])
		out[a.ConfirmationID] = a
	}
	return out, nil
}

func (r *Repo) SupersedeAssignments(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	err := New(postgres.Conn(ctx, r.pool)).SupersedeAssignments(ctx, SupersedeAssignmentsParams{
		KnownSeq: seq, At: at, OrganizationID: uuid.UUID(orgID), Ids: toUUIDs(ids),
	})
	if err != nil {
		return fmt.Errorf("org: supersede assignments: %w", err)
	}
	return nil
}

func (r *Repo) InsertAssignment(ctx context.Context, orgID entity.ID, a entity.AccountAssignment) (entity.ID, error) {
	p := InsertAssignmentParams{
		OrganizationID: uuid.UUID(orgID), SourceAccountID: uuid.UUID(a.SourceAccountID), ActorKind: string(a.ActorKind), KnownSeq: a.KnownSeq,
		ConfirmedBy: uuid.UUID(a.ConfirmedBy), ConfirmedAt: a.ConfirmedAt, ConfirmationID: uuid.UUID(a.ConfirmationID),
	}
	if a.PersonID != nil {
		p.PersonID = nullID(*a.PersonID)
	}
	id, err := New(postgres.Conn(ctx, r.pool)).InsertAssignment(ctx, p)
	if err != nil {
		return entity.ID{}, fmt.Errorf("org: insert assignment: %w", err)
	}
	return entity.ID(id), nil
}

// AssignmentAt is the link of an account that holds for the slice seq.
func (r *Repo) AssignmentAt(ctx context.Context, orgID, accountID entity.ID, seq int64) (entity.AccountAssignment, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).AssignmentAt(ctx, AssignmentAtParams{
		OrganizationID: uuid.UUID(orgID), SourceAccountID: uuid.UUID(accountID), KnownSeq: seq,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.AccountAssignment{}, false, nil
	}
	if err != nil {
		return entity.AccountAssignment{}, false, fmt.Errorf("org: assignment at: %w", err)
	}
	return assignment(&row), true, nil
}

// UserSourceAccount is the account of a user in an instance by the confirmed link of the user's person.
func (r *Repo) UserSourceAccount(ctx context.Context, orgID, userID, instanceID entity.ID) (entity.UserAccount, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).UserSourceAccount(ctx, UserSourceAccountParams{
		OrganizationID: uuid.UUID(orgID), UserID: uuid.UUID(userID), InstanceID: uuid.UUID(instanceID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.UserAccount{}, false, nil
	}
	if err != nil {
		return entity.UserAccount{}, false, fmt.Errorf("org: user source account: %w", err)
	}
	return entity.UserAccount{SourceAccountID: entity.ID(row.ID), NativeID: row.NativeAccountID}, true, nil
}

func accountView(id, instance uuid.UUID, native string, login, name, email pgtype.Text, kind string) entity.AccountView {
	return entity.AccountView{
		ID: entity.ID(id), InstanceID: entity.ID(instance),
		Account: entity.SourceAccount{NativeID: native, Login: login.String, DisplayName: name.String, Email: email.String, Kind: entity.PersonKind(kind)},
	}
}

func assignment(row *OrgAccountAssignment) entity.AccountAssignment {
	k := entity.Knowledge{KnownSeq: row.KnownSeq}
	if row.SupersededSeq.Valid {
		k.SupersededSeq = &row.SupersededSeq.Int64
	}
	if row.SupersededAt.Valid {
		k.SupersededAt = &row.SupersededAt.Time
	}
	return entity.AccountAssignment{
		ID: entity.ID(row.ID), SourceAccountID: entity.ID(row.SourceAccountID), PersonID: idPtr(row.PersonID), ActorKind: entity.PersonKind(row.ActorKind),
		Knowledge: k, ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID),
	}
}

func idPtr(id uuid.NullUUID) *entity.ID {
	if !id.Valid {
		return nil
	}
	out := entity.ID(id.UUID)
	return &out
}
