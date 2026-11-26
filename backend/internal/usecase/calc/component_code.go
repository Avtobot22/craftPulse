package calc

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type CodeSources interface {
	CodeReportSources(context.Context, entity.ID, []entity.ID) (map[entity.ID]entity.CodeReportSource, error)
}

type codeStream struct {
	profile, producer, format string
	source                    entity.CodeReportSource
}

func (u *UseCase) ComponentCodeFacts(ctx context.Context, org entity.ID, component entity.Component, profiles []entity.ProfileEntry) ([]entity.ComponentCodeFact, error) {
	selected := map[string][]entity.CodeReportSource{}
	var ids []string
	for _, p := range profiles {
		if _, ok := selected[p.ProfileID]; !ok {
			ids = append(ids, p.ProfileID)
		}
		selected[p.ProfileID] = append(selected[p.ProfileID], entity.CodeReportSource{Mode: p.Mode, Language: p.Language})
	}
	var out []entity.ComponentCodeFact
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		out = nil
		revisions, err := u.d.Repo.ComponentCodeRevisions(ctx, org, component, ids)
		if err != nil || len(revisions) == 0 {
			return err
		}
		reports := make([]entity.ID, len(revisions))
		for i := range revisions {
			reports[i] = revisions[i].ReportID
		}
		sources, err := u.d.CodeSources.CodeReportSources(ctx, org, reports)
		if err != nil {
			return err
		}
		seen := map[codeStream]bool{}
		for i := range revisions {
			r := &revisions[i]
			source := sources[r.ReportID]
			key := codeStream{r.ProfileID, r.Producer, r.Format, source}
			if !slices.Contains(selected[r.ProfileID], source) || seen[key] {
				continue
			}
			points, err := u.d.Repo.RevisionPoints(ctx, org, r.ID)
			if err != nil {
				return err
			}
			seen[key] = true
			for i := range points {
				out = append(out, entity.ComponentCodeFact{SeriesPoint: points[i], Source: source.Mode, Window: r.Window})
			}
		}
		return nil
	})
	return out, err
}
