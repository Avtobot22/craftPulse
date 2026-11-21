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
	ReasonOutsideScope      = "outside_scope"
	ReasonNoPermission      = "no_permission"
	ReasonSourceDenied      = "source_denied"
	ReasonSourceUnavailable = "source_check_unavailable"
	ReasonSourceAccessLost  = "source_access_lost"
)

type rule int

const (
	ruleSession rule = iota + 1
	ruleAreaRead
	ruleAdministrator
	// ruleProjectLead is an administrator or a tech lead of the project: upload tokens and imports.
	ruleProjectLead
	// ruleReceipts is every administrator and a project lead for the receipts of their projects.
	ruleReceipts
	// ruleProcessOwner is the process owner of the team of the request; no other role stands in for it.
	ruleProcessOwner
	ruleServiceOwner
	// ruleProjectManager is a project manager of the project; the scope holds only the projects of that role.
	ruleProjectManager
	// ruleTechLead is a tech lead of the project; an administrator does not stand in for it.
	ruleTechLead
)

// _rules is table 5.3 for the actions served so far; an action missing here is granted to nobody.
var _rules = map[string]rule{
	"profile.manage":        ruleSession,
	"session.read":          ruleSession,
	"session.manage":        ruleSession,
	"org.read":              ruleAreaRead,
	"report.read":           ruleAreaRead,
	"mr.read":               ruleAreaRead,
	"ci.read":               ruleAreaRead,
	"org.manage":            ruleAdministrator,
	"users.manage":          ruleAdministrator,
	"installation.read":     ruleAdministrator,
	"audit.read":            ruleAdministrator,
	"connections.manage":    ruleAdministrator,
	"accounts.manage":       ruleAdministrator,
	"upload_tokens.manage":  ruleProjectLead,
	"imports.create":        ruleProjectLead,
	"receipts.read":         ruleReceipts,
	"sync.read":             ruleAdministrator,
	"process.manage":        ruleProcessOwner,
	"docs.read":             ruleAreaRead,
	"docs.manage":           ruleServiceOwner,
	"links.decide":          ruleProjectManager,
	"check_profiles.manage": ruleTechLead,
}

// Profiles is the org module; Profile runs in the caller's organization transaction.
type Profiles interface {
	Profile(ctx context.Context, orgID, userID entity.ID, at time.Time) (entity.Profile, error)
}

type Auditor interface {
	RecordSeparately(ctx context.Context, e entity.AuditEvent) error
}

// Structure is the org module; ProjectAreas runs in the caller's organization transaction.
type Structure interface {
	ProjectAreas(ctx context.Context, orgID entity.ID, at time.Time) ([]entity.Link, error)
}

// Accounts is the org module: the account of a user in an instance by the confirmed link of their person.
type Accounts interface {
	UserSourceAccount(ctx context.Context, orgID, userID, instanceID entity.ID) (entity.UserAccount, bool, error)
}

// Sources is the connect module, the only caller of adapters (TZ 10.3).
type Sources interface {
	AreaSources(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaSource, error)
	CheckAccess(ctx context.Context, orgID, connectionID entity.ID, in entity.SourceAccessInput) (entity.SourceAccessResult, error)
}

// Metrics count the lookups of the cache of decisions and the failed checks of every connection.
type Metrics interface {
	CacheLookup(hit bool)
	CheckFailed(connectionID entity.ID)
}

type Deps struct {
	LinkAuditor LinkAuditor
	Links       LinkSources
	LinkRepo    repo.SourceLinkRepo
	Tx          repo.Transactor
	Profiles    Profiles
	Structure   Structure
	Accounts    Accounts
	Sources     Sources
	Auditor     Auditor
	Metrics     Metrics
	Clock       clock.Clock
	// CacheSize bounds the decisions kept in memory; _cacheSize when zero.
	CacheSize int
	// CheckTimeout is the budget of one CheckAccess on the path of a read; _checkTimeout when zero.
	CheckTimeout time.Duration
}

type UseCase struct {
	linkAuditor LinkAuditor
	links       LinkSources
	linkRepo    repo.SourceLinkRepo
	tx          repo.Transactor
	profiles    Profiles
	structure   Structure
	accounts    Accounts
	sources     Sources
	auditor     Auditor
	metrics     Metrics
	clock       clock.Clock
	checks      *checks
}

func New(d Deps) *UseCase {
	if d.CacheSize == 0 {
		d.CacheSize = _cacheSize
	}
	if d.CheckTimeout == 0 {
		d.CheckTimeout = _checkTimeout
	}
	if d.Metrics == nil {
		d.Metrics = noMetrics{}
	}
	u := &UseCase{
		linkAuditor: d.LinkAuditor, links: d.Links, linkRepo: d.LinkRepo, tx: d.Tx, profiles: d.Profiles, structure: d.Structure, accounts: d.Accounts, sources: d.Sources,
		auditor: d.Auditor, metrics: d.Metrics, clock: d.Clock,
		checks: newChecks(d.Sources, d.Metrics, d.Clock, d.CacheSize, d.CheckTimeout),
	}
	u.checks.expired = u.expireLink
	return u
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
		return entity.GrantScope(entity.ScopeGrant{Projects: []entity.ID{s.ProjectID}}), nil
	}
	p, err := u.profile(ctx, s)
	if err != nil {
		return entity.Scope{}, err
	}
	reason, denial := decide(p, r)
	if denial == nil && _rules[r.Action] == ruleProjectManager {
		return managedScope(p), nil
	}
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
	if rule := _rules[r.Action]; (rule == ruleProjectLead || rule == ruleReceipts) && !r.Area.ID.IsZero() && !slices.Contains(p.UploadProjects, r.Area.ID) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if _rules[r.Action] == ruleTechLead && !r.Area.ID.IsZero() && !slices.Contains(p.LeadProjects, r.Area.ID) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if _rules[r.Action] == ruleProcessOwner && !ownsProcess(p, r.Area) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if _rules[r.Action] == ruleProjectManager && !r.Area.ID.IsZero() && !managesProject(p, r.Area) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if _rules[r.Action] == ruleServiceOwner && !ownsService(p, r.Area) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	if !granted(p, r.Action) {
		return ReasonNoPermission, entity.ErrForbidden
	}
	return "", nil
}

// ownsProcess tells whether the subject is the process owner of the team of the request.
func ownsProcess(p entity.Profile, a entity.AreaRef) bool {
	return a.Kind == entity.AreaTeam && slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool {
		return r.Role == entity.RoleProcessOwner && r.TeamID == a.ID
	})
}

func ownsService(p entity.Profile, a entity.AreaRef) bool {
	return a.Kind == entity.AreaService && slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool {
		return r.Role == entity.RoleServiceOwner && r.ServiceID == a.ID
	})
}

// managesProject tells whether the subject is a project manager of the project of the request or of every project.
func managesProject(p entity.Profile, a entity.AreaRef) bool {
	return a.Kind == entity.AreaProject && slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool {
		return r.Role == entity.RoleProjectManager && (r.ProjectID == a.ID || r.ProjectID.IsZero() && r.TeamID.IsZero() && r.ServiceID.IsZero())
	})
}

// managedScope narrows the scope to the projects of the project manager role, so source checks see only their areas.
func managedScope(p entity.Profile) entity.Scope {
	var projects []entity.ID
	for _, r := range p.Roles {
		if r.Role != entity.RoleProjectManager {
			continue
		}
		if r.ProjectID.IsZero() && r.TeamID.IsZero() && r.ServiceID.IsZero() {
			return scope(p)
		}
		if !r.ProjectID.IsZero() {
			projects = append(projects, r.ProjectID)
		}
	}
	return entity.GrantScope(entity.ScopeGrant{Projects: projects})
}

func granted(p entity.Profile, action string) bool {
	switch _rules[action] {
	case ruleProjectLead:
		return len(p.UploadProjects) > 0
	case ruleTechLead:
		return len(p.LeadProjects) > 0
	case ruleReceipts:
		return len(p.UploadProjects) > 0 || slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleAdministrator })
	case ruleSession:
		return true
	case ruleAreaRead:
		return p.Whole || len(p.Areas.Teams)+len(p.Areas.Projects)+len(p.Areas.Services) > 0
	case ruleAdministrator:
		return slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleAdministrator })
	case ruleServiceOwner:
		return slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleServiceOwner })
	case ruleProcessOwner:
		return slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleProcessOwner })
	case ruleProjectManager:
		return slices.ContainsFunc(p.Roles, func(r entity.RoleAssignment) bool { return r.Role == entity.RoleProjectManager })
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
	return entity.GrantScope(entity.ScopeGrant{
		Whole: p.Whole, Teams: ids(p.Areas.Teams), Projects: ids(p.Areas.Projects), Services: ids(p.Areas.Services),
	})
}
