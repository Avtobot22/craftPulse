package change

import (
	"context"
	"errors"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type PublicationStore interface {
	PublicationRevision(context.Context, entity.ID, entity.ID) (entity.SummaryRevision, error)
	PublicationItems(context.Context, entity.ID, entity.ID) ([]entity.SummaryItem, error)
	SavePublication(context.Context, entity.ID, entity.ID, entity.Publication) error
	SummaryRevision(context.Context, entity.ID, entity.ID, *int) (entity.SummaryRevision, error)
	SummaryItems(context.Context, entity.ID, entity.ID) ([]entity.SummaryItem, error)
}

func (u *UseCase) PublicationSummary(ctx context.Context, org, mr entity.ID) (entity.SummaryPublication, error) {
	var out entity.SummaryPublication
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out.MR, err = u.d.Summary.Facts.SummaryMR(ctx, org, mr, false)
		if err != nil {
			return err
		}
		out.Revision, err = u.d.Summary.Publication.PublicationRevision(ctx, org, mr)
		if err != nil || out.Revision.PipelineState == "running" || out.Revision.Publication.Status == entity.PublicationSucceeded {
			return err
		}
		out.Revision.Items, err = u.d.Summary.Publication.PublicationItems(ctx, org, out.Revision.ID)
		return err
	})
	if err != nil || out.Revision.PipelineState == "running" || out.Revision.Publication.Status == entity.PublicationSucceeded {
		return out, err
	}
	source, err := u.summaryAreaSource(ctx, org, out.MR)
	if errors.Is(err, entity.ErrNotFound) {
		return out, nil
	}
	out.ConnectionID = source.ConnectionID
	return out, err
}

func (u *UseCase) CheckPublicationHead(ctx context.Context, org entity.ID, s *entity.SummaryPublication) (bool, error) {
	var mr entity.SummaryMR
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		mr, err = u.d.Summary.Facts.SummaryMR(ctx, org, s.MR.MRID, false)
		return err
	})
	if errors.Is(err, entity.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if mr.HeadSHA != s.Revision.HeadSHA {
		return false, entity.ErrPublicationHeadChanged
	}
	if s.Revision.Status == "stale" {
		return false, nil
	}
	source, err := u.summaryAreaSource(ctx, org, mr)
	if err != nil {
		return false, err
	}
	if source.ConnectionID != s.ConnectionID {
		return false, errPublicationConnectionChanged
	}
	head, err := u.readPublicationHead(ctx, org, source, mr.Source)
	if err != nil {
		return false, err
	}
	if head != s.Revision.HeadSHA {
		return false, entity.ErrPublicationHeadChanged
	}
	var latest entity.SummaryRevision
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		latest, err = u.d.Summary.Publication.PublicationRevision(ctx, org, mr.MRID)
		return err
	})
	if errors.Is(err, entity.ErrNotFound) {
		return false, nil
	}
	return latest.ID == s.Revision.ID && latest.Status == "ready", err
}

var errPublicationConnectionChanged = errors.New("change: publication connection changed")

// FinishPublication runs inside the transaction that commits the outbound action and the River job.
func (u *UseCase) FinishPublication(ctx context.Context, org, mr, revision entity.ID, p entity.Publication,
	complete func(context.Context) error,
) error {
	// Serialize job completion with finishSummary, which persists a ready revision and enqueues in the same transaction.
	_, err := u.d.Summary.Facts.SummaryMR(ctx, org, mr, true)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return err
	}
	if !revision.IsZero() && p.Status != "" {
		if err := u.d.Summary.Publication.SavePublication(ctx, org, revision, p); err != nil {
			return err
		}
	}
	if complete == nil {
		return nil
	}
	if err := complete(ctx); err != nil {
		return err
	}
	latest, err := u.d.Summary.Publication.PublicationRevision(ctx, org, mr)
	if errors.Is(err, entity.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if latest.ID != revision && latest.Status == "ready" && latest.PipelineState != "running" &&
		latest.Publication.Status != entity.PublicationSucceeded {
		return u.d.Summary.Queue.Enqueue(ctx, entity.OutboundPublishJob{OrganizationID: org, MRID: mr, SummaryRevisionID: latest.ID}, entity.QueueInteractive)
	}
	return nil
}

// ReadSummary retrieves the selected immutable summary and its current publication outcome.
func (u *UseCase) ReadSummary(ctx context.Context, org, mr entity.ID, no *int) (entity.SummaryRevision, error) {
	var v entity.SummaryRevision
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		v, err = u.d.Summary.Publication.SummaryRevision(ctx, org, mr, no)
		if err != nil {
			return err
		}
		v.Items, err = u.d.Summary.Publication.SummaryItems(ctx, org, v.ID)
		return err
	})
	return v, err
}
