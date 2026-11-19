package entity

import (
	"encoding/json"
	"time"
)

// SavedView keeps the UI query, never a reader's permissions or a cached report (FR-023).
type SavedView struct {
	ID, OrganizationID, OwnerUserID ID
	Name                            string
	SeriesID                        ID
	Filters                         json.RawMessage
	PinnedRevisionID                *ID
	CreatedAt                       time.Time
}

type SavedViewKey struct {
	CreatedAt time.Time
	ID        ID
}

type SavedViewQuery struct {
	Limit int
	After *SavedViewKey
}

type SavedViewPage struct {
	Items []SavedView
	Next  *SavedViewKey
}

// OpenSavedView is the descriptor and the report resolved for this reader and this revision.
type OpenSavedView struct {
	View   SavedView
	Query  ReportQuery
	Report TeamReport
}
