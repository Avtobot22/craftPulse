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
	Publication(context.Context, entity.ID, entity.ID, entity.ID) (entity.Publication, error)
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
	if err != nil || mr.HeadSHA != s.Revision.HeadSHA || s.Revision.Status == "stale" {
		return false, err
	}
	source, err := u.summaryAreaSource(ctx, org, mr)
	if err != nil {
		return false, err
	}
	if source.ConnectionID != s.ConnectionID {
		return false, errPublicationConnectionChanged
	}
	head, err := u.readPublicationHead(ctx, org, source, mr.Source)
	return head == s.Revision.HeadSHA, err
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

// Publication reads the outcome for a revision after the caller checks access to its MR.
func (u *UseCase) Publication(ctx context.Context, org, mr, revision entity.ID) (entity.Publication, error) {
	var p entity.Publication
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		p, err = u.d.Summary.Publication.Publication(ctx, org, mr, revision)
		return err
	})
	return p, err
}
