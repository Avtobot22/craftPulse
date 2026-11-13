package access

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// Reasons of a denied access in the audit (TZ 13.8).
const (
	ReasonOutsideScope = "outside_scope"
	ReasonNoPermission = "no_permission"
)

type rule int

const (
	ruleSession rule = iota + 1
	ruleAreaRead
	ruleAdministrator
	ruleUploadManage
)

// _rules is table 5.3 for the actions served so far; an action missing here is granted to nobody.
var _rules = map[string]rule{
	"session.read":         ruleSession,
	"session.manage":       ruleSession,
	"org.read":             ruleAreaRead,
	"org.manage":           ruleAdministrator,
	"users.manage":         ruleAdministrator,
	"installation.read":    ruleAdministrator,
	"audit.read":           ruleAdministrator,
	"connections.manage":   ruleAdministrator,
	"upload_tokens.manage": ruleUploadManage,
}

// Profiles is the org module; Profile runs in the caller's organization transaction.
type Profiles interface {
	Profile(ctx context.Context, orgID, userID entity.ID, at time.Time) (entity.Profile, error)
}

type Auditor interface {
	RecordSeparately(ctx context.Context, e entity.AuditEvent) error
}

type UseCase struct {
	tx       repo.Transactor
	profiles Profiles
	auditor  Auditor
	clock    clock.Clock
}

func New(tx repo.Transactor, p Profiles, a Auditor, c clock.Clock) *UseCase {
	return &UseCase{tx: tx, profiles: p, auditor: a, clock: c}
}

// Authorize returns entity.ErrNotFound when the area is not visible to the subject and entity.ErrForbidden
// when it is visible but no role grants the action (TZ 13.4); both are recorded in the audit. On success it
// returns the scope that read methods of modules take.
func (u *UseCase) Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error) {
	if !s.UploadTokenID.IsZero() {
		if r.Action != "ingest.upload" && r.Action != "receipts.read" {
			return entity.Scope{}, entity.ErrForbidden
		}
		if !r.Area.ID.IsZero() && (r.Area.Kind != entity.AreaProject || r.Area.ID != s.ProjectID) {
			return entity.Scope{}, entity.ErrNotFound
		}
		return entity.Scope{Projects: []entity.ID{s.ProjectID}}, nil
	}
	p, err := u.profile(ctx, s)
	if err != nil {
		return entity.Scope{}, err
	}
	reason, denial := decide(p, r)
	if denial == nil {
		return scope(p), nil
	}
	err = u.auditor.RecordSeparately(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID,
		ActorKind:      entity.ActorUser,
		ActorID:        s.UserID,
		Operation:      r.Operation,
		ObjectKind:     string(r.Area.Kind),
		ObjectID:       r.Area.ID,
		Outcome:        entity.AuditDenied,
		ReasonCode:     reason,
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		Details:        map[string]any{"action": r.Action},
	})
	if err != nil {
		return entity.Scope{}, err
	}
	return entity.Scope{}, denial
}

// CurrentUser lists the actions the roles grant in at least one area; the server still checks each request.
func (u *UseCase) CurrentUser(ctx context.Context, s entity.Subject) (entity.CurrentUser, error) {
	p, err := u.profile(ctx, s)
	if err != nil {
		return entity.CurrentUser{}, err
	}
	actions := make([]string, 0, len(_rules))
	for action := range _rules {
		if granted(p, action) {
			actions = append(actions, action)
		}
	}
	slices.Sort(actions)
	return entity.CurrentUser{Profile: p, Slug: s.OrganizationSlug, Actions: actions, Session: s}, nil
}

func (u *UseCase) profile(ctx context.Context, s entity.Subject) (entity.Profile, error) {
	var p entity.Profile
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		p, err = u.profiles.Profile(ctx, s.OrganizationID, s.UserID, u.clock.Now())
		return err
	})
	return p, err
}

func decide(p entity.Profile, r entity.AccessRequest) (string, error) {
	if !visible(p, r.Area) {
		return ReasonOutsideScope, entity.ErrNotFound
	}
	if r.Action == "upload_tokens.manage" && !r.Area.ID.IsZero() && !slices.Contains(p.UploadProjects, r.Area.ID) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if !granted(p, r.Action) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	return "", nil
}

func granted(p entity.Profile, action string) bool {
	switch _rules[action] {
	case ruleUploadManage:
		return len(p.UploadProjects) > 0
	case ruleSession:
		return true
	case ruleAreaRead:
		return p.Whole || len(p.Areas.Teams)+len(p.Areas.Projects)+len(p.Areas.Services) > 0
	case ruleAdministrator:
		return slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleAdministrator })
	}
	return false
}

// visible checks the area lists even for a whole-organization role: they hold every area of the organization,
// so an ID of another organization or of nothing gives the same 404.
func visible(p entity.Profile, a entity.AreaRef) bool {
	if a.ID.IsZero() {
		return true
	}
	var areas []entity.Area
	switch a.Kind {
	case entity.AreaTeam:
		areas = p.Areas.Teams
	case entity.AreaProject:
		areas = p.Areas.Projects
	case entity.AreaService:
		areas = p.Areas.Services
	}
	return slices.ContainsFunc(areas, func(x entity.Area) bool { return x.ID == a.ID })
}

func scope(p entity.Profile) entity.Scope {
	ids := func(areas []entity.Area) []entity.ID {
		out := make([]entity.ID, len(areas))
		for i, a := range areas {
			out[i] = a.ID
		}
		return out
	}
	return entity.Scope{Whole: p.Whole, Teams: ids(p.Areas.Teams), Projects: ids(p.Areas.Projects), Services: ids(p.Areas.Services)}
}
