package calc

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// WikiCollection supplies the source manifest result from sync.
type WikiCollection interface {
	WikiCollectionAt(context.Context, entity.ID, entity.ID, int64, time.Time) (entity.WikiCollection, error)
}

func wikiPoints(p period, wiki entity.WikiCollection) []entity.SeriesPoint {
	coverage := analysisPoint(p, "D27", "", UnitRatio)
	coverage.ReasonCode = "wiki_reconciliation_missing"
	switch {
	case wiki.Failed:
		coverage.ReasonCode = "wiki_reconciliation_failed"
	case !wiki.RunID.IsZero():
		analysisRatio(&coverage.Point, wiki.Read, wiki.Expected)
		if wiki.Read < wiki.Expected {
			coverage.State, coverage.ReasonCode = entity.StateProvisional, "wiki_pages_unread"
		}
		for i, c := range []struct {
			role  entity.ContributionRole
			value *float64
		}{{entity.RoleNumerator, coverage.Numerator}, {entity.RoleDenominator, coverage.Denominator}} {
			coverage.Contributions = append(coverage.Contributions, entity.Contribution{
				ObjectKind: entity.ObjectConnection, ObjectID: wiki.ConnectionID, Role: c.role, ItemNo: i + 1, Value: c.value,
			})
		}
	}
	last := wikiTimestamp(p, "last_success", "wiki_sync_unknown", wiki.LastSuccess)
	first := wikiTimestamp(p, "earliest_version", "document_version_unknown", wiki.EarliestVersion)
	return []entity.SeriesPoint{coverage, last, first}
}

func wikiTimestamp(p period, estimator, reason string, at *time.Time) entity.SeriesPoint {
	point := analysisPoint(p, "D27", estimator, UnitSeconds)
	point.ReasonCode = reason
	if at != nil {
		point.State, point.ReasonCode, point.Value = entity.StateAssessed, "", num(timestampSeconds(*at))
	}
	return point
}
