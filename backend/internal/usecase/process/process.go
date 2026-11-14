// Package process keeps the settings of the process of a team: immutable versions of the mapping of statuses,
// resolutions and fields of its sources and of its working calendar, confirmed by the process owner and
// protected from a stale form and a repeated submission (TZ 10.3, 12.5.6; FR-007, FR-008, FR-017).
package process

import (
	"context"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

const (
	_operationConfirmMapping  = "process.mapping.confirm"
	_operationConfirmCalendar = "process.calendar.confirm"
)

// Teams is the org module: the source areas of the projects of a team.
type Teams interface {
	TeamAreas(ctx context.Context, orgID, teamID entity.ID, at time.Time) ([]entity.ID, error)
}

// Dictionaries is the normalize module: statuses, resolutions, types and fields of areas with their instances.
type Dictionaries interface {
	Dictionaries(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaDictionary, error)
}

// Auditor writes in the caller's transaction, so a confirmation without its record is not saved (DR-004).
type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Repo         repo.ProcessRepo
	Tenants      repo.TenantRepo
	Teams        Teams
	Dictionaries Dictionaries
	Auditor      Auditor
	Tx           repo.Transactor
	Clock        clock.Clock
	NewID        func() (entity.ID, error)
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

func errorf(format string, args ...any) error { return fmt.Errorf("process: "+format, args...) }
