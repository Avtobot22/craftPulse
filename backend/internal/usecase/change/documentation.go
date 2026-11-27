package change

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// OpenDocumentationViolations projects the canonical findings of the supplied
// root reports. The caller selects the last completed checks, plus any subsequent
// partial observations. An absent finding in a partial report stays open.
func (u *UseCase) OpenDocumentationViolations(ctx context.Context, org entity.ID, reports []entity.ID) (map[string]int, error) {
	counts := make(map[string]int)
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		occurrences, err := u.d.Reports.TechnicalOccurrences(ctx, org, reports)
		if err != nil {
			return err
		}
		var ids []entity.ID
		for _, report := range reports {
			for i := range occurrences[report] {
				o := &occurrences[report][i]
				if o.Path != "" && (o.Kind == entity.AnalysisDocs || o.Kind == entity.AnalysisOpenAPI) {
					ids = append(ids, o.ID)
				}
			}
		}
		if len(ids) == 0 {
			return nil
		}
		canonical, err := u.d.Repo.OccurrenceIdentities(ctx, org, ids)
		if err != nil {
			return err
		}
		byOccurrence := make(map[entity.ID]entity.ID, len(canonical))
		for _, c := range canonical {
			byOccurrence[c.OccurrenceID] = c.FindingIdentityID
		}
		seenID, seenFingerprint := map[entity.ID]bool{}, map[string]bool{}
		for _, report := range reports {
			for i := range occurrences[report] {
				o := &occurrences[report][i]
				if o.Path == "" || (o.Kind != entity.AnalysisDocs && o.Kind != entity.AnalysisOpenAPI) {
					continue
				}
				id, fp := byOccurrence[o.ID], string(o.Fingerprint)
				if (!id.IsZero() && seenID[id]) || seenFingerprint[fp] {
					continue
				}
				seenID[id], seenFingerprint[fp] = true, true
				counts[o.Path]++
			}
		}
		return nil
	})
	return counts, err
}
