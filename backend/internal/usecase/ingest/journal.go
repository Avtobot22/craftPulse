package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _classSnapshot = "snapshot"

// errStoredPackageGone means an equal package was found before the transaction and is gone in it: the
// cleaned object was not stored, so the intake runs again.
var errStoredPackageGone = errors.New("ingest: the package this one repeats is gone, the intake is repeated")

// PreparePage checks and cleans a page of sync and stores the cleaned object before any transaction
// (TZ 10.3, rule 2); a rejected page keeps its reason and the scope it was collected for.
func (u *UseCase) PreparePage(ctx context.Context, orgID, connectionID, scopeID entity.ID, page io.Reader) (*entity.PreparedPackage, error) {
	id, err := u.d.NewID()
	if err != nil {
		return nil, err
	}
	conn, scope := connectionID, scopeID
	rc := entity.Receipt{ID: id, OrganizationID: orgID, Channel: entity.ChannelSync, ConnectionID: &conn, ScopeID: &scope, ReceivedAt: u.d.Clock.Now()}
	return u.prepare(ctx, rc, page)
}

// RecordPage writes the journal of a prepared page in the transaction of sync, so the facts, the
// checkpoint and the next job commit together.
func (u *UseCase) RecordPage(ctx context.Context, p *entity.PreparedPackage) (entity.Receipt, error) {
	return u.record(ctx, p)
}

func (u *UseCase) prepare(ctx context.Context, rc entity.Receipt, r io.Reader) (*entity.PreparedPackage, error) {
	var cleaned bytes.Buffer
	res, err := envelope.Clean(r, u.d.Limits, &cleaned)
	if err != nil {
		return nil, err
	}
	p := &entity.PreparedPackage{Receipt: rc}
	p.Receipt.RecordCount = len(res.Records)
	if err := describe(&p.Receipt, res.Manifest); err != nil {
		return nil, err
	}
	if res.Envelope != nil {
		p.Receipt.Status, p.Receipt.ReasonCode = entity.ReceiptRejected, packageReason(res.Envelope.Code)
		return p, nil //nolint:nilerr // the rejection is the outcome of the intake
	}
	instance, allowed, err := u.channel(ctx, rc, res.Manifest)
	if err != nil {
		return nil, err
	}
	p.Receipt.SourceInstanceID, p.Receipt.PackageHash, p.Receipt.ObjectKey = &instance, res.ContentHash, packageKey(rc.OrganizationID, rc.ID)
	p.Records = records(res, instance, allowed)
	err = u.d.Tx.InOrgTx(ctx, rc.OrganizationID, func(ctx context.Context) error {
		_, p.Duplicate, err = u.d.Repo.FirstReceiptOfHash(ctx, rc.OrganizationID, p.Receipt.PackageHash)
		return err
	})
	if err != nil || p.Duplicate {
		return p, err
	}
	return p, u.d.Objects.Put(ctx, p.Receipt.ObjectKey, bytes.NewReader(cleaned.Bytes()))
}

// describe copies what the receipt keeps of the manifest.
func describe(rc *entity.Receipt, m *envelope.Manifest) error {
	if m == nil {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return errorf("encode manifest: %w", err)
	}
	rc.Manifest, rc.SchemaID, rc.SchemaVersion = raw, string(m.SchemaId), m.SchemaVersion
	return nil
}

// channel limits what a channel accepts before keys are compared (TZ 12.6): sync takes only the scope
// of its page, the import only a project where the user may import. It also names the instance.
func (u *UseCase) channel(ctx context.Context, rc entity.Receipt, m *envelope.Manifest) (entity.ID, bool, error) {
	switch rc.Channel {
	case entity.ChannelSync:
		scopes, err := u.d.Connect.Scopes(ctx, rc.OrganizationID, *rc.ConnectionID)
		if err != nil {
			return entity.ID{}, false, err
		}
		i := slices.IndexFunc(scopes, func(s entity.ConnectionScope) bool { return s.ID == *rc.ScopeID })
		if i < 0 {
			return entity.ID{}, false, errorf("scope %s is not a scope of connection %s", rc.ScopeID, rc.ConnectionID)
		}
		s := &scopes[i]
		return s.InstanceID, s.Active && string(s.Kind) == string(m.Area.Kind) && s.NativeScopeID == m.Area.NativeId, nil
	case entity.ChannelImport:
		inst, err := u.d.Connect.EnsureInstance(ctx, rc.OrganizationID, entity.Provider(m.Source.Provider), m.Source.InstanceKey)
		if err != nil {
			return entity.ID{}, false, err
		}
		s := entity.Subject{OrganizationID: rc.OrganizationID, UserID: *rc.RequestedBy}
		_, err = u.d.Access.Authorize(ctx, s, entity.AccessRequest{
			Operation: "createImport", Action: "imports.create", Area: entity.AreaRef{Kind: entity.AreaProject, ID: *rc.ProjectID},
		})
		if errors.Is(err, entity.ErrForbidden) || errors.Is(err, entity.ErrNotFound) {
			return inst.ID, false, nil
		}
		return inst.ID, err == nil, err
	case entity.ChannelCI:
	}
	return entity.ID{}, false, errorf("channel %s has no intake yet", rc.Channel)
}

// records turns the checked records into journal rows. A record without its own revision is revised by its
// content hash, a snapshot by the hash together with the read ID (TZ 12.6).
func records(res *envelope.Result, instance entity.ID, allowed bool) []entity.IngestRecord {
	readID := ""
	if res.Manifest != nil && res.Manifest.ReadId != nil {
		readID = *res.Manifest.ReadId
	}
	out := make([]entity.IngestRecord, 0, len(res.Records))
	for i := range res.Records {
		r := &res.Records[i]
		rec := entity.IngestRecord{No: r.No, Class: r.Class, Kind: r.Kind, PayloadSchema: r.PayloadSchema, NativeNS: r.NativeNS, NativeID: r.NativeID}
		switch {
		case r.Rejection != nil:
			rec.Outcome, rec.ReasonCode = entity.RecordRejected, recordReason(r.Rejection.Code)
		case !allowed:
			rec.Outcome, rec.ReasonCode = entity.RecordRejected, entity.RecordOutsideScope
		default:
			rec.InstanceID, rec.ContentHash = instance, unhex(r.Hash)
			rec.SourceRevision = r.Hash
			if r.SourceRevision != nil && *r.SourceRevision != "" {
				rec.SourceRevision = *r.SourceRevision
			} else if r.Class == _classSnapshot {
				sum := sha256.Sum256([]byte(readID + "\x00" + r.Hash))
				rec.SourceRevision = hex.EncodeToString(sum[:])
			}
		}
		out = append(out, rec)
	}
	return out
}

func unhex(s string) []byte {
	b, _ := hex.DecodeString(s) // the envelope computes the hash as hex
	return b
}

// record writes the journal of a prepared package in the caller's transaction. An equal package stored
// before gives a duplicate; otherwise each record goes in with one insert: inserted is a new fact, an
// accepted record of the same key with the same hash is a replay, with another hash a conflict that
// leaves the accepted fact as it is (BR-002).
func (u *UseCase) record(ctx context.Context, p *entity.PreparedPackage) (entity.Receipt, error) {
	rc := p.Receipt
	org := rc.OrganizationID
	rc.QuarantineKey = ""
	if rc.Status == entity.ReceiptRejected {
		if err := u.d.Repo.SaveReceipt(ctx, rc); err != nil {
			return entity.Receipt{}, err
		}
		u.d.Metrics.PackageRejected(rc.Channel, rc.ReasonCode)
		return rc, nil
	}
	if err := u.d.Repo.LockPackageHash(ctx, org, rc.PackageHash); err != nil {
		return entity.Receipt{}, err
	}
	first, dup, err := u.d.Repo.FirstReceiptOfHash(ctx, org, rc.PackageHash)
	if err != nil {
		return entity.Receipt{}, err
	}
	if dup {
		rc.Status, rc.DuplicateOf, rc.ObjectKey = entity.ReceiptDuplicate, &first, ""
		return rc, u.d.Repo.SaveReceipt(ctx, rc)
	}
	if p.Duplicate {
		return entity.Receipt{}, errStoredPackageGone
	}
	rc.Status = entity.ReceiptQuarantined
	if err := u.d.Repo.SaveReceipt(ctx, rc); err != nil {
		return entity.Receipt{}, err
	}
	journal, err := u.outcomes(ctx, rc, p.Records)
	if err != nil {
		return entity.Receipt{}, err
	}
	for i := range journal {
		switch journal[i].Outcome {
		case entity.RecordAccepted:
			rc.AcceptedCount++
		case entity.RecordRejected:
			rc.RejectedCount++
		case entity.RecordReplay, entity.RecordConflict:
		}
	}
	rc.Status = entity.ReceiptAccepted
	if rc.RejectedCount > 0 {
		rc.Status = entity.ReceiptPartial
	}
	if err := u.d.Repo.SaveReceipt(ctx, rc); err != nil {
		return entity.Receipt{}, err
	}
	if rc.AcceptedCount > 0 {
		queue := entity.QueueBulk
		if rc.Channel == entity.ChannelCI {
			queue = entity.QueueInteractive
		}
		job := entity.NormalizeBatchJob{
			OrganizationID: org, ReceiptID: rc.ID, ObservedVia: p.ObservedVia, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		}
		if err := u.d.Queue.Enqueue(ctx, job, queue); err != nil {
			return entity.Receipt{}, err
		}
	}
	return rc, nil
}

// outcomes inserts the candidates as accepted in one batch, reads the accepted records the others met
// in another and writes replays, conflicts and rejected records.
func (u *UseCase) outcomes(ctx context.Context, rc entity.Receipt, recs []entity.IngestRecord) ([]entity.IngestRecord, error) {
	org := rc.OrganizationID
	var candidates, others []entity.IngestRecord
	for i := range recs {
		if recs[i].Outcome == entity.RecordRejected {
			others = append(others, recs[i])
		} else {
			candidates = append(candidates, recs[i])
		}
	}
	inserted, err := u.d.Repo.AcceptRecords(ctx, org, rc.ID, candidates)
	if err != nil {
		return nil, err
	}
	var accepted, met []entity.IngestRecord
	for i := range candidates {
		if inserted[i] {
			candidates[i].Outcome = entity.RecordAccepted
			accepted = append(accepted, candidates[i])
		} else {
			met = append(met, candidates[i])
		}
	}
	existing, err := u.d.Repo.AcceptedRecords(ctx, org, met)
	if err != nil {
		return nil, err
	}
	var conflicts []entity.IngestRecord
	for i := range met {
		met[i].FirstReceiptID, met[i].FirstRecordNo = existing[i].FirstReceiptID, existing[i].FirstRecordNo
		if met[i].FirstReceiptID == nil {
			return nil, errorf("record %d met no accepted record", met[i].No)
		}
		met[i].Outcome = entity.RecordReplay
		if !bytes.Equal(existing[i].ContentHash, met[i].ContentHash) {
			met[i].Outcome = entity.RecordConflict
			conflicts = append(conflicts, met[i])
		}
	}
	others = append(others, met...)
	if err := u.d.Repo.InsertRecords(ctx, org, rc.ID, others); err != nil {
		return nil, err
	}
	if err := u.d.Repo.InsertConflicts(ctx, org, rc.ID, conflicts, u.d.Clock.Now()); err != nil {
		return nil, err
	}
	return append(others, accepted...), nil
}
