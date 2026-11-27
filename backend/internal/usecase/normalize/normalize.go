// Package normalize turns accepted records of intake into canonical facts: identities of external objects by
// BR-001, versions of knowledge of tasks, their history and dictionaries, and the current area of every
// object that access decides by (TZ 10.3, 12.4-12.6).
package normalize

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _partSize bounds the records of one transaction, so the knowledge counter is held for a fraction of a
// second (TZ 12.6).
const _partSize = 200

// Ingest is the ingest module: a receipt with its accepted records, and the records normalize could not read.
type Ingest interface {
	RecordCIResult(context.Context, entity.ID, entity.ID, *entity.CIResultCoordinates, bool) error
	Accepted(ctx context.Context, orgID, receiptID entity.ID) (entity.Receipt, []entity.IngestRecord, error)
	RejectRecords(ctx context.Context, orgID, receiptID entity.ID, reasons map[int]string) error
}

// Accounts is org: source accounts, the confirmed project areas and the components of a repository (TZ 10.3,
// rules 1 and 7).
type Accounts interface {
	ProjectAreas(context.Context, entity.ID, time.Time) ([]entity.Link, error)
	RepositoryComponents(ctx context.Context, orgID, areaID entity.ID) ([]entity.Component, error)
	EnsureSourceAccounts(ctx context.Context, orgID, instanceID entity.ID, accounts []entity.SourceAccount) (map[string]entity.ID, error)
	AssignmentAt(ctx context.Context, orgID, accountID entity.ID, seq int64) (entity.AccountAssignment, bool, error)
}

type ReportAreas interface {
	LockStructureVersion(context.Context, entity.ID) (int64, error)
	LinkReportArea(ctx context.Context, orgID, projectID, areaID entity.ID, version, seq int64) error
}

// Complete ends the job in the transaction of its part.
type Complete = func(ctx context.Context) error

type Deps struct {
	SonarRepository interface {
		SonarRepository(ctx context.Context, org, scope entity.ID) (*entity.ID, error)
	}
	SummaryTargets interface {
		SummaryTargets(context.Context, entity.ID, int64) ([]entity.ID, error)
	}
	Repo        repo.NormalizeRepo
	Tenants     repo.TenantRepo
	Ingest      Ingest
	Accounts    Accounts
	ReportAreas ReportAreas
	// Links records normalized relationships; Rights is needed only for decideLink.
	Links   Links
	Rights  Rights
	Objects repo.ObjectStore
	Queue   repo.JobQueue
	Tx      repo.Transactor
	Clock   clock.Clock
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
	if len(part) > 0 || job.ScanManifest {
		if in, err = u.prepare(ctx, rc, part, via(job, rc), job.AfterNo == 0, !more); err != nil {
			return err
		}
	}
	reports := job.Reports || in != nil && len(in.reports) > 0
	codeReports := job.CodeReports || in != nil && hasCodeReports(in.reports)
	docsReports := job.DocsReports || in != nil && slices.ContainsFunc(in.reports, func(r *reportCandidate) bool { return r.docs != nil })
	securityReports := job.SecurityReports || in != nil && slices.ContainsFunc(in.reports, func(c *reportCandidate) bool {
		return c.parser != nil && c.report.SHA != "" && securityReportKind(c.report.Kind)
	})
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		areas := job.Areas
		if in != nil {
			var structureVersion *int64
			if in.hasImportedReport() {
				version, err := u.d.ReportAreas.LockStructureVersion(ctx, org)
				if err != nil {
					return err
				}
				structureVersion = &version
			}
			if err := u.d.Links.LockWrites(ctx, org); err != nil {
				return err
			}
			seq, err := u.d.Tenants.NextSeq(ctx, org)
			if err != nil {
				return err
			}
			changed, err := u.write(ctx, org, seq, in, job.ResolvedNo, structureVersion)
			if err != nil {
				return err
			}
			areas = union(areas, changed)
		}
		analysisReports := job.AnalysisReports || in != nil && (len(in.analysis) > 0 || in.sonarRuns)
		if more {
			next := job
			next.AfterNo, next.Areas, next.Reports = part[len(part)-1].No, areas, reports
			next.CodeReports, next.DocsReports = codeReports, docsReports
			next.SecurityReports, next.AnalysisReports = securityReports, analysisReports
			if err := u.d.Queue.Enqueue(ctx, next, queueOf(rc)); err != nil {
				return err
			}
			return complete(ctx)
		}
		if rc.Channel == entity.ChannelCI {
			if err := u.d.Ingest.RecordCIResult(ctx, org, rc.ID, nil, false); err != nil {
				return err
			}
		}
		if err := u.recount(ctx, org, areas, job.CorrelationID); err != nil {
			return err
		}
		if codeReports {
			recount := entity.CalcRecountJob{OrganizationID: org, CodeReceiptID: job.ReceiptID, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, recount, entity.QueueBulk); err != nil {
				return err
			}
		}
		if docsReports {
			recount := entity.CalcRecountJob{OrganizationID: org, DocsReceiptID: job.ReceiptID, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, recount, entity.QueueBulk); err != nil {
				return err
			}
		}
		if securityReports {
			recount := entity.CalcRecountJob{OrganizationID: org, SecurityReceiptID: job.ReceiptID, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, recount, entity.QueueBulk); err != nil {
				return err
			}
		}
		if analysisReports {
			recount := entity.CalcRecountJob{OrganizationID: org, AnalysisReceiptID: job.ReceiptID, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, recount, entity.QueueBulk); err != nil {
				return err
			}
		}
		if reports {
			identify := entity.ChangeIdentifyJob{OrganizationID: org, ReceiptID: job.ReceiptID, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, identify, queueOf(rc)); err != nil {
				return err
			}
		}
		return complete(ctx)
	})
}

// recount asks calc to recount the areas the receipt changed once its last part is written (TZ 10.3, rule 6).
func (u *UseCase) recount(ctx context.Context, org entity.ID, areas []entity.ID, corr entity.ID) error {
	for _, area := range areas {
		job := entity.CalcRecountJob{OrganizationID: org, AreaID: area, CorrelationID: corr}
		if err := u.d.Queue.Enqueue(ctx, job, entity.QueueBulk); err != nil {
			return err
		}
	}
	return nil
}

// union adds the areas missing from a, in their order.
func union(a, b []entity.ID) []entity.ID {
	out := slices.Clone(a)
	for _, id := range b {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
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

// prepare reads a part; the first part also reads the execution manifest of the package.
func (u *UseCase) prepare(ctx context.Context, rc entity.Receipt, part []entity.IngestRecord, via entity.ObservedVia, first, last bool) (*input, error) {
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
	in, err := prepare(rc, pkg, part, via, first)
	if err != nil {
		return nil, err
	}
	if last {
		in.analysis, err = analysisCandidates(pkg, rc.RecordCount)
		if err != nil {
			return nil, err
		}
	}
	return in, u.readReports(ctx, rc, in)
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
