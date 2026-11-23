// Package decision owns the versions, transitions, and effective intervals of finding exceptions.
package decision

import (
	"context"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

const (
	_actionRead   = "findings.read"
	_actionManage = "exceptions.manage"
	_actionReview = "exceptions.review"
)

// Access checks the source and roles for the area and class saved with the decision.
type Access interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
	VisibleFindings(ctx context.Context, s entity.Subject, scope entity.Scope, operation string, targets []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

// Queue schedules one timer per decision and can advance it when a new boundary is earlier.
type Queue interface {
	Enqueue(ctx context.Context, job entity.Job, queue string) error
	EnqueueNearest(ctx context.Context, job entity.Job, queue string, at time.Time) error
}

// Auditor writes an audit entry in its mutation's transaction.
type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

// Deps contains the repositories and module interfaces used by decision.
type Deps struct {
	Repo    repo.DecisionRepo
	Tenants repo.TenantRepo
	Tx      repo.Transactor
	Queue   Queue
	Access  Access
	Auditor Auditor
	Clock   clock.Clock
	NewID   func() (entity.ID, error)
}

// UseCase stores and evaluates exceptions without consulting canonical finding storage.
type UseCase struct{ d Deps }

// New constructs the decision module.
func New(d Deps) *UseCase { return &UseCase{d: d} }

// CheckDraft validates and normalizes a draft before access checks and persistence.
func (*UseCase) CheckDraft(d *entity.ExceptionDraft) error {
	v := &entity.ValidationError{}
	switch d.Type {
	case entity.ExceptionFalsePositive, entity.ExceptionAcceptedRisk, entity.ExceptionNotApplicable:
	default:
		v.Add("/decision_type", entity.FieldNotAllowedValue, "unknown exception type")
	}
	d.Reason = strings.TrimSpace(d.Reason)
	if d.Reason == "" {
		v.Add("/reason", entity.FieldRequired, "a reason is required")
	} else if utf8.RuneCountInString(d.Reason) > 4000 || strings.ContainsRune(d.Reason, 0) {
		v.Add("/reason", entity.FieldOutOfRange, "reason must contain 1 to 4000 characters")
	}
	d.EffectiveFrom, d.ExpiresAt = stamp(d.EffectiveFrom), stamp(d.ExpiresAt)
	if d.EffectiveFrom.IsZero() || !d.EffectiveFrom.Before(d.ExpiresAt) {
		v.Add("/expires_at", entity.FieldOutOfRange, "expiry must be later than the start")
	}
	s := &d.Scope
	if s.AreaID.IsZero() {
		v.Add("/scope/area_id", entity.FieldRequired, "a source area is required")
	}
	s.RuleID = strings.TrimSpace(s.RuleID)
	if s.FindingID.IsZero() && s.RuleID == "" {
		v.Add("/scope", entity.FieldRequired, "a finding or rule is required")
	}
	if len(s.RuleID) > 255 || strings.ContainsRune(s.RuleID, 0) {
		v.Add("/scope/rule_id", entity.FieldOutOfRange, "invalid rule identifier")
	}
	if s.PathPrefix != "" {
		s.PathPrefix = strings.TrimSuffix(s.PathPrefix, "/")
		if !validPrefix(s.PathPrefix) {
			v.Add("/scope/path_prefix", entity.FieldNotAllowedValue, "expected a relative repository path")
		}
	}
	if len(s.PackagePURL) > 1024 || strings.ContainsRune(s.PackagePURL, 0) {
		v.Add("/scope/package_purl", entity.FieldOutOfRange, "invalid package identifier")
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}

func validPrefix(p string) bool {
	return p != "" && len(p) <= 1024 && p == path.Clean(p) && !path.IsAbs(p) && p != "." && p != ".." &&
		!strings.HasPrefix(p, "../") && !strings.ContainsRune(p, '\\') && !strings.ContainsRune(p, 0)
}

func stamp(at time.Time) time.Time { return at.UTC().Truncate(time.Microsecond) }

func allowedKind(kind string) bool {
	return kind == "code" || kind == "openapi" || entity.SensitiveFinding(kind)
}
