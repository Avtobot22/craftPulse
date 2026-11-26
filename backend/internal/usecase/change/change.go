// Package change owns the technical results of a change (TZ 10.3): on M0 the matrix of checks of a project, the
// profile the step of CI reads by its upload token, the configuration snippet that adds the step to CI, the
// canonical findings across reports, the baselines of the main branch and MR summary revisions.
//
// A summary ranks up to five new findings by mapped severity (critical, high, medium, low, info,
// unknown). At equal severity it puts security first, then orders by analysis kind, finding ID and
// occurrence ID. Newly observed or accepted findings retain their separate lifecycle without a rank.
package change

import (
	"context"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Components is org: the confirmed components of the repositories of a project and the areas of projects; both
// run in the caller's organization transaction.
type Components interface {
	ConfirmedComponent(ctx context.Context, orgID, id entity.ID) (entity.Component, error)
	ProjectComponents(ctx context.Context, orgID, projectID entity.ID, at time.Time) ([]entity.Component, error)
	ProjectAreas(ctx context.Context, orgID entity.ID, at time.Time) ([]entity.Link, error)
}

// Roots is docs: the confirmed Git documentation roots of repositories.
type Roots interface {
	AreaRoots(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.DocumentationRoot, error)
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Summary       SummaryDeps
	TaskSummaries TaskSummaries
	Repo          repo.ChangeRepo
	Reports       Reports
	Decisions     Decisions
	FindingRights FindingRights
	Tenants       repo.TenantRepo
	Tx            repo.Transactor
	Components    Components
	Roots         Roots
	Auditor       Auditor
	Catalog       entity.CheckCatalog
	Clock         clock.Clock
	NewID         func() (entity.ID, error)
}

type UseCase struct {
	d        Deps
	profiles map[string]entity.CheckProfile
}

func New(d Deps) *UseCase {
	u := &UseCase{d: d, profiles: make(map[string]entity.CheckProfile, len(d.Catalog.Profiles))}
	for i := range d.Catalog.Profiles {
		u.profiles[d.Catalog.Profiles[i].ID] = d.Catalog.Profiles[i]
	}
	return u
}

func errorf(format string, args ...any) error {
	return fmt.Errorf("change: "+format, args...)
}
