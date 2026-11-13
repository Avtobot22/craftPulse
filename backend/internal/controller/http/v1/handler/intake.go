package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
)

const (
	// _maxImportFile is the limit of the intake; the package around the file is checked again by it.
	_maxImportFile  = 25 << 20
	_maxFormField   = 1024
	_receiptCursor  = "receipt"
	_recordCursor   = "receipt_record"
	_conflictCursor = "ingest_conflict"
	_receiptPoll    = 3
)

var (
	errPayloadTooLarge = errors.New("handler: the file is larger than the limit of the intake")
	errUnsupportedFile = errors.New("handler: the file is neither JSON nor CSV")
	errMalformedForm   = errors.New("handler: the form of the import is not valid multipart")
)

type intake struct{ uc usecase.Intake }

func (h *intake) CreateImport(ctx context.Context, req openapi.CreateImportRequestObject) (openapi.CreateImportResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, errUnsupportedFile
	}
	f, err := importForm(req.Body)
	if err != nil {
		return nil, err
	}
	rc, err := h.uc.Import(ctx, s, f)
	if err != nil {
		return nil, err
	}
	location := "/api/v1/receipts/" + rc.ID.String()
	body := openapi.ReceiptAccepted{ReceiptId: uuid.UUID(rc.ID), Status: openapi.ReceiptAcceptedStatus(rc.Status)}
	switch rc.Status {
	case entity.ReceiptDuplicate:
		body.DuplicateOf = uuidPtr(rc.DuplicateOf)
	case entity.ReceiptQuarantined:
		n := _receiptPoll
		body.RetryAfterSeconds = &n
	default:
	}
	return openapi.CreateImport202JSONResponse{Body: body, Headers: openapi.CreateImport202ResponseHeaders{Location: &location}}, nil
}

// importForm reads the fields and the one file of the form; the file is read up to the limit of the intake.
func importForm(r *multipart.Reader) (entity.ImportFile, error) {
	var (
		f       entity.ImportFile
		fields  = map[string]string{}
		hasFile bool
	)
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return entity.ImportFile{}, errPayloadTooLarge
		}
		if err != nil {
			return entity.ImportFile{}, errMalformedForm
		}
		name := part.FormName()
		if name == "file" {
			if f.Content, err = io.ReadAll(io.LimitReader(part, _maxImportFile+1)); err != nil {
				if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
					return entity.ImportFile{}, errPayloadTooLarge
				}
				return entity.ImportFile{}, errMalformedForm
			}
			if len(f.Content) > _maxImportFile {
				return entity.ImportFile{}, errPayloadTooLarge
			}
			if f.Format, err = fileFormat(part.Header.Get("Content-Type"), f.Content); err != nil {
				return entity.ImportFile{}, err
			}
			hasFile = true
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, _maxFormField+1))
		if err != nil || len(value) > _maxFormField {
			return entity.ImportFile{}, errMalformedForm
		}
		fields[name] = string(value)
	}
	v := &entity.ValidationError{}
	if !hasFile {
		v.Add("/file", entity.FieldRequired, "")
	}
	for _, name := range []string{"schema_id", "schema_version", "source_provider", "source_instance_key", "project_id"} {
		if fields[name] == "" {
			v.Add("/"+name, entity.FieldRequired, "")
		}
	}
	project, err := uuid.Parse(fields["project_id"])
	if fields["project_id"] != "" && err != nil {
		v.Add("/project_id", entity.FieldInvalidFormat, "")
	}
	if raw, set := fields["snapshot"]; set {
		snapshot, err := strconv.ParseBool(raw)
		if err != nil {
			v.Add("/snapshot", entity.FieldInvalidFormat, "")
		}
		f.Snapshot = snapshot
	}
	for _, name := range []string{"report_format", "commit_sha"} {
		if fields[name] != "" {
			v.Add("/"+name, entity.FieldNotAllowedValue, "reports of findings, tests and coverage come with the CI step")
		}
	}
	if len(v.Fields) > 0 {
		return entity.ImportFile{}, v
	}
	f.ProjectID, f.SchemaID, f.SchemaVersion = entity.ID(project), fields["schema_id"], fields["schema_version"]
	f.Provider, f.InstanceKey = entity.Provider(fields["source_provider"]), fields["source_instance_key"]
	return f, nil
}

// fileFormat takes the media type of the part, and for a generic one looks at the first character.
func fileFormat(contentType string, content []byte) (entity.ImportFormat, error) {
	media, _, _ := mime.ParseMediaType(contentType)
	switch media {
	case "application/json":
		return entity.ImportJSON, nil
	case "text/csv":
		return entity.ImportCSV, nil
	case "", "application/octet-stream", "text/plain":
		trimmed := bytes.TrimLeft(content, " \t\r\n\ufeff")
		if len(trimmed) > 0 && trimmed[0] == '[' {
			return entity.ImportJSON, nil
		}
		return entity.ImportCSV, nil
	}
	return "", errUnsupportedFile
}

func (h *intake) ListReceipts(ctx context.Context, req openapi.ListReceiptsRequestObject) (openapi.ListReceiptsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	p := req.Params
	f := entity.ReceiptFilter{Limit: limit}
	if p.Channel != nil {
		f.Channel = entity.ReceiptChannel(*p.Channel)
	}
	if p.Status != nil {
		f.Status = entity.ReceiptStatus(*p.Status)
	}
	if p.ConnectionId != nil {
		id := entity.ID(*p.ConnectionId)
		f.ConnectionID = &id
	}
	if p.ProjectId != nil {
		id := entity.ID(*p.ProjectId)
		f.ProjectID = &id
	}
	filter := string(f.Channel) + "|" + string(f.Status) + "|" + optionalID(f.ConnectionID) + "|" + optionalID(f.ProjectID)
	if p.Cursor != nil {
		c, ok := decodeCursor(*p.Cursor)
		if !ok || c.Kind != _receiptCursor || c.Name != filter || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		f.After = &entity.AuditKey{OccurredAt: c.At, ID: entity.ID(c.ID)}
	}
	page, err := h.uc.ListReceipts(ctx, s, f)
	if err != nil {
		return nil, err
	}
	out := openapi.ReceiptPage{Data: make([]openapi.ReceiptSummary, 0, len(page.Items))}
	for i := range page.Items {
		rc := &page.Items[i]
		out.Data = append(out.Data, openapi.ReceiptSummary{
			Id: uuid.UUID(rc.ID), Channel: openapi.ReceiptSummaryChannel(rc.Channel), Status: openapi.ReceiptSummaryStatus(rc.Status),
			ReceivedAt: rc.ReceivedAt, SchemaId: optional(rc.SchemaID), SchemaVersion: optional(rc.SchemaVersion),
			RecordCount: rc.RecordCount, AcceptedCount: rc.AcceptedCount, RejectedCount: rc.RejectedCount,
			ReasonCode: optional(rc.ReasonCode), DuplicateOf: uuidPtr(rc.DuplicateOf), AffectedMetrics: ingest.AffectedMetrics(rc.SchemaID),
		})
	}
	if page.Next != nil {
		c := encodeCursor(cursorData{Kind: _receiptCursor, Name: filter, At: page.Next.OccurredAt, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &c
	}
	return openapi.ListReceipts200JSONResponse(out), nil
}

func (h *intake) GetReceipt(ctx context.Context, req openapi.GetReceiptRequestObject) (openapi.GetReceiptResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	rc, err := h.uc.Receipt(ctx, s, entity.ID(req.ReceiptId))
	if err != nil {
		return nil, err
	}
	out := openapi.Receipt{
		Id: uuid.UUID(rc.ID), Channel: openapi.ReceiptChannel(rc.Channel), Status: openapi.ReceiptStatus(rc.Status),
		ReceivedAt: rc.ReceivedAt, SchemaId: optional(rc.SchemaID), SchemaVersion: optional(rc.SchemaVersion),
		RecordCount: rc.RecordCount, AcceptedCount: rc.AcceptedCount, RejectedCount: rc.RejectedCount,
		ReplayCount: rc.ReplayCount, ConflictCount: rc.ConflictCount, ReasonCode: optional(rc.ReasonCode),
		DuplicateOf: uuidPtr(rc.DuplicateOf), AffectedMetrics: ingest.AffectedMetrics(rc.SchemaID),
	}
	if rc.Status == entity.ReceiptQuarantined {
		n := _receiptPoll
		out.RetryAfterSeconds = &n
	}
	return openapi.GetReceipt200JSONResponse(out), nil
}

func (h *intake) ListReceiptRecords(ctx context.Context, req openapi.ListReceiptRecordsRequestObject) (openapi.ListReceiptRecordsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.RecordFilter{Limit: limit}
	if req.Params.Outcome != nil {
		f.Outcome = entity.RecordOutcome(*req.Params.Outcome)
	}
	name := req.ReceiptId.String() + "|" + string(f.Outcome)
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		no, err := strconv.Atoi(c.Sort)
		if !ok || c.Kind != _recordCursor || c.Name != name || err != nil || no < 1 {
			return nil, errMalformedPage
		}
		f.AfterNo = no
	}
	page, err := h.uc.ListRecords(ctx, s, entity.ID(req.ReceiptId), f)
	if err != nil {
		return nil, err
	}
	out := openapi.ReceiptRecordPage{Data: make([]openapi.ReceiptRecord, 0, len(page.Items))}
	for i := range page.Items {
		rec := &page.Items[i]
		out.Data = append(out.Data, openapi.ReceiptRecord{
			RecordNo: rec.No, RecordKind: rec.Kind, NativeKey: optional(ingest.NativeKey(*rec)),
			Outcome: openapi.ReceiptRecordOutcome(rec.Outcome), ReasonCode: optional(rec.ReasonCode),
		})
	}
	if page.NextNo > 0 {
		c := encodeCursor(cursorData{Kind: _recordCursor, Name: name, Sort: strconv.Itoa(page.NextNo)})
		out.NextCursor = &c
	}
	return openapi.ListReceiptRecords200JSONResponse(out), nil
}

func (h *intake) ListIngestConflicts(ctx context.Context, req openapi.ListIngestConflictsRequestObject) (openapi.ListIngestConflictsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.ConflictFilter{Limit: limit, Open: req.Params.Status == nil || *req.Params.Status == openapi.ListIngestConflictsParamsStatusOpen}
	name := strconv.FormatBool(f.Open)
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != _conflictCursor || c.Name != name || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		f.After = &entity.AuditKey{OccurredAt: c.At, ID: entity.ID(c.ID)}
	}
	page, err := h.uc.ListConflicts(ctx, s, f)
	if err != nil {
		return nil, err
	}
	out := openapi.IngestConflictPage{Data: make([]openapi.IngestConflict, 0, len(page.Items))}
	for i := range page.Items {
		out.Data = append(out.Data, conflictBody(&page.Items[i]))
	}
	if page.Next != nil {
		c := encodeCursor(cursorData{Kind: _conflictCursor, Name: name, At: page.Next.OccurredAt, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &c
	}
	return openapi.ListIngestConflicts200JSONResponse(out), nil
}

func (h *intake) ResolveIngestConflict(ctx context.Context, req openapi.ResolveIngestConflictRequestObject) (openapi.ResolveIngestConflictResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/resolution", Code: entity.FieldRequired}}}
	}
	c, err := h.uc.ResolveConflict(ctx, s, entity.ID(req.ConflictId), string(req.Body.Resolution))
	if err != nil {
		return nil, err
	}
	return openapi.ResolveIngestConflict200JSONResponse(conflictBody(&c)), nil
}

func conflictBody(c *entity.IngestConflict) openapi.IngestConflict {
	out := openapi.IngestConflict{
		Id: uuid.UUID(c.ID), ReceiptId: uuid.UUID(c.ReceiptID), RecordNo: c.RecordNo, ExistingReceiptId: uuid.UUID(c.ExistingReceipt),
		ExistingRecordNo: c.ExistingRecordNo, NativeKey: c.NativeKey, DetectedAt: c.DetectedAt, Status: openapi.IngestConflictStatusResolved,
	}
	if c.Open {
		out.Status = openapi.IngestConflictStatusOpen
	}
	if c.Resolution != "" {
		r := openapi.IngestConflictResolution(c.Resolution)
		out.Resolution = &r
	}
	return out
}

func optionalID(id *entity.ID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
