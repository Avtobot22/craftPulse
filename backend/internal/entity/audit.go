package entity

import (
	"errors"
	"time"
)

type ActorKind string

const (
	ActorUser        ActorKind = "user"
	ActorAPIKey      ActorKind = "api_key"
	ActorUploadToken ActorKind = "upload_token"
	ActorSystem      ActorKind = "system"
	ActorSource      ActorKind = "source"
)

type AuditOutcome string

const (
	AuditSucceeded AuditOutcome = "succeeded"
	AuditDenied    AuditOutcome = "denied"
	AuditFailed    AuditOutcome = "failed"
)

const AuditDetailsLimit = 4096

var (
	ErrAuditDetailsTooLarge = errors.New("audit: details exceed 4096 bytes")
	ErrAuditForbiddenDetail = errors.New("audit: details must not carry tokens, state, nonce, session IDs or credentials")
)

// AuditEvent is one record of audit.event (TZ 13.8); zero IDs and empty strings are stored as NULL.
type AuditEvent struct {
	OrganizationID ID
	OccurredAt     time.Time
	ActorKind      ActorKind
	ActorID        ID
	Operation      string
	ObjectKind     string
	ObjectID       ID
	AreaID         ID
	Outcome        AuditOutcome
	ReasonCode     string
	BaseVersion    string
	NewVersion     string
	CorrelationID  ID
	Details        map[string]any
}
