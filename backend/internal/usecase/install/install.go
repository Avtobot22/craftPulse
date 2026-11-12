package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _operationCreate = "organization.create"

type Organizations interface {
	CreateOrganization(ctx context.Context, o entity.NewOrganization) error
}

type Logins interface {
	OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error)
	RegisterLogin(ctx context.Context, orgID entity.ID, slug string, p entity.OIDCProvider) error
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Tx            repo.Transactor
	Repo          repo.InstallRepo
	Organizations Organizations
	Logins        Logins
	Auditor       Auditor
	Site          repo.SiteProbe
	Identity      repo.IdentityProviderProbe
	ObjectStore   repo.ObjectStoreProbe
	Clock         clock.Clock
	NewID         func() (entity.ID, error)
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

// Apply creates the organizations of the installation parameters. An organization whose login route
// already exists is left as it is, so a repeated run neither duplicates it nor changes its first admin.
func (u *UseCase) Apply(ctx context.Context, params entity.InstallParams) ([]entity.AppliedOrganization, error) {
	applied := make([]entity.AppliedOrganization, 0, len(params.Organizations))
	for _, o := range params.Organizations {
		id, found, err := u.d.Logins.OrganizationBySlug(ctx, o.Slug)
		if err != nil {
			return applied, err
		}
		if found {
			applied = append(applied, entity.AppliedOrganization{Slug: o.Slug, ID: id})
			continue
		}
		if o.ID, err = u.d.NewID(); err != nil {
			return applied, fmt.Errorf("install: new organization ID: %w", err)
		}
		if err := u.d.Tx.InOrgTx(ctx, o.ID, func(ctx context.Context) error { return u.create(ctx, o) }); err != nil {
			return applied, fmt.Errorf("install: organization %s: %w", o.Slug, err)
		}
		applied = append(applied, entity.AppliedOrganization{Slug: o.Slug, ID: o.ID, Created: true})
	}
	return applied, nil
}

func (u *UseCase) create(ctx context.Context, o entity.NewOrganization) error {
	if err := u.d.Organizations.CreateOrganization(ctx, o); err != nil {
		return err
	}
	if err := u.d.Logins.RegisterLogin(ctx, o.ID, o.Slug, o.Provider); err != nil {
		return err
	}
	return u.d.Auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: o.ID,
		ActorKind:      entity.ActorSystem,
		Operation:      _operationCreate,
		ObjectKind:     "organization",
		ObjectID:       o.ID,
		Outcome:        entity.AuditSucceeded,
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		Details:        map[string]any{"slug": o.Slug, "issuer": o.Provider.Issuer, "first_admin_subject": o.FirstAdminSubject},
	})
}

// Check runs every installation check, stores the protocol and returns it (FR-011).
func (u *UseCase) Check(ctx context.Context, params entity.InstallParams) (entity.Protocol, error) {
	p := entity.Protocol{StartedAt: u.d.Clock.Now(), ReleaseSHA: params.ReleaseSHA}
	facts, factsErr := u.d.Repo.DatabaseFacts(ctx)
	manifest, err := json.Marshal(map[string]any{
		"release_sha": params.ReleaseSHA, "images": params.Images, "schema_version": facts.ExpectedSchema,
	})
	if err != nil {
		return p, fmt.Errorf("install: encode manifest: %w", err)
	}
	if p.ID, err = u.d.Repo.CreateProtocol(ctx, p.StartedAt, params.ReleaseSHA, manifest); err != nil {
		return p, errors.Join(err, factsErr)
	}

	p.Checks = []entity.CheckResult{
		u.checkSite(ctx, params.SiteURL),
		u.checkIdentityProviders(ctx, params.Organizations),
		checkDatabase(facts, factsErr),
		u.checkObjectStore(ctx),
		skipped(entity.CheckAdapter, "no built-in adapters are configured"),
		skipped(entity.CheckAdapterStore, "no adapter credential stores are configured"),
		skipped(entity.CheckUpdateSource, "no update source of vulnerability databases and rules is configured"),
		checkRunnerNetwork(params.RunnerNetwork),
	}
	p.Outcome = entity.ProtocolPassed
	for _, c := range p.Checks {
		if err := u.d.Repo.AddCheckResult(ctx, p.ID, c); err != nil {
			return p, err
		}
		if c.Outcome == entity.CheckFailed {
			p.Outcome = entity.ProtocolFailed
		}
	}
	p.FinishedAt = u.d.Clock.Now()
	return p, u.d.Repo.FinishProtocol(ctx, p.ID, p.FinishedAt, p.Outcome)
}

func (u *UseCase) checkSite(ctx context.Context, url string) entity.CheckResult {
	return fromProbe(entity.CheckSite, url, u.d.Site.ProbeSite(ctx, url))
}

// checkIdentityProviders folds every organization into one row: the protocol keeps one result per check kind.
func (u *UseCase) checkIdentityProviders(ctx context.Context, orgs []entity.NewOrganization) entity.CheckResult {
	res := entity.CheckResult{Kind: entity.CheckIdentityProvider, Outcome: entity.CheckPassed}
	lines := make([]string, 0, len(orgs))
	for _, o := range orgs {
		r := fromProbe(entity.CheckIdentityProvider, o.Provider.Issuer, u.d.Identity.ProbeIssuer(ctx, o.Provider.Issuer))
		lines = append(lines, o.Slug+": "+r.Detail)
		if r.Outcome == entity.CheckFailed && res.Outcome == entity.CheckPassed {
			res.Outcome, res.Reason = entity.CheckFailed, r.Reason
		}
	}
	res.Detail = strings.Join(lines, "; ")
	return res
}

func (u *UseCase) checkObjectStore(ctx context.Context) entity.CheckResult {
	if err := u.d.ObjectStore.Check(ctx); err != nil {
		return entity.CheckResult{
			Kind: entity.CheckObjectStore, Outcome: entity.CheckFailed,
			Reason: entity.ReasonUnavailable, Detail: err.Error(),
		}
	}
	return entity.CheckResult{Kind: entity.CheckObjectStore, Outcome: entity.CheckPassed, Detail: "bucket is reachable"}
}

func checkDatabase(f entity.DatabaseFacts, err error) entity.CheckResult {
	res := entity.CheckResult{Kind: entity.CheckDatabase, Outcome: entity.CheckFailed}
	switch {
	case err != nil:
		res.Reason, res.Detail = entity.ReasonUnavailable, err.Error()
	case !supportedLocale(f):
		res.Reason, res.Detail = entity.ReasonLocaleUnsupported,
			fmt.Sprintf("locale %q of provider %q, expected builtin C.UTF-8 or C", f.Locale, f.LocaleProvider)
	case !f.HasBtreeGist:
		res.Reason, res.Detail = entity.ReasonExtensionMissing, "extension btree_gist is not installed"
	case f.SchemaVersion < f.ExpectedSchema:
		res.Reason, res.Detail = entity.ReasonSchemaOutdated,
			fmt.Sprintf("schema version %d is below %d, run api migrate", f.SchemaVersion, f.ExpectedSchema)
	default:
		res.Outcome = entity.CheckPassed
		res.Detail = fmt.Sprintf("locale %s, btree_gist installed, schema version %d", f.Locale, f.SchemaVersion)
	}
	return res
}

// supportedLocale is the builtin C.UTF-8 or, on a managed database without the builtin provider, C (TZ 12.2).
func supportedLocale(f entity.DatabaseFacts) bool {
	return f.Locale == "C" || f.LocaleProvider == "b" && f.Locale == "C.UTF-8"
}

func checkRunnerNetwork(prefixes []string) entity.CheckResult {
	if len(prefixes) == 0 {
		return skipped(entity.CheckRunnerNetwork, "RUNNER_NETWORK is not set, CI packages are not accepted")
	}
	return entity.CheckResult{
		Kind: entity.CheckRunnerNetwork, Outcome: entity.CheckPassed,
		Detail: "CI packages are accepted from " + strings.Join(prefixes, ", "),
	}
}

func skipped(kind entity.CheckKind, detail string) entity.CheckResult {
	return entity.CheckResult{Kind: kind, Outcome: entity.CheckSkipped, Reason: entity.ReasonNotConfigured, Detail: detail}
}

func fromProbe(kind entity.CheckKind, target string, err error) entity.CheckResult {
	if err == nil {
		return entity.CheckResult{Kind: kind, Outcome: entity.CheckPassed, Detail: target + " is reachable"}
	}
	if f, ok := errors.AsType[*entity.CheckFailure](err); ok {
		return entity.CheckResult{Kind: kind, Outcome: entity.CheckFailed, Reason: f.Reason, Detail: f.Detail}
	}
	return entity.CheckResult{Kind: kind, Outcome: entity.CheckFailed, Reason: entity.ReasonUnavailable, Detail: err.Error()}
}
