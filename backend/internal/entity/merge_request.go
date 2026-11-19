package entity

import "time"

type RepositoryVersion struct {
	ID           ID
	RepositoryID ID
	Knowledge
	Record          RecordRef
	Revision        Revision
	ObservedAt      time.Time
	SourceUpdatedAt *time.Time
	Path            string
	Name            string
	DefaultBranch   string
	Visibility      string
	WebURL          string
}

type MRVersion struct {
	ID     ID
	MRID   ID
	AreaID ID
	Knowledge
	Record             RecordRef
	Revision           Revision
	ObservedAt         time.Time
	SourceUpdatedAt    *time.Time
	IID                *int
	Title              string
	State              string
	Draft              bool
	AuthorAccountID    *ID
	SourceBranch       string
	TargetBranch       string
	HeadSHA            string
	BaseSHA            string
	MergeBaseSHA       string
	ReviewerField      bool
	ReviewerAccountIDs []ID
	OpenedAt           time.Time
	ReadyAt            *time.Time
	MergedAt           *time.Time
	ClosedAt           *time.Time
}

type MRRevision struct {
	ID          ID
	MRID        ID
	RevisionKey string
	Knowledge
	Record     RecordRef
	Revision   Revision
	HeadSHA    string
	BaseSHA    string
	StartSHA   string
	CreatedAt  time.Time
	ObservedAt time.Time
}

type MREvent struct {
	ID       ID
	MRID     ID
	EventKey string
	Knowledge
	Record          RecordRef
	Revision        Revision
	Kind            string
	ActorAccountID  *ID
	TargetAccountID *ID
	HeadSHA         string
	ReviewState     string
	EventTime       time.Time
	EventOffsetS    int
	ObservedAt      time.Time
	ObservedVia     ObservedVia
	// ActorKind comes only from the confirmed assignment at the requested knowledge slice.
	ActorKind PersonKind
}

type MRFactKey struct {
	MRID ID
	Key  string
}
