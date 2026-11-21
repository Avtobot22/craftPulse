package normalize

import (
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _approvalSuffix = "/approval"

// _githubOutcomes maps the native status of GitHub Actions runs and jobs - the conclusion of a completed one, the
// status otherwise - to outcomes; the native value stays, so calc tells timed_out from failure. A neutral conclusion
// neither passes nor fails a check and is skipped; action_required and stale say nothing of the code and are unknown.
var _githubOutcomes = map[string]entity.CIOutcome{
	"queued":          entity.CIRunning,
	"requested":       entity.CIRunning,
	"waiting":         entity.CIRunning,
	"pending":         entity.CIRunning,
	"in_progress":     entity.CIRunning,
	"success":         entity.CISuccess,
	"failure":         entity.CIFailed,
	"timed_out":       entity.CIFailed,
	"startup_failure": entity.CIFailed,
	"cancelled":       entity.CICanceled,
	"skipped":         entity.CISkipped,
	"neutral":         entity.CISkipped,
}

// githubReview reads a review of a pull request by its state: an approval is a review and an approval, a request
// for changes is a review, and a review with comments only is a comment, not an answer of the review (FR-031). The
// approval it returns, if any, is an event of its own.
func (in *input) githubReview(c *mrEventCandidate) *mrEventCandidate {
	if in.provider != _providerGitHub || c.event.Kind != "review" {
		return nil
	}
	switch c.event.ReviewState {
	case "commented":
		c.event.Kind = "comment"
	case "approved":
		approval := *c
		approval.event.Kind, approval.event.EventKey = "approval", c.event.EventKey+_approvalSuffix
		return &approval
	}
	return nil
}
