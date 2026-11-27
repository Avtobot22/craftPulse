package entity

import "time"

// StepDay is the step of the daily series of events (A22).
const StepDay Step = "day"

const (
	// MetricSetReview is the set of the review metrics of MRs of a team or a repository (A05, A06, A07, A10).
	MetricSetReview = "mr_review"
	// MetricSetEvents is the set of the daily history of events (A22).
	MetricSetEvents = "events"
)

// ReviewWaitKind is what a ready MR waits for at a date (FR-031).
type ReviewWaitKind string

const (
	ReviewNoReviewer      ReviewWaitKind = "no_reviewer"
	ReviewAwaitingFirst   ReviewWaitKind = "awaiting_first_response"
	ReviewResponded       ReviewWaitKind = "responded"
	ReviewUnknown         ReviewWaitKind = "unknown"
	ReasonReviewerUnknown                = "reviewer_field_unavailable"
	// ReasonExternalResponse marks a point of A07 whose responses are comments: the source gives no reviews, so it
	// is the time to the first external response (FR-031 A3).
	ReasonExternalResponse = "first_external_response"
	// ReasonFallbackOpen marks a wait counted from the opening of an MR whose ready time is unknown (FR-031 A1).
	ReasonFallbackOpen = "fallback_open"
)

// ReviewWait is the wait of an MR for review at a date: Since is the ready time, or the opening with FallbackOpen,
// and for a responded MR the time of the response; WaitSeconds is up to the date or up to the response.
type ReviewWait struct {
	Kind         ReviewWaitKind
	Since        time.Time
	WaitSeconds  int64
	FallbackOpen bool
	// ResponseKind is the event kind of the first qualifying response: review, approval or comment.
	ResponseKind string
}

// ReviewFacts are the MRs of an area with their events, the events of its tasks and the kinds of the accounts
// that acted in them, all at one knowledge number. ReviewsGiven tells whether the source of the area gives reviews
// or approvals at all; without them the first external comment is the response.
type ReviewFacts struct {
	InstanceID   ID
	AreaID       ID
	MRs          []MRVersion
	MREvents     []MREvent
	MRRevisions  []MRRevision
	TaskEvents   []TaskEvent
	ActorKinds   map[ID]PersonKind
	ReviewsGiven bool
}

// ReviewScope is the scope of a series of reviews: a team through the repositories of its projects, or a repository.
type ReviewScope struct {
	TeamID       ID
	RepositoryID ID
}

// MRReview is the visible state and wait of a merge request at a consistent read cut.
type MRReview struct {
	Version    MRVersion
	Wait       *ReviewWait
	AuthorKind PersonKind
}

type MRQueueQuery struct {
	Project, Area *ID
	State, Sort   string
	Wait          ReviewWaitKind
	Limit         int
	After         *MRQueueCursor
}

type MRQueueCursor struct {
	AsOf        time.Time
	Seq         int64
	WaitSeconds int64
	OpenedAt    time.Time
	ID          ID
}

type MRQueuePage struct {
	Items []MRDetail
	Notes []AccessNote
	Next  *MRQueueCursor
}
