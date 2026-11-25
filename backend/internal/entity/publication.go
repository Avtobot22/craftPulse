package entity

import (
	"errors"
	"time"
)

const (
	MRComment                = "mr_comment"
	PublicationSucceeded     = "succeeded"
	PublicationFailed        = "failed"
	PublicationUnknown       = "unknown"
	PublicationStale         = "stale"
	WritePermissionMissing   = "write_permission_missing"
	PublicationProviderError = "provider_error"
	PublicationHeadChanged   = "head_changed"
)

// ErrPublicationHeadChanged means the stored or source head differs from the selected revision.
var ErrPublicationHeadChanged = errors.New("publication: head changed")

// Publication is the outcome recorded against a single summary revision.
type Publication struct {
	Status, ExternalID, ErrorCode string
	PublishedAt                   *time.Time
}

type PublicationTarget struct {
	Kind, Marker string
	Object       ObjectKey
}

type PublishInput struct {
	PublicationTarget
	ExternalID, HeadSHA, Markdown string
}

// SummaryPublication carries the normalized MR and the revision selected for delivery.
type SummaryPublication struct {
	MR           SummaryMR
	Revision     SummaryRevision
	ConnectionID ID
}

type SummarySignalLocation struct {
	Rule    string `json:"rule"`
	Path    string `json:"path,omitempty"`
	Package string `json:"package,omitempty"`
	Line    *int   `json:"line,omitempty"`
}
