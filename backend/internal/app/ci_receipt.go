package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

// receiptIntake assembles the result link after ingest has checked receipt access.
// The destination applies its own session permissions when opened.
type receiptIntake struct {
	usecase.Intake
	tx        repo.Transactor
	facts     *normalizerepo.Repo
	summaries *changerepo.Repo
}

func withReceiptResults(intake usecase.Intake, pool *pgxpool.Pool, tx repo.Transactor) usecase.Intake {
	if intake == nil {
		return nil
	}
	return receiptIntake{
		Intake: intake, tx: tx, facts: normalizerepo.NewRepo(pool), summaries: changerepo.NewRepo(pool),
	}
}

func (u receiptIntake) Receipt(ctx context.Context, s entity.Subject, id entity.ID) (entity.Receipt, error) {
	rc, err := u.Intake.Receipt(ctx, s, id)
	if err != nil || rc.CIResultPending || rc.CIResult == nil {
		return rc, err
	}
	err = u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		pipeline, mr, head, err := u.facts.CIReceiptTarget(ctx, s.OrganizationID, rc.CIResult)
		if err != nil {
			return err
		}
		if mr != nil && entity.ValidGitSHA(head) {
			exists, err := u.summaries.HasBuiltSummary(ctx, s.OrganizationID, *mr, head)
			if err != nil {
				return err
			}
			if exists {
				rc.ResultURL = "/merge-requests/" + mr.String() + "/summary?head_sha=" + head
			}
		} else if pipeline != nil {
			rc.ResultURL = "/pipelines/" + pipeline.String()
		}
		// Normalization can finish before collection or the summary worker.
		rc.CIResultPending = rc.ResultURL == ""
		return nil
	})
	return rc, err
}
