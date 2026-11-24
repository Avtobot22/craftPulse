// Package decision stores exceptions and their immutable versions in PostgreSQL.
package decision

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// Repo reads and writes exception records under row-level security.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo constructs a repository using the caller's organization transaction.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

// LockConfirmation serializes requests with the same tenant and idempotency key.
func (r *Repo) LockConfirmation(ctx context.Context, org, key entity.ID) error {
	return wrap("lock confirmation", r.q(ctx).LockConfirmation(ctx, LockConfirmationParams{
		OrganizationID: uuid.UUID(org), ConfirmationID: uuid.UUID(key),
	}))
}

// InsertDecision starts a new exception.
func (r *Repo) InsertDecision(ctx context.Context, org, actor entity.ID, at time.Time) (entity.ID, error) {
	id, err := r.q(ctx).InsertDecision(ctx, InsertDecisionParams{OrganizationID: uuid.UUID(org), CreatedBy: uuid.UUID(actor), CreatedAt: at})
	return entity.ID(id), wrap("insert decision", err)
}

// Decision returns a full history with the root optionally locked until the transaction commits.
func (r *Repo) Decision(ctx context.Context, org, id entity.ID, lock bool) (entity.ExceptionRecord, error) {
	var root DecisionDecision
	var err error
	if lock {
		root, err = r.q(ctx).LockDecision(ctx, LockDecisionParams{OrganizationID: uuid.UUID(org), ID: uuid.UUID(id)})
	} else {
		root, err = r.q(ctx).DecisionByID(ctx, DecisionByIDParams{OrganizationID: uuid.UUID(org), ID: uuid.UUID(id)})
	}
	if err != nil {
		return entity.ExceptionRecord{}, wrap("load decision", err)
	}
	records, err := r.Histories(ctx, org, []entity.ID{id}, math.MaxInt64)
	if err != nil {
		return entity.ExceptionRecord{}, err
	}
	out := *records[id]
	out.CreatedAt = root.CreatedAt.UTC()
	if root.WakeAt.Valid {
		at := root.WakeAt.Time.UTC()
		out.WakeAt = &at
	}
	return out, nil
}

// SetWake records the expected next effective boundary.
func (r *Repo) SetWake(ctx context.Context, org, id entity.ID, at *time.Time) error {
	var stamp pgtype.Timestamptz
	if at != nil {
		stamp = pgtype.Timestamptz{Time: *at, Valid: true}
	}
	return wrap("set wake", r.q(ctx).SetWake(ctx, SetWakeParams{OrganizationID: uuid.UUID(org), ID: uuid.UUID(id), WakeAt: stamp}))
}

// VersionByConfirmation finds the version written by an idempotent request.
func (r *Repo) VersionByConfirmation(ctx context.Context, org, key entity.ID) (entity.ExceptionVersion, error) {
	v, err := r.q(ctx).VersionByConfirmation(ctx, VersionByConfirmationParams{OrganizationID: uuid.UUID(org), ConfirmationID: uuid.UUID(key)})
	return versionOf(&v), wrap("find confirmation", err)
}

// TransitionByConfirmation finds the transition and decision for an idempotent review.
func (r *Repo) TransitionByConfirmation(ctx context.Context, org, key entity.ID) (entity.ID, entity.ExceptionTransition, error) {
	row, err := r.q(ctx).TransitionByConfirmation(ctx, TransitionByConfirmationParams{
		OrganizationID: uuid.UUID(org), ConfirmationID: nullID(key),
	})
	return entity.ID(row.DecisionID), transitionOf((*TransitionsRow)(&row)), wrap("find transition confirmation", err)
}

// InsertVersion writes the scoped draft and returns its version ID.
func (r *Repo) InsertVersion(ctx context.Context, org entity.ID, v *entity.ExceptionVersion) (entity.ID, error) {
	if v.Number > math.MaxInt32 || v.Base > math.MaxInt32 || v.Number < 1 || v.Base < 0 {
		return entity.ID{}, entity.ErrVersionConflict
	}
	base := pgtype.Int4{Int32: int32(v.Base), Valid: v.Base > 0}
	s := &v.Scope
	id, err := r.q(ctx).InsertVersion(ctx, InsertVersionParams{
		OrganizationID: uuid.UUID(org), DecisionID: uuid.UUID(v.DecisionID), VersionNo: int32(v.Number), BaseVersionNo: base,
		DecisionType: v.Type, Reason: v.Reason, AreaID: uuid.UUID(s.AreaID), ComponentID: nullID(s.ComponentID),
		FindingIdentityID: nullID(s.FindingID), FindingKind: v.FindingKind, RuleID: nullText(s.RuleID), PathPrefix: nullText(s.PathPrefix),
		PackagePurl: nullText(s.PackagePURL), EffectiveFrom: v.EffectiveFrom, ExpiresAt: v.ExpiresAt, ApprovalRequired: v.ApprovalRequired,
		AuthorUserID: uuid.UUID(v.AuthorID), CreatedAt: v.CreatedAt, KnownSeq: v.Seq, ConfirmationID: uuid.UUID(v.Key),
	})
	return entity.ID(id), wrap("insert version", err)
}

// InsertTransition appends the status at its knowledge number and event time.
func (r *Repo) InsertTransition(ctx context.Context, org entity.ID, t *entity.ExceptionTransition) error {
	return wrap("insert transition", r.q(ctx).InsertTransition(ctx, InsertTransitionParams{
		OrganizationID: uuid.UUID(org), DecisionVersionID: uuid.UUID(t.VersionID), Status: t.Status, ActorUserID: uuid.UUID(t.ActorID),
		OccurredAt: t.At, KnownSeq: t.Seq, ConfirmationID: nullID(t.Key), Comment: t.Comment,
	}))
}

// Histories reads visible-to-the-cut records in two batch queries.
func (r *Repo) Histories(ctx context.Context, org entity.ID, ids []entity.ID, seq int64) (map[entity.ID]*entity.ExceptionRecord, error) {
	out := make(map[entity.ID]*entity.ExceptionRecord, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	queryIDs := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		queryIDs[i] = uuid.UUID(id)
		out[id] = &entity.ExceptionRecord{ID: id}
	}
	q := r.q(ctx)
	rows, err := q.Versions(ctx, VersionsParams{OrganizationID: uuid.UUID(org), DecisionIds: queryIDs, KnownSeq: seq})
	if err != nil {
		return nil, wrap("load versions", err)
	}
	for i := range rows {
		row := &rows[i]
		rec := out[entity.ID(row.DecisionID)]
		rec.Versions = append(rec.Versions, versionOf(row))
	}
	transitions, err := q.Transitions(ctx, TransitionsParams{OrganizationID: uuid.UUID(org), DecisionIds: queryIDs, KnownSeq: seq})
	if err != nil {
		return nil, wrap("load transitions", err)
	}
	for i := range transitions {
		row := &transitions[i]
		rec := out[entity.ID(row.DecisionID)]
		rec.Transitions = append(rec.Transitions, transitionOf(row))
	}
	return out, nil
}

// AreaDecisionIDs selects area decisions with a started version at the cut.
func (r *Repo) AreaDecisionIDs(ctx context.Context, org, area entity.ID, seq int64, at time.Time, includeExpired bool) ([]entity.ID, error) {
	rows, err := r.q(ctx).AreaDecisionIDs(ctx, AreaDecisionIDsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), KnownSeq: seq, AsOf: at, IncludeExpired: includeExpired,
	})
	if err != nil {
		return nil, wrap("load area decisions", err)
	}
	out := make([]entity.ID, len(rows))
	for i, id := range rows {
		out[i] = entity.ID(id)
	}
	return out, nil
}

// PageDecisions selects stable sort boundaries over latest versions at a fixed knowledge cut.
func (r *Repo) PageDecisions(ctx context.Context, org entity.ID, q *entity.ExceptionQuery, cut *entity.ExceptionCursor, limit int32) ([]entity.ExceptionCursor, error) {
	params := PageDecisionsParams{
		OrganizationID: uuid.UUID(org), KnownSeq: cut.Seq, AsOf: cut.At, ByExpiry: strings.TrimPrefix(q.Sort, "-") != "created_at",
		Descending: strings.HasPrefix(q.Sort, "-"), ComponentID: nullID(q.ComponentID), FindingID: nullID(q.FindingID), PageSize: limit,
	}
	if !cut.ID.IsZero() {
		params.AfterID, params.AfterSortAt = nullID(cut.ID), pgtype.Timestamptz{Time: cut.SortAt, Valid: true}
	}
	rows, err := r.q(ctx).PageDecisions(ctx, params)
	if err != nil {
		return nil, wrap("page decisions", err)
	}
	out := make([]entity.ExceptionCursor, len(rows))
	for i, row := range rows {
		out[i] = entity.ExceptionCursor{ID: entity.ID(row.ID), SortAt: row.SortAt.UTC(), At: cut.At, Seq: cut.Seq}
	}
	return out, nil
}

func versionOf(v *DecisionDecisionVersion) entity.ExceptionVersion {
	return entity.ExceptionVersion{
		ID: entity.ID(v.ID), DecisionID: entity.ID(v.DecisionID), AuthorID: entity.ID(v.AuthorUserID), Key: entity.ID(v.ConfirmationID),
		Number: int(v.VersionNo), Base: int(v.BaseVersionNo.Int32), Seq: v.KnownSeq, FindingKind: v.FindingKind,
		ApprovalRequired: v.ApprovalRequired, CreatedAt: v.CreatedAt.UTC(),
		ExceptionDraft: entity.ExceptionDraft{
			Type: v.DecisionType, Reason: v.Reason, EffectiveFrom: v.EffectiveFrom.UTC(), ExpiresAt: v.ExpiresAt.UTC(),
			Scope: entity.ExceptionScope{
				AreaID: entity.ID(v.AreaID), ComponentID: entity.ID(v.ComponentID.UUID), FindingID: entity.ID(v.FindingIdentityID.UUID),
				RuleID: v.RuleID.String, PathPrefix: v.PathPrefix.String, PackagePURL: v.PackagePurl.String,
			},
		},
	}
}

func transitionOf(t *TransitionsRow) entity.ExceptionTransition {
	return entity.ExceptionTransition{
		VersionID: entity.ID(t.DecisionVersionID), ActorID: entity.ID(t.ActorUserID), Key: entity.ID(t.ConfirmationID.UUID),
		Version: int(t.VersionNo), Seq: t.KnownSeq, Status: t.Status, Comment: t.Comment, At: t.OccurredAt.UTC(),
	}
}

func nullID(id entity.ID) uuid.NullUUID {
	return uuid.NullUUID{UUID: uuid.UUID(id), Valid: !id.IsZero()}
}
func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func wrap(operation string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return entity.ErrNotFound
	default:
		return fmt.Errorf("decision: %s: %w", operation, err)
	}
}
