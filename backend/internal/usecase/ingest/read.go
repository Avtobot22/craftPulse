package ingest

import (
	"context"
	"errors"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// _affected are the metrics of the MVP catalog a data kind feeds (docs/product/metrics/mvp.md).
var _affected = map[string][]string{
	"tasks":          {"T04", "T05", "T07", "T08", "T10", "T11", "T12", "T13", "T14", "T15", "T18", "T23"},
	"task_events":    {"T04", "T05", "T07", "T08", "T10", "T11", "T12", "T13", "T14", "T15", "T18", "T23"},
	"relations":      {"T13", "T15", "A18", "A20", "T28"},
	"merge_requests": {"A05", "A06", "A07", "A10", "A18", "A20", "A22", "T28"},
	"ci_runs":        {"I01", "I02", "I03", "I04", "I05", "I06", "I07", "I08", "I09", "I19"},
	"junit":          {"I03", "I04", "I05", "I07", "I08", "I09"},
	"coverage":       {"I25", "I26"},
	"documents":      {"D01", "D03", "D10", "D24", "D30"},
}

// AffectedMetrics are the metrics whose data the schema of a receipt carries.
func AffectedMetrics(schemaID string) []string {
	return slices.Clone(_affected[schemaID])
}

// Receipt reads a receipt its subject may see: an administrator any, a tech lead the ones of projects
// where they may import, an upload token those of its own packages; another one is not found.
func (u *UseCase) Receipt(ctx context.Context, s entity.Subject, id entity.ID) (entity.Receipt, error) {
	var rc entity.Receipt
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		if rc, err = u.d.Repo.Receipt(ctx, s.OrganizationID, id); err != nil {
			return err
		}
		receipts := []entity.Receipt{rc}
		err = u.d.Repo.CountRepeats(ctx, s.OrganizationID, receipts)
		rc = receipts[0]
		return err
	})
	if err != nil {
		return entity.Receipt{}, err
	}
	if err := u.visible(ctx, s, rc); err != nil {
		return entity.Receipt{}, err
	}
	return rc, nil
}

func (u *UseCase) visible(ctx context.Context, s entity.Subject, rc entity.Receipt) error {
	if !s.UploadTokenID.IsZero() {
		if rc.UploadTokenID == nil || *rc.UploadTokenID != s.UploadTokenID {
			return entity.ErrNotFound
		}
		return nil
	}
	r := entity.AccessRequest{Operation: "getReceipt", Action: "sync.read"}
	if rc.ProjectID != nil {
		r.Action, r.Area = "receipts.read", entity.AreaRef{Kind: entity.AreaProject, ID: *rc.ProjectID}
	}
	_, err := u.d.Access.Authorize(ctx, s, r)
	if errors.Is(err, entity.ErrForbidden) {
		return entity.ErrNotFound
	}
	return err
}

// ListReceipts pages the journal of intake, newest first.
func (u *UseCase) ListReceipts(ctx context.Context, s entity.Subject, f entity.ReceiptFilter) (entity.ReceiptPage, error) {
	limit := f.Limit
	f.Limit++
	var out entity.ReceiptPage
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageReceipts(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			out.Next = &entity.AuditKey{OccurredAt: items[limit-1].ReceivedAt, ID: items[limit-1].ID}
		}
		out.Items = items
		return u.d.Repo.CountRepeats(ctx, s.OrganizationID, out.Items)
	})
	return out, err
}

// ListRecords pages the records of a receipt by number: their outcome and reason, never their content.
func (u *UseCase) ListRecords(ctx context.Context, s entity.Subject, receiptID entity.ID, f entity.RecordFilter) (entity.RecordPage, error) {
	if _, err := u.Receipt(ctx, s, receiptID); err != nil {
		return entity.RecordPage{}, err
	}
	limit := f.Limit
	f.Limit++
	var out entity.RecordPage
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageRecords(ctx, s.OrganizationID, receiptID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			out.NextNo = items[limit-1].No
		}
		out.Items = items
		return nil
	})
	return out, err
}

// ListConflicts pages the conflicts of intake, oldest first, with the identity of their records.
func (u *UseCase) ListConflicts(ctx context.Context, s entity.Subject, f entity.ConflictFilter) (entity.ConflictPage, error) {
	limit := f.Limit
	f.Limit++
	var out entity.ConflictPage
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageConflicts(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			out.Next = &entity.AuditKey{OccurredAt: items[limit-1].DetectedAt, ID: items[limit-1].ID}
		}
		for i := range items {
			rec, err := u.d.Repo.Record(ctx, s.OrganizationID, items[i].ReceiptID, items[i].RecordNo)
			if err != nil {
				return err
			}
			items[i].NativeKey = NativeKey(rec)
		}
		out.Items = items
		return nil
	})
	return out, err
}

// NativeKey shows the identity of a record of the source; empty when it has none.
func NativeKey(rec entity.IngestRecord) string {
	if rec.NativeID == "" || rec.NativeNS == "" {
		return rec.NativeID
	}
	return rec.NativeNS + "/" + rec.NativeID
}

// ResolveConflict records the decision on a conflict once (BR-002). kept_existing and source_corrected
// leave the accepted record as it is; accepted_new makes the new record the accepted one, the former
// one points to it as a conflict, and the new record goes to normalization.
func (u *UseCase) ResolveConflict(ctx context.Context, s entity.Subject, id entity.ID, resolution string) (entity.IngestConflict, error) {
	switch resolution {
	case entity.ResolutionKeptExisting, entity.ResolutionSourceCorrected, entity.ResolutionAcceptedNew:
	default:
		return entity.IngestConflict{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/resolution", Code: entity.FieldNotAllowedValue}}}
	}
	var out entity.IngestConflict
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		c, err := u.d.Repo.LockConflict(ctx, s.OrganizationID, id)
		if err != nil {
			return err
		}
		if !c.Open {
			return entity.ErrStateConflict
		}
		if resolution == entity.ResolutionAcceptedNew {
			if err := u.acceptNew(ctx, s.OrganizationID, c); err != nil {
				return err
			}
		}
		now, by := u.d.Clock.Now(), s.UserID
		c.Open, c.Resolution, c.ResolvedBy, c.ResolvedAt = false, resolution, &by, &now
		if err := u.d.Repo.ResolveConflict(ctx, s.OrganizationID, c); err != nil {
			return err
		}
		rec, err := u.d.Repo.Record(ctx, s.OrganizationID, c.ReceiptID, c.RecordNo)
		if err != nil {
			return err
		}
		c.NativeKey, out = NativeKey(rec), c
		return u.d.Auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: s.OrganizationID, OccurredAt: now, ActorKind: entity.ActorUser, ActorID: s.UserID,
			Operation: "resolveIngestConflict", ObjectKind: "ingest_conflict", ObjectID: c.ID, Outcome: entity.AuditSucceeded,
			ReasonCode: resolution, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		})
	})
	return out, err
}

// acceptNew swaps the accepted record of the key: the former one first, since one key has one accepted record.
func (u *UseCase) acceptNew(ctx context.Context, orgID entity.ID, c entity.IngestConflict) error {
	former, err := u.d.Repo.Record(ctx, orgID, c.ExistingReceipt, c.ExistingRecordNo)
	if err != nil {
		return err
	}
	if former.Outcome != entity.RecordAccepted {
		return entity.ErrStateConflict
	}
	receipt := c.ReceiptID
	former.Outcome, former.FirstReceiptID, former.FirstRecordNo = entity.RecordConflict, &receipt, c.RecordNo
	if err := u.d.Repo.SetRecordOutcome(ctx, orgID, c.ExistingReceipt, former); err != nil {
		return err
	}
	fresh := entity.IngestRecord{No: c.RecordNo, Outcome: entity.RecordAccepted}
	if err := u.d.Repo.SetRecordOutcome(ctx, orgID, c.ReceiptID, fresh); err != nil {
		return err
	}
	job := entity.NormalizeBatchJob{
		OrganizationID: orgID, ReceiptID: c.ReceiptID, ResolvedNo: c.RecordNo, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
	}
	return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}

// Accepted reads a receipt with its accepted records in the order of their numbers: the input of
// normalization. The records carry their identity and hash; their content is in the stored package.
func (u *UseCase) Accepted(ctx context.Context, orgID, receiptID entity.ID) (entity.Receipt, []entity.IngestRecord, error) {
	var (
		rc   entity.Receipt
		recs []entity.IngestRecord
	)
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if rc, err = u.d.Repo.Receipt(ctx, orgID, receiptID); err != nil {
			return err
		}
		recs, err = u.d.Repo.Records(ctx, orgID, receiptID)
		return err
	})
	if err != nil {
		return entity.Receipt{}, nil, err
	}
	recs = slices.DeleteFunc(recs, func(r entity.IngestRecord) bool { return r.Outcome != entity.RecordAccepted })
	slices.SortFunc(recs, func(a, b entity.IngestRecord) int { return a.No - b.No })
	return rc, recs, nil
}

// AffectedMetrics are the metrics whose data a schema of the envelope carries.
func (*UseCase) AffectedMetrics(schemaID string) []string { return AffectedMetrics(schemaID) }
