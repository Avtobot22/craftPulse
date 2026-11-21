package calc

import (
	"encoding/json"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// covered requires a completed CI reconciliation over the point, never a tracker watermark or a connection-wide object count.
func (c *ciSource) covered(p period) bool {
	if len(c.areas) == 0 {
		return false
	}
	for _, area := range c.areas {
		found := false
		for i := range c.collection {
			s := &c.collection[i]
			if s.AreaID != area {
				continue
			}
			found = true
			if s.Completeness != entity.CollectionComplete || s.Watermark == nil || s.Watermark.Before(p.end) ||
				s.HistoryFrom != nil && s.HistoryFrom.After(p.start) || s.Reconcile == nil || s.Reconcile.DataKind != "ci" ||
				s.Reconcile.Outcome != entity.ReconcileComplete || s.Reconcile.FinishedAt == nil {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (c *ciSource) completeness(est string, p period) entity.Point {
	t := ciTally(p)
	var complete, loaded float64
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) {
			continue
		}
		loaded++
		role := entity.RoleDenominator
		if est == "loaded" {
			role = entity.RoleNumerator
		}
		ciAdd(t, r, nil, nil, role, num(1), "loaded_run")
		if r.complete {
			complete++
			if est != "loaded" {
				ciAdd(t, r, nil, nil, entity.RoleNumerator, num(1), "complete_attempts")
			}
		} else {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		}
	}
	covered := c.covered(p)
	pt := t.point()
	n := int(loaded)
	pt.N, pt.LowSample = &n, n < _lowSample
	if est == "loaded" {
		pt.State, pt.Value = entity.StateAssessed, num(loaded)
		if !covered {
			pt.State, pt.Value, pt.ReasonCode = entity.StateBounded, nil, "ci_total_unknown"
			pt.Bound = &entity.Bound{Kind: entity.BoundLower, Value: loaded}
		}
	} else if !covered {
		pt.State, pt.ReasonCode = entity.StateUnknown, "ci_total_unknown"
	} else if loaded == 0 {
		pt.State = entity.StateNotApplicable
	} else {
		pt.State, pt.Value, pt.Numerator, pt.Denominator = entity.StateAssessed, num(complete/loaded), num(complete), num(loaded)
	}
	for i := range c.collection {
		s := &c.collection[i]
		evidence, _ := json.Marshal(map[string]any{
			"area_id": s.AreaID, "connection_id": s.ConnectionID, "completeness": s.Completeness,
			"phase": s.Phase, "watermark": s.Watermark, "history_from": s.HistoryFrom, "reconciliation": s.Reconcile,
		})
		pt.Contributions = append(pt.Contributions, entity.Contribution{
			ObjectID: s.ConnectionID, ObjectKind: entity.ObjectConnection,
			ItemNo: len(pt.Contributions) + 1, Role: entity.RoleExcluded, ReasonCode: "ci_collection_state", Evidence: evidence,
		})
	}
	return pt
}
