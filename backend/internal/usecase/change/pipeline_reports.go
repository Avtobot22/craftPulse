package change

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// ExpectedPipelineReports reads matrix versions in force at the start of the pipeline, restricted to the
// repository and visible projects. A matrix confirmed after the run cannot change the run's expectations.
func (u *UseCase) ExpectedPipelineReports(ctx context.Context, scope entity.Scope, org, area entity.ID, at time.Time) ([]entity.ExpectedReport, error) {
	if !scope.Area(area) {
		return nil, entity.ErrNotFound
	}
	var reports []entity.ExpectedReport
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		reports, err = u.d.Repo.ExpectedPipelineReports(ctx, org, area, at, scope.Whole(), scope.Projects())
		return err
	})
	return reports, err
}
