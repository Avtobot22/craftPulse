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
	ReasonCode       string
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
