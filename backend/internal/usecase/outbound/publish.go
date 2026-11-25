// Package outbound coordinates writes to external systems.
package outbound

import (
	"context"
	"errors"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Changes resolves the latest summary, verifies its head and stores the outcome.
type Changes interface {
	PublicationSummary(context.Context, entity.ID, entity.ID) (entity.SummaryPublication, error)
	CheckPublicationHead(context.Context, entity.ID, *entity.SummaryPublication) (bool, error)
	FinishPublication(context.Context, entity.ID, entity.ID, entity.ID, entity.Publication, func(context.Context) error) error
}

// Adapter resolves publication operations through the MR connection.
type Adapter interface {
	FindPublished(context.Context, entity.ID, entity.ID, entity.PublicationTarget) (string, error)
	Publish(context.Context, entity.ID, entity.ID, entity.PublishInput) (entity.Publication, error)
}

// Store records publication targets and write attempts.
type Store interface {
	Start(context.Context, entity.ID, entity.ID, entity.ID, entity.PublicationTarget, time.Time) (entity.ID, error)
	Finish(context.Context, entity.ID, entity.ID, entity.ID, entity.ID, entity.PublicationTarget, entity.Publication, time.Time) error
}

// Auditor records attempts and outcomes in the organization journal.
type Auditor interface {
	Record(context.Context, entity.AuditEvent) error
}

// Transactor commits the journal and summary outcome together.
type Transactor interface {
	InOrgTx(context.Context, entity.ID, func(context.Context) error) error
}

// Deps supplies outbound with its module interfaces.
type Deps struct {
	Changes Changes
	Adapter Adapter
	Store   Store
	Audit   Auditor
	Tx      Transactor
	Clock   clock.Clock
	SiteURL string
	Observe func(kind, status string, latency *time.Duration)
}

// UseCase publishes MR summaries.
type UseCase struct{ d Deps }

// New constructs the outbound use case.
func New(d Deps) *UseCase { return &UseCase{d: d} }

var errUnknown = errors.New("outbound: publication outcome unknown")

// RunPublish delivers the latest revision available when the job starts.
func (u *UseCase) RunPublish(ctx context.Context, job entity.OutboundPublishJob, complete func(context.Context) error, finalAttempt bool) error {
	org, mr := job.OrganizationID, job.MRID
	s, err := u.d.Changes.PublicationSummary(ctx, org, mr)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return err
	}
	if errors.Is(err, entity.ErrNotFound) || s.Revision.PipelineState == "running" ||
		s.Revision.Publication.Status == entity.PublicationSucceeded || s.Revision.Publication.Status == entity.PublicationStale {
		return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			return u.d.Changes.FinishPublication(ctx, org, mr, s.Revision.ID, entity.Publication{}, complete)
		})
	}

	target := entity.PublicationTarget{Kind: entity.MRComment, Marker: mr.String(), Object: s.MR.Source}
	var action entity.ID
	if !s.ConnectionID.IsZero() {
		err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			var err error
			action, err = u.d.Store.Start(ctx, org, s.ConnectionID, s.Revision.ID, target, u.d.Clock.Now())
			if err != nil {
				return err
			}
			return u.d.Audit.Record(ctx, entity.AuditEvent{
				OrganizationID: org, ActorKind: entity.ActorSystem, Operation: "outbound.publish.intent",
				ObjectKind: target.Kind, ObjectID: mr, AreaID: s.MR.AreaID, Outcome: entity.AuditSucceeded,
				Details: map[string]any{"action_id": action, "revision_id": s.Revision.ID},
			})
		})
		if err != nil {
			return err
		}
	}

	outcome, retry := u.publish(ctx, org, &s, target)
	if ae, ok := errors.AsType[*entity.AdapterError](retry); ok && ae.RetryAt != nil &&
		(ae.Class == entity.CallPaused || isSourceError(retry, entity.SourceRateLimited)) {
		retry = &entity.RetryLaterError{At: *ae.RetryAt, Err: retry}
	}
	now := u.d.Clock.Now()
	if outcome.Status == entity.PublicationSucceeded {
		outcome.PublishedAt = &now
	}
	if _, snooze := errors.AsType[*entity.RetryLaterError](retry); finalAttempt && !snooze {
		retry = nil
	}
	if retry != nil {
		complete = nil
	}
	// Persist the result even if the call used up the worker's deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if !action.IsZero() {
			if err := u.d.Store.Finish(ctx, org, action, s.ConnectionID, s.Revision.ID, target, outcome, now); err != nil {
				return err
			}
		}
		auditOutcome := entity.AuditFailed
		switch {
		case outcome.Status == entity.PublicationSucceeded:
			auditOutcome = entity.AuditSucceeded
		case outcome.ErrorCode == entity.WritePermissionMissing:
			auditOutcome = entity.AuditDenied
		}
		if err := u.d.Audit.Record(ctx, entity.AuditEvent{
			OrganizationID: org, ActorKind: entity.ActorSystem, Operation: "outbound.publish",
			ObjectKind: target.Kind, ObjectID: mr, AreaID: s.MR.AreaID, Outcome: auditOutcome, ReasonCode: outcome.ErrorCode,
			Details: map[string]any{"action_id": action, "revision_id": s.Revision.ID, "status": outcome.Status},
		}); err != nil {
			return err
		}
		return u.d.Changes.FinishPublication(ctx, org, mr, s.Revision.ID, outcome, complete)
	})
	if err != nil {
		return err
	}
	if u.d.Observe != nil {
		var latency *time.Duration
		if outcome.PublishedAt != nil && !s.Revision.ReportsReadyAt.IsZero() {
			d := max(0, now.Sub(s.Revision.ReportsReadyAt))
			latency = &d
		}
		u.d.Observe(target.Kind, outcome.Status, latency)
	}
	return retry
}

func (u *UseCase) publish(ctx context.Context, org entity.ID, s *entity.SummaryPublication, target entity.PublicationTarget) (entity.Publication, error) {
	stale := entity.Publication{Status: entity.PublicationStale, ErrorCode: entity.PublicationHeadChanged}
	failed := entity.Publication{Status: entity.PublicationFailed, ErrorCode: entity.PublicationProviderError}
	denied := entity.Publication{Status: entity.PublicationFailed, ErrorCode: entity.WritePermissionMissing}
	unknown := entity.Publication{Status: entity.PublicationUnknown, ErrorCode: entity.PublicationProviderError}

	if s.Revision.Status == "stale" || s.MR.HeadSHA != s.Revision.HeadSHA {
		return stale, nil
	}
	if s.ConnectionID.IsZero() {
		return failed, nil
	}

	markdown := render(u.d.SiteURL, &s.Revision)
	externalID, err := u.d.Adapter.FindPublished(ctx, org, s.ConnectionID, target)
	if err != nil {
		if isSourceError(err, entity.SourceAuth) {
			return denied, nil
		}
		return failed, err
	}
	current, err := u.d.Changes.CheckPublicationHead(ctx, org, s)
	if err != nil {
		return failed, err
	}
	if !current {
		return stale, nil
	}
	result, err := u.d.Adapter.Publish(ctx, org, s.ConnectionID, entity.PublishInput{
		PublicationTarget: target, ExternalID: externalID, HeadSHA: s.Revision.HeadSHA, Markdown: markdown,
	})
	switch {
	case isSourceError(err, "head_conflict"):
		return stale, nil
	case isSourceError(err, entity.SourceAuth):
		return denied, nil
	case isSourceError(err, entity.SourceRateLimited), errors.Is(err, entity.ErrConnectionPaused):
		return failed, err
	case err != nil:
		return unknown, errors.Join(errUnknown, err)
	case result.Status == entity.PublicationUnknown:
		return unknown, errUnknown
	default:
		return result, nil
	}
}

func isSourceError(err error, code string) bool {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	return ok && ae.Class == entity.CallSource && ae.Code == code
}
