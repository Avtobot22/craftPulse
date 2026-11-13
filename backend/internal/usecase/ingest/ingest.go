// Package ingest is the one intake of canonical packages from sync, CI and the universal import: limits and
// safe unpacking, the envelope and the schemas, quarantine, the journal that tells a new fact from a
// replay and a conflict, and partial acceptance (TZ 10.3, 12.6).
package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// Connect is the connect module: the instance of a package and the scopes a connection collects.
type Connect interface {
	EnsureInstance(ctx context.Context, orgID entity.ID, provider entity.Provider, key string) (entity.SourceInstance, error)
	Scopes(ctx context.Context, orgID, connectionID entity.ID) ([]entity.ConnectionScope, error)
}

// Authorizer is the access module: the right to import into a project.
type Authorizer interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Metrics interface {
	PackageRejected(channel entity.ReceiptChannel, reason string)
}

type Deps struct {
	Repo    repo.IngestRepo
	Objects repo.ObjectStore
	Queue   repo.JobQueue
	Tx      repo.Transactor
	Connect Connect
	Access  Authorizer
	Auditor Auditor
	Metrics Metrics
	Clock   clock.Clock
	NewID   func() (entity.ID, error)
	// Limits are envelope.DefaultLimits when zero.
	Limits envelope.Limits
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	if d.Limits == (envelope.Limits{}) {
		d.Limits = envelope.DefaultLimits
	}
	return &UseCase{d: d}
}

func quarantineKey(orgID, receiptID entity.ID) string {
	return "quarantine/" + orgID.String() + "/" + receiptID.String() + ".tar.gz"
}

func packageKey(orgID, receiptID entity.ID) string {
	return "packages/" + orgID.String() + "/" + receiptID.String() + ".tar.gz"
}

// SubmitImport puts a package of the universal import into quarantine and checks it at once: a package
// that breaks a limit, a path rule or the envelope is refused, one equal to a stored package is a duplicate,
// and both lose their quarantine object; a valid one waits for the job ingest.package (TZ 10.5.2).
func (u *UseCase) SubmitImport(ctx context.Context, s entity.Subject, projectID entity.ID, pkg io.Reader) (entity.Receipt, error) {
	id, err := u.d.NewID()
	if err != nil {
		return entity.Receipt{}, err
	}
	user, project := s.UserID, projectID
	rc := entity.Receipt{
		ID: id, OrganizationID: s.OrganizationID, Channel: entity.ChannelImport, ProjectID: &project, RequestedBy: &user,
		ReceivedAt: u.d.Clock.Now(), QuarantineKey: quarantineKey(s.OrganizationID, id),
	}
	counted := &countingReader{r: io.LimitReader(pkg, u.d.Limits.MaxPackageBytes+1)}
	if err := u.d.Objects.Put(ctx, rc.QuarantineKey, counted); err != nil {
		return entity.Receipt{}, err
	}
	if counted.n > u.d.Limits.MaxPackageBytes {
		return u.refuse(ctx, s, rc, nil, entity.PackageLimitExceeded)
	}
	body, err := u.d.Objects.Get(ctx, rc.QuarantineKey)
	if err != nil {
		return entity.Receipt{}, err
	}
	res, err := envelope.Clean(body, u.d.Limits, io.Discard)
	_ = body.Close()
	if err != nil {
		return entity.Receipt{}, err
	}
	if res.Envelope != nil {
		return u.refuse(ctx, s, rc, res.Manifest, packageReason(res.Envelope.Code))
	}
	rc.Status, rc.RecordCount = entity.ReceiptQuarantined, len(res.Records)
	if err := describe(&rc, res.Manifest); err != nil {
		return entity.Receipt{}, err
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		first, dup, err := u.d.Repo.FirstReceiptOfHash(ctx, s.OrganizationID, res.ContentHash)
		if err != nil {
			return err
		}
		if dup {
			rc.Status, rc.DuplicateOf, rc.PackageHash = entity.ReceiptDuplicate, &first, res.ContentHash
			return u.saveSubmitted(ctx, s, rc)
		}
		if err := u.saveSubmitted(ctx, s, rc); err != nil {
			return err
		}
		job := entity.IngestPackageJob{OrganizationID: s.OrganizationID, ReceiptID: id, CorrelationID: entity.ID(logger.CorrelationID(ctx))}
		return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
	})
	if err != nil {
		return entity.Receipt{}, err
	}
	if rc.Status == entity.ReceiptDuplicate {
		return rc, u.d.Objects.Delete(ctx, quarantineKey(s.OrganizationID, id))
	}
	return rc, nil
}

func (u *UseCase) saveSubmitted(ctx context.Context, s entity.Subject, rc entity.Receipt) error {
	if rc.Status != entity.ReceiptQuarantined {
		rc.QuarantineKey = ""
	}
	if err := u.d.Repo.SaveReceipt(ctx, rc); err != nil {
		return err
	}
	return u.d.Auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID, OccurredAt: u.d.Clock.Now(), ActorKind: entity.ActorUser, ActorID: s.UserID,
		Operation: "createImport", ObjectKind: "receipt", ObjectID: rc.ID, AreaID: *rc.ProjectID, Outcome: entity.AuditSucceeded,
		ReasonCode: rc.ReasonCode, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
	})
}

// refuse deletes the quarantine object and records the refused package with its reason.
func (u *UseCase) refuse(ctx context.Context, s entity.Subject, rc entity.Receipt, m *envelope.Manifest, reason string) (entity.Receipt, error) {
	if err := u.d.Objects.Delete(ctx, rc.QuarantineKey); err != nil {
		return entity.Receipt{}, err
	}
	rc.Status, rc.ReasonCode, rc.QuarantineKey = entity.ReceiptRejected, reason, ""
	if err := describe(&rc, m); err != nil {
		return entity.Receipt{}, err
	}
	if err := u.d.Tx.InOrgTx(ctx, rc.OrganizationID, func(ctx context.Context) error { return u.saveSubmitted(ctx, s, rc) }); err != nil {
		return entity.Receipt{}, err
	}
	u.d.Metrics.PackageRejected(rc.Channel, reason)
	return rc, nil
}

// ProcessPackage is the job ingest.package. It cleans the quarantined package, stores the cleaned object
// under the ID of the receipt and writes the journal in one transaction; the quarantine object goes after
// the commit. The same input gives the same object and the same journal, so a repeat after an interrupted
// run gives what one run gives, and a repeat after the commit finds the receipt done.
func (u *UseCase) ProcessPackage(ctx context.Context, orgID, receiptID entity.ID) error {
	var rc entity.Receipt
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		rc, err = u.d.Repo.Receipt(ctx, orgID, receiptID)
		return err
	})
	if err != nil {
		return err
	}
	key := quarantineKey(orgID, receiptID)
	if rc.Status != entity.ReceiptQuarantined {
		return u.d.Objects.Delete(ctx, key)
	}
	body, err := u.d.Objects.Get(ctx, key)
	if errors.Is(err, entity.ErrNotFound) {
		body = io.NopCloser(&brokenReader{})
	} else if err != nil {
		return err
	}
	p, err := u.prepare(ctx, rc, body)
	_ = body.Close()
	if err != nil {
		return err
	}
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		locked, err := u.d.Repo.LockReceipt(ctx, orgID, receiptID)
		if err != nil || locked.Status != entity.ReceiptQuarantined {
			return err
		}
		_, err = u.record(ctx, p)
		return err
	})
	if err != nil {
		return err
	}
	return u.d.Objects.Delete(ctx, key)
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// brokenReader stands for a quarantine object that is gone: the package is refused as unreadable.
type brokenReader struct{}

func (*brokenReader) Read([]byte) (int, error) {
	return 0, errors.New("ingest: quarantine object is missing")
}

// packageReason maps a rejection of the envelope to the reason of the receipt.
func packageReason(code string) string {
	switch code {
	case envelope.ReasonLimitExceeded:
		return entity.PackageLimitExceeded
	case envelope.ReasonUnsafePath:
		return entity.PackagePathUnsafe
	case envelope.ReasonUnsupportedSchema, envelope.ReasonUnsupportedSchemaVersion:
		return entity.PackageSchemaUnsupported
	}
	return entity.PackageEnvelopeInvalid
}

func recordReason(code string) string {
	if code == envelope.ReasonMissingNativeID {
		return entity.RecordMissingSourceID
	}
	return code
}

func errorf(format string, args ...any) error { return fmt.Errorf("ingest: "+format, args...) }
