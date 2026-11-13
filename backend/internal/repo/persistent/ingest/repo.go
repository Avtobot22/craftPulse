package ingest

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// SaveReceipt writes a receipt or replaces the outcome of one written earlier in the same intake.
func (r *Repo) SaveReceipt(ctx context.Context, rc entity.Receipt) error {
	if rc.RecordCount > math.MaxInt32 {
		return errors.New("ingest: record count does not fit its column")
	}
	err := New(postgres.Conn(ctx, r.pool)).SaveReceipt(ctx, SaveReceiptParams{
		OrganizationID: uuid.UUID(rc.OrganizationID), ID: uuid.UUID(rc.ID), Channel: string(rc.Channel),
		ConnectionID: nullID(rc.ConnectionID), ScopeID: nullID(rc.ScopeID), ProjectID: nullID(rc.ProjectID), UploadTokenID: nullID(rc.UploadTokenID),
		RequestedBy: nullID(rc.RequestedBy), SourceInstanceID: nullID(rc.SourceInstanceID), ReceivedAt: rc.ReceivedAt,
		Status: string(rc.Status), DuplicateOf: nullID(rc.DuplicateOf), SchemaID: text(rc.SchemaID), SchemaVersion: text(rc.SchemaVersion),
		PackageHash: rc.PackageHash, ObjectKey: text(rc.ObjectKey), QuarantineKey: text(rc.QuarantineKey),
		RecordCount: int32(rc.RecordCount), AcceptedCount: int32(rc.AcceptedCount), RejectedCount: int32(rc.RejectedCount), //nolint:gosec // bounded by the check above
		ReasonCode: text(rc.ReasonCode), Manifest: rc.Manifest,
	})
	if err != nil {
		return fmt.Errorf("ingest: save receipt: %w", err)
	}
	return nil
}

func (r *Repo) Receipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetReceipt(ctx, GetReceiptParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	return receipt(row, err)
}

// LockReceipt reads a receipt for update: one intake of a package finishes it.
func (r *Repo) LockReceipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockReceipt(ctx, LockReceiptParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	return receipt(row, err)
}

// FirstReceiptOfHash is the stored package an equal one repeats.
func (r *Repo) FirstReceiptOfHash(ctx context.Context, orgID entity.ID, hash []byte) (entity.ID, bool, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).FirstReceiptOfHash(ctx, FirstReceiptOfHashParams{OrganizationID: uuid.UUID(orgID), PackageHash: hash})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, false, nil
	}
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("ingest: find package: %w", err)
	}
	return entity.ID(id), true, nil
}

// LockPackageHash makes two intakes of an equal package run one after another until the commit.
func (r *Repo) LockPackageHash(ctx context.Context, orgID entity.ID, hash []byte) error {
	err := New(postgres.Conn(ctx, r.pool)).LockPackageHash(ctx, LockPackageHashParams{OrganizationID: orgID.String(), PackageHash: hash})
	if err != nil {
		return fmt.Errorf("ingest: lock package: %w", err)
	}
	return nil
}

// AcceptRecords inserts every record as accepted in one batch and reports which ones went in; a record
// whose key already has an accepted one is left out, also when a concurrent intake committed it.
func (r *Repo) AcceptRecords(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord) ([]bool, error) {
	if len(recs) == 0 {
		return nil, nil
	}
	params := make([]AcceptRecordParams, len(recs))
	for i := range recs {
		rec := &recs[i]
		params[i] = AcceptRecordParams{
			OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNo: number(rec.No),
			RecordClass: text(rec.Class), RecordKind: text(rec.Kind), PayloadSchema: rec.PayloadSchema,
			InstanceID: uuid.NullUUID{UUID: uuid.UUID(rec.InstanceID), Valid: true}, NativeNs: rec.NativeNS,
			NativeID: text(rec.NativeID), SourceRevision: text(rec.SourceRevision), ContentHash: rec.ContentHash,
		}
	}
	inserted := make([]bool, len(recs))
	var batchErr error
	New(postgres.Conn(ctx, r.pool)).AcceptRecord(ctx, params).QueryRow(func(i int, _ int32, err error) {
		switch {
		case err == nil:
			inserted[i] = true
		case errors.Is(err, pgx.ErrNoRows):
		case batchErr == nil:
			batchErr = err
		}
	})
	if batchErr != nil {
		return nil, fmt.Errorf("ingest: accept records: %w", batchErr)
	}
	return inserted, nil
}

// AcceptedRecords reads the accepted record of the key of every record.
func (r *Repo) AcceptedRecords(ctx context.Context, orgID entity.ID, recs []entity.IngestRecord) ([]entity.IngestRecord, error) {
	if len(recs) == 0 {
		return nil, nil
	}
	params := make([]AcceptedRecordParams, len(recs))
	for i := range recs {
		rec := &recs[i]
		params[i] = AcceptedRecordParams{
			OrganizationID: uuid.UUID(orgID), InstanceID: uuid.NullUUID{UUID: uuid.UUID(rec.InstanceID), Valid: true},
			RecordKind: text(rec.Kind), NativeNs: rec.NativeNS, NativeID: text(rec.NativeID),
			SourceRevision: text(rec.SourceRevision), PayloadSchema: rec.PayloadSchema,
		}
	}
	out := make([]entity.IngestRecord, len(recs))
	var batchErr error
	New(postgres.Conn(ctx, r.pool)).AcceptedRecord(ctx, params).QueryRow(func(i int, row AcceptedRecordRow, err error) {
		if err != nil {
			if batchErr == nil {
				batchErr = err
			}
			return
		}
		id := entity.ID(row.ReceiptID)
		out[i] = entity.IngestRecord{FirstReceiptID: &id, FirstRecordNo: int(row.RecordNo), ContentHash: row.ContentHash}
	})
	if batchErr != nil {
		return nil, fmt.Errorf("ingest: read accepted records: %w", batchErr)
	}
	return out, nil
}

// InsertRecords writes records with an outcome other than accepted.
func (r *Repo) InsertRecords(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord) error {
	if len(recs) == 0 {
		return nil
	}
	params := make([]InsertRecordParams, len(recs))
	for i := range recs {
		rec := &recs[i]
		p := InsertRecordParams{
			OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNo: number(rec.No),
			RecordClass: text(rec.Class), RecordKind: text(rec.Kind), PayloadSchema: rec.PayloadSchema, NativeNs: rec.NativeNS,
			NativeID: text(rec.NativeID), SourceRevision: text(rec.SourceRevision), ContentHash: rec.ContentHash,
			Outcome: string(rec.Outcome), ReasonCode: text(rec.ReasonCode), FirstReceiptID: nullID(rec.FirstReceiptID),
		}
		if !rec.InstanceID.IsZero() {
			p.InstanceID = uuid.NullUUID{UUID: uuid.UUID(rec.InstanceID), Valid: true}
		}
		if rec.FirstReceiptID != nil {
			p.FirstRecordNo = pgtype.Int4{Int32: number(rec.FirstRecordNo), Valid: true}
		}
		params[i] = p
	}
	var batchErr error
	New(postgres.Conn(ctx, r.pool)).InsertRecord(ctx, params).Exec(func(_ int, err error) {
		if err != nil && batchErr == nil {
			batchErr = err
		}
	})
	if batchErr != nil {
		return fmt.Errorf("ingest: insert records: %w", batchErr)
	}
	return nil
}

// InsertConflicts opens a conflict for every record with the accepted record it met.
func (r *Repo) InsertConflicts(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord, at time.Time) error {
	if len(recs) == 0 {
		return nil
	}
	params := make([]InsertConflictParams, len(recs))
	for i := range recs {
		params[i] = InsertConflictParams{
			OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNo: number(recs[i].No),
			ExistingReceiptID: uuid.UUID(*recs[i].FirstReceiptID), ExistingRecordNo: number(recs[i].FirstRecordNo), DetectedAt: at,
		}
	}
	var batchErr error
	New(postgres.Conn(ctx, r.pool)).InsertConflict(ctx, params).Exec(func(_ int, err error) {
		if err != nil && batchErr == nil {
			batchErr = err
		}
	})
	if batchErr != nil {
		return fmt.Errorf("ingest: insert conflicts: %w", batchErr)
	}
	return nil
}

func (r *Repo) Records(ctx context.Context, orgID, receiptID entity.ID) ([]entity.IngestRecord, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ListRecords(ctx, ListRecordsParams{OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID)})
	if err != nil {
		return nil, fmt.Errorf("ingest: list records: %w", err)
	}
	out := make([]entity.IngestRecord, 0, len(rows))
	for i := range rows {
		out = append(out, record(&rows[i]))
	}
	return out, nil
}

func receipt(row IngestReceipt, err error) (entity.Receipt, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Receipt{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Receipt{}, fmt.Errorf("ingest: get receipt: %w", err)
	}
	return entity.Receipt{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), Channel: entity.ReceiptChannel(row.Channel),
		ConnectionID: idPtr(row.ConnectionID), ScopeID: idPtr(row.ScopeID), ProjectID: idPtr(row.ProjectID), UploadTokenID: idPtr(row.UploadTokenID),
		RequestedBy: idPtr(row.RequestedBy), SourceInstanceID: idPtr(row.SourceInstanceID), ReceivedAt: row.ReceivedAt,
		Status: entity.ReceiptStatus(row.Status), DuplicateOf: idPtr(row.DuplicateOf), SchemaID: row.SchemaID.String,
		SchemaVersion: row.SchemaVersion.String, PackageHash: row.PackageHash, ObjectKey: row.ObjectKey.String,
		QuarantineKey: row.QuarantineKey.String, RecordCount: int(row.RecordCount), AcceptedCount: int(row.AcceptedCount),
		RejectedCount: int(row.RejectedCount), ReasonCode: row.ReasonCode.String, Manifest: row.Manifest,
	}, nil
}

func number(n int) int32 {
	if n > math.MaxInt32 || n < 0 {
		return math.MaxInt32
	}
	return int32(n)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func nullID(id *entity.ID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func idPtr(id uuid.NullUUID) *entity.ID {
	if !id.Valid {
		return nil
	}
	out := entity.ID(id.UUID)
	return &out
}

func (r *Repo) PageReceipts(ctx context.Context, orgID entity.ID, f entity.ReceiptFilter) ([]entity.Receipt, error) {
	p := PageReceiptsParams{
		OrganizationID: uuid.UUID(orgID), Channel: string(f.Channel), Status: string(f.Status),
		ConnectionID: nullID(f.ConnectionID), ProjectID: nullID(f.ProjectID), PageLimit: number(f.Limit),
	}
	if f.After != nil {
		p.HasCursor, p.AfterTime, p.AfterID = true, f.After.OccurredAt, uuid.UUID(f.After.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageReceipts(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("ingest: page receipts: %w", err)
	}
	out := make([]entity.Receipt, 0, len(rows))
	for i := range rows {
		rc, err := receipt(rows[i], nil)
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

// CountRepeats fills the replay and conflict counts of the receipts.
func (r *Repo) CountRepeats(ctx context.Context, orgID entity.ID, receipts []entity.Receipt) error {
	ids := make([]uuid.UUID, len(receipts))
	at := make(map[uuid.UUID]int, len(receipts))
	for i := range receipts {
		ids[i] = uuid.UUID(receipts[i].ID)
		at[ids[i]] = i
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).CountOutcomes(ctx, CountOutcomesParams{OrganizationID: uuid.UUID(orgID), ReceiptIds: ids})
	if err != nil {
		return fmt.Errorf("ingest: count outcomes: %w", err)
	}
	for _, row := range rows {
		rc := &receipts[at[row.ReceiptID]]
		if row.Outcome == string(entity.RecordReplay) {
			rc.ReplayCount = int(row.N)
		} else {
			rc.ConflictCount = int(row.N)
		}
	}
	return nil
}

func (r *Repo) PageRecords(ctx context.Context, orgID, receiptID entity.ID, f entity.RecordFilter) ([]entity.IngestRecord, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).PageRecords(ctx, PageRecordsParams{
		OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), Outcome: string(f.Outcome),
		AfterNo: number(f.AfterNo), PageLimit: number(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: page records: %w", err)
	}
	out := make([]entity.IngestRecord, 0, len(rows))
	for i := range rows {
		out = append(out, record(&rows[i]))
	}
	return out, nil
}

func (r *Repo) Record(ctx context.Context, orgID, receiptID entity.ID, no int) (entity.IngestRecord, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetRecord(ctx, GetRecordParams{OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNo: number(no)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.IngestRecord{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.IngestRecord{}, fmt.Errorf("ingest: get record: %w", err)
	}
	return record(&row), nil
}

// SetRecordOutcome changes the outcome of a record and the accepted record it points to.
func (r *Repo) SetRecordOutcome(ctx context.Context, orgID, receiptID entity.ID, rec entity.IngestRecord) error {
	p := SetRecordOutcomeParams{
		OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNo: number(rec.No),
		Outcome: string(rec.Outcome), FirstReceiptID: nullID(rec.FirstReceiptID),
	}
	if rec.FirstReceiptID != nil {
		p.FirstRecordNo = pgtype.Int4{Int32: number(rec.FirstRecordNo), Valid: true}
	}
	if err := New(postgres.Conn(ctx, r.pool)).SetRecordOutcome(ctx, p); err != nil {
		return fmt.Errorf("ingest: set record outcome: %w", err)
	}
	return nil
}

func (r *Repo) PageConflicts(ctx context.Context, orgID entity.ID, f entity.ConflictFilter) ([]entity.IngestConflict, error) {
	status := "resolved"
	if f.Open {
		status = "open"
	}
	p := PageConflictsParams{OrganizationID: uuid.UUID(orgID), Status: status, PageLimit: number(f.Limit)}
	if f.After != nil {
		p.HasCursor, p.AfterTime, p.AfterID = true, f.After.OccurredAt, uuid.UUID(f.After.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageConflicts(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("ingest: page conflicts: %w", err)
	}
	out := make([]entity.IngestConflict, 0, len(rows))
	for i := range rows {
		out = append(out, conflict(&rows[i]))
	}
	return out, nil
}

func (r *Repo) LockConflict(ctx context.Context, orgID, id entity.ID) (entity.IngestConflict, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LockConflict(ctx, LockConflictParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.IngestConflict{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.IngestConflict{}, fmt.Errorf("ingest: lock conflict: %w", err)
	}
	return conflict(&row), nil
}

func (r *Repo) ResolveConflict(ctx context.Context, orgID entity.ID, c entity.IngestConflict) error {
	err := New(postgres.Conn(ctx, r.pool)).ResolveConflict(ctx, ResolveConflictParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(c.ID), Resolution: text(c.Resolution),
		ResolvedBy: nullID(c.ResolvedBy), ResolvedAt: pgtype.Timestamptz{Time: *c.ResolvedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("ingest: resolve conflict: %w", err)
	}
	return nil
}

func conflict(row *IngestConflict) entity.IngestConflict {
	out := entity.IngestConflict{
		ID: entity.ID(row.ID), ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo),
		ExistingReceipt: entity.ID(row.ExistingReceiptID), ExistingRecordNo: int(row.ExistingRecordNo),
		DetectedAt: row.DetectedAt, Open: row.Status == "open", Resolution: row.Resolution.String, ResolvedBy: idPtr(row.ResolvedBy),
	}
	if row.ResolvedAt.Valid {
		out.ResolvedAt = &row.ResolvedAt.Time
	}
	return out
}

func record(row *IngestRecord) entity.IngestRecord {
	rec := entity.IngestRecord{
		No: int(row.RecordNo), Class: row.RecordClass.String, Kind: row.RecordKind.String, PayloadSchema: row.PayloadSchema,
		InstanceID: entity.ID(row.InstanceID.UUID), NativeNS: row.NativeNs, NativeID: row.NativeID.String,
		SourceRevision: row.SourceRevision.String, ContentHash: row.ContentHash, Outcome: entity.RecordOutcome(row.Outcome),
		ReasonCode: row.ReasonCode.String, FirstRecordNo: int(row.FirstRecordNo.Int32),
	}
	if row.FirstReceiptID.Valid {
		id := entity.ID(row.FirstReceiptID.UUID)
		rec.FirstReceiptID = &id
	}
	return rec
}
