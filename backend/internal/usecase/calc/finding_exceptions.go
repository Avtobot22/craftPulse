package calc

import (
	"encoding/json"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type exceptionIndex struct {
	byFinding map[entity.ID][]*entity.ExceptionVersion
	byRule    map[string][]*entity.ExceptionVersion
}

func indexExceptions(in *FindingMetricsInput, all []entity.ExceptionVersion) exceptionIndex {
	index := exceptionIndex{
		byFinding: make(map[entity.ID][]*entity.ExceptionVersion),
		byRule:    make(map[string][]*entity.ExceptionVersion),
	}
	for i := range all {
		v := &all[i]
		if appliesToFindingScope(in, v) && activeException(v, in.AsOf) {
			if v.Scope.FindingID.IsZero() {
				index.byRule[v.Scope.RuleID] = append(index.byRule[v.Scope.RuleID], v)
			} else {
				index.byFinding[v.Scope.FindingID] = append(index.byFinding[v.Scope.FindingID], v)
			}
		}
	}
	return index
}

func (index *exceptionIndex) match(f entity.FindingCoordinates) *entity.ExceptionVersion {
	for _, v := range index.byFinding[f.FindingID] {
		if v.Matches(f) {
			return v
		}
	}
	for _, v := range index.byRule[f.RuleID] {
		if v.Matches(f) {
			return v
		}
	}
	return nil
}

func appliesToFindingScope(in *FindingMetricsInput, v *entity.ExceptionVersion) bool {
	return v.FindingKind == in.Kind && v.Scope.AreaID == in.AreaID &&
		(in.ComponentID.IsZero() || v.Scope.ComponentID.IsZero() || v.Scope.ComponentID == in.ComponentID)
}

func activeException(v *entity.ExceptionVersion, at time.Time) bool {
	return v.Status == entity.ExceptionApproved && !at.Before(v.EffectiveFrom) && at.Before(v.ExpiresAt)
}

type exceptionEvidence struct {
	VersionID     entity.ID              `json:"version_id"`
	AuthorID      entity.ID              `json:"author_id"`
	Scope         exceptionScopeEvidence `json:"scope"`
	Type          string                 `json:"decision_type"`
	Reason        string                 `json:"reason"`
	EffectiveFrom string                 `json:"effective_from"`
	ExpiresAt     string                 `json:"expires_at"`
}

type exceptionScopeEvidence struct {
	AreaID      entity.ID `json:"area_id"`
	ComponentID entity.ID `json:"component_id,omitzero"`
	FindingID   entity.ID `json:"finding_id,omitzero"`
	Rule        string    `json:"rule_id,omitempty"`
	PathPrefix  string    `json:"path_prefix,omitempty"`
	PackagePURL string    `json:"package_purl,omitempty"`
}

func exceptionMetrics(in *FindingMetricsInput, versions []entity.ExceptionVersion, metricID string) []entity.SeriesPoint {
	active := snapshot(metricID, _estActive, in.AsOf, metricState{state: entity.StateAssessed})
	expired := snapshot(metricID, _estExpired, in.AsOf, metricState{state: entity.StateAssessed})
	for i := range versions {
		v := &versions[i]
		if !appliesToFindingScope(in, v) {
			continue
		}
		switch {
		case activeException(v, in.AsOf):
			active.add(exceptionContribution(v))
		case v.Status == entity.ExceptionExpired && !in.AsOf.Before(v.ExpiresAt):
			expired.add(exceptionContribution(v))
		}
	}
	return []entity.SeriesPoint{active.result(), expired.result()}
}

func exceptionContribution(v *entity.ExceptionVersion) entity.Contribution {
	s := v.Scope
	evidence, _ := json.Marshal(exceptionEvidence{
		VersionID: v.ID, AuthorID: v.AuthorID, Type: v.Type, Reason: v.Reason,
		Scope: exceptionScopeEvidence{
			AreaID: s.AreaID, ComponentID: s.ComponentID, FindingID: s.FindingID,
			Rule: s.RuleID, PathPrefix: s.PathPrefix, PackagePURL: s.PackagePURL,
		},
		EffectiveFrom: v.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		ExpiresAt:     v.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
	return entity.Contribution{
		ObjectID: v.DecisionID, ObjectKind: _objectException, ItemNo: 1, Role: entity.RoleNumerator,
		Value: num(1), Evidence: evidence,
	}
}
