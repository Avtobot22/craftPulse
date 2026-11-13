package entity

import "time"

type ReceiptChannel string

const (
	ChannelSync   ReceiptChannel = "sync"
	ChannelCI     ReceiptChannel = "ci"
	ChannelImport ReceiptChannel = "import"
)

type ReceiptStatus string

const (
	ReceiptQuarantined ReceiptStatus = "quarantined"
	ReceiptAccepted    ReceiptStatus = "accepted"
	ReceiptPartial     ReceiptStatus = "partial"
	ReceiptRejected    ReceiptStatus = "rejected"
	ReceiptDuplicate   ReceiptStatus = "duplicate"
)

// Reasons of a rejected package (Receipt.reason_code).
const (
	PackageLimitExceeded     = "limit_exceeded"
	PackagePathUnsafe        = "path_unsafe"
	PackageEnvelopeInvalid   = "envelope_invalid"
	PackageSchemaUnsupported = "schema_unsupported"
)

type RecordOutcome string

const (
	RecordAccepted RecordOutcome = "accepted"
	RecordReplay   RecordOutcome = "replay"
	RecordConflict RecordOutcome = "conflict"
	RecordRejected RecordOutcome = "rejected"
)

// Reasons of a rejected record the channel adds to those of the envelope.
const (
	RecordOutsideScope    = "outside_scope"
	RecordMissingSourceID = "missing_source_id"
)

// Receipt is one received package (DR-003); the records of the source live only in its stored object.
type Receipt struct {
	ID               ID
	OrganizationID   ID
	Channel          ReceiptChannel
	ConnectionID     *ID
	ScopeID          *ID
	ProjectID        *ID
	UploadTokenID    *ID
	RequestedBy      *ID
	SourceInstanceID *ID
	ReceivedAt       time.Time
	Status           ReceiptStatus
	DuplicateOf      *ID
	SchemaID         string
	SchemaVersion    string
	PackageHash      []byte
	ObjectKey        string
	QuarantineKey    string
	RecordCount      int
	AcceptedCount    int
	RejectedCount    int
	// ReplayCount and ConflictCount are counted from the records when a receipt is read.
	ReplayCount   int
	ConflictCount int
	ReasonCode    string
	// Manifest is the manifest of the package as JSON; nil when the package has none that reads.
	Manifest []byte
}

// IngestRecord is a record of a package in the journal: its identity of the source and its outcome.
type IngestRecord struct {
	No             int
	Class          string
	Kind           string
	PayloadSchema  string
	InstanceID     ID
	NativeNS       string
	NativeID       string
	SourceRevision string
	ContentHash    []byte
	Outcome        RecordOutcome
	ReasonCode     string
	// FirstReceiptID and FirstRecordNo point to the accepted record a replay or a conflict met.
	FirstReceiptID *ID
	FirstRecordNo  int
}

// PreparedPackage is a checked and cleaned package whose journal is not written yet.
type PreparedPackage struct {
	Receipt Receipt
	Records []IngestRecord
	// Duplicate is set when a stored package with the same hash was found before the transaction.
	Duplicate bool
}

type ReceiptFilter struct {
	Channel      ReceiptChannel
	Status       ReceiptStatus
	ConnectionID *ID
	ProjectID    *ID
	Limit        int
	After        *AuditKey
}

type ReceiptPage struct {
	Items []Receipt
	Next  *AuditKey
}

type RecordFilter struct {
	Outcome RecordOutcome
	Limit   int
	AfterNo int
}

type RecordPage struct {
	Items []IngestRecord
	// NextNo is the record number to continue after; zero when the page is the last one.
	NextNo int
}

// Resolutions of a conflict of intake (BR-002).
const (
	ResolutionSourceCorrected = "source_corrected"
	ResolutionKeptExisting    = "kept_existing"
	ResolutionAcceptedNew     = "accepted_new"
)

// IngestConflict is a record with the source_revision of an accepted record and another content.
type IngestConflict struct {
	ID               ID
	ReceiptID        ID
	RecordNo         int
	ExistingReceipt  ID
	ExistingRecordNo int
	// NativeKey shows the identity of the source of the record.
	NativeKey  string
	DetectedAt time.Time
	Open       bool
	Resolution string
	ResolvedBy *ID
	ResolvedAt *time.Time
}

type ConflictFilter struct {
	Open  bool
	Limit int
	After *AuditKey
}

type ConflictPage struct {
	Items []IngestConflict
	Next  *AuditKey
}

type ImportFormat string

const (
	ImportJSON ImportFormat = "json"
	ImportCSV  ImportFormat = "csv"
)

// ImportFile is a file of the universal import with the fields of its form (ImportUpload).
type ImportFile struct {
	ProjectID     ID
	SchemaID      string
	SchemaVersion string
	Provider      Provider
	InstanceKey   string
	Snapshot      bool
	Format        ImportFormat
	Content       []byte
}
