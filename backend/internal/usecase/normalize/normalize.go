// Package normalize turns accepted records of intake into canonical facts: identities of external objects by
// BR-001, versions of knowledge of tasks, their history and dictionaries, and the current area of every
// object that access decides by (TZ 10.3, 12.4-12.6).
package normalize

import (
	"context"
	"errors"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _partSize bounds the records of one transaction, so the knowledge counter is held for a fraction of a
// second (TZ 12.6).
const _partSize = 200

// Ingest is the ingest module: a receipt with its accepted records.
type Ingest interface {
	Accepted(ctx context.Context, orgID, receiptID entity.ID) (entity.Receipt, []entity.IngestRecord, error)
}

// Accounts is the org module, which owns the accounts of sources (TZ 10.3, rules 1 and 7).
type Accounts interface {
	EnsureSourceAccounts(ctx context.Context, orgID, instanceID entity.ID, accounts []entity.SourceAccount) (map[string]entity.ID, error)
}

// Complete ends the job in the transaction of its part.
type Complete = func(ctx context.Context) error

type Deps struct {
	Repo     repo.NormalizeRepo
	Tenants  repo.TenantRepo
	Ingest   Ingest
	Accounts Accounts
	Objects  repo.ObjectStore
	Queue    repo.JobQueue
	Tx       repo.Transactor
	Clock    clock.Clock
	// PartSize is _partSize when zero.
	PartSize int
	// Limits are envelope.DefaultLimits when zero.
	Limits envelope.Limits
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	if d.PartSize == 0 {
		d.PartSize = _partSize
	}
	if d.Limits == (envelope.Limits{}) {
		d.Limits = envelope.DefaultLimits
	}
	return &UseCase{d: d}
}

// RunBatch is the job normalize.batch: it normalizes the part of the accepted records after the cursor of
// the job. Parsing comes before the transaction; the transaction takes the next knowledge number first,
// writes the versions, queues the next part and ends the job, so an interrupted worker repeats only the
// part that did not commit.
func (u *UseCase) RunBatch(ctx context.Context, job entity.NormalizeBatchJob, complete Complete) error {
	org := job.OrganizationID
	rc, recs, err := u.d.Ingest.Accepted(ctx, org, job.ReceiptID)
	if errors.Is(err, entity.ErrNotFound) {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	if err != nil {
		return err
	}
	part, more := u.part(recs, job.AfterNo)
	var in *input
	if len(part) > 0 {
		if in, err = u.prepare(ctx, rc, part, via(job, rc)); err != nil {
			return err
		}
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if in != nil {
			seq, err := u.d.Tenants.NextSeq(ctx, org)
			if err != nil {
				return err
			}
			if err := u.write(ctx, org, seq, in, job.ResolvedNo); err != nil {
				return err
			}
		}
		if more {
			next := job
			next.AfterNo = part[len(part)-1].No
			if err := u.d.Queue.Enqueue(ctx, next, queueOf(rc)); err != nil {
				return err
			}
		}
		return complete(ctx)
	})
}

// part is the accepted records after the cursor, at most PartSize of them, and whether more follow.
func (u *UseCase) part(recs []entity.IngestRecord, after int) ([]entity.IngestRecord, bool) {
	start := len(recs)
	for i := range recs {
		if recs[i].No > after {
			start = i
			break
		}
	}
	rest := recs[start:]
	if len(rest) > u.d.PartSize {
		return rest[:u.d.PartSize], true
	}
	return rest, false
}

func (u *UseCase) prepare(ctx context.Context, rc entity.Receipt, part []entity.IngestRecord, via entity.ObservedVia) (*input, error) {
	if rc.ObjectKey == "" || rc.SourceInstanceID == nil {
		return nil, errorf("receipt %s has accepted records but no stored package", rc.ID)
	}
	body, err := u.d.Objects.Get(ctx, rc.ObjectKey)
	if err != nil {
		return nil, err
	}
	pkg, err := envelope.ReadCleaned(body, u.d.Limits)
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	return prepare(rc, pkg, part, via)
}

// via is how craftPulse learned of the events of a receipt: sync names its phase, the channel decides
// otherwise.
func via(job entity.NormalizeBatchJob, rc entity.Receipt) entity.ObservedVia {
	switch {
	case job.ObservedVia != "":
		return job.ObservedVia
	case rc.Channel == entity.ChannelImport:
		return entity.ViaImport
	}
	return entity.ViaIncremental
}

// queueOf keeps the parts of a receipt in the queue of its source: CI is interactive, the rest bulk (TZ 10.6).
func queueOf(rc entity.Receipt) string {
	if rc.Channel == entity.ChannelCI {
		return entity.QueueInteractive
	}
	return entity.QueueBulk
}
