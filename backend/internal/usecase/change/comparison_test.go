package change

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCompareReports_ComparabilityAndLifecycle(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		reason string
		change func(base, head *entity.TechnicalReport)
		states map[entity.ID]string
	}{
		{"V-25 complete pair", "", func(_, _ *entity.TechnicalReport) {}, states(entity.FindingResolved, entity.FindingUnchanged, entity.FindingNew)},
		{"V-25 missing base", _baseMissing, func(b, _ *entity.TechnicalReport) { b.SHA = "other" }, states("", entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"V-25 missing head", _headMissing, func(_, h *entity.TechnicalReport) { h.SHA = "old" }, states(entity.FindingOpen, entity.FindingOpen, "")},
		{"V-24 partial head", _scanIncomplete, func(_, h *entity.TechnicalReport) { h.Completeness = "partial" }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"V-25 timed out head", _scanIncomplete, func(_, h *entity.TechnicalReport) { h.ExitClass = "timeout" }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"V-25 canceled head", _scanIncomplete, func(_, h *entity.TechnicalReport) { h.Interrupted = true }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"V-19 changed rules", _rulesChanged, func(_, h *entity.TechnicalReport) { h.RulesHash = []byte{9} }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"changed profile", _profileChanged, func(_, h *entity.TechnicalReport) { h.ProfileID = "other" }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"changed producer", _profileChanged, func(_, h *entity.TechnicalReport) { h.Producer = "other" }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"changed scope", _scopeChanged, func(_, h *entity.TechnicalReport) { h.ScopeHash = []byte{9} }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
		{"missing provenance", _scanIncomplete, func(b, h *entity.TechnicalReport) { b.ConfigHash, h.ConfigHash = nil, nil }, states(entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingNewlyObserved)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, h, ids := comparisonFixture()
			q := entity.TechnicalComparisonQuery{BaseSHA: b.SHA, HeadSHA: h.SHA, Kind: entity.AnalysisCode}
			tt.change(&b, &h)
			out := CompareReports(q, &b, &h, ids)
			assert.Equal(t, tt.reason == "", out.Comparable)
			got := make(map[entity.ID]string)
			for _, f := range out.Findings {
				got[f.ID] = f.Lifecycle
			}
			assert.Equal(t, tt.states, got)
			if tt.reason == "" {
				assert.Equal(t, entity.TechnicalFactor{Factor: _codeChanged, NewCount: 1, ResolvedCount: 1, FindingIDs: []entity.ID{{1}, {3}}}, out.Factors[0])
				return
			}
			assert.Contains(t, out.IncomparableReasons, tt.reason)
			for _, f := range out.Factors {
				assert.Zero(t, f.NewCount)
				assert.Zero(t, f.ResolvedCount)
			}
		})
	}
}

func TestCompareReports_UncertainMoveKeepsBothCandidates(t *testing.T) {
	t.Parallel()
	b, h, ids := comparisonFixture()
	b.Occurrences[0].RuleID = "move"
	h.Occurrences[1].RuleID = "move"
	ids[3].MatchKind = entity.MatchUncertain
	q := entity.TechnicalComparisonQuery{BaseSHA: b.SHA, HeadSHA: h.SHA, Kind: entity.AnalysisCode}
	out := CompareReports(q, &b, &h, ids)
	assert.True(t, out.Comparable)
	assert.Equal(t, []entity.TechnicalFinding{
		{ID: entity.ID{1}, Kind: entity.FindingCode, Lifecycle: entity.FindingUncertain},
		{ID: entity.ID{2}, Kind: entity.FindingCode, Lifecycle: entity.FindingUnchanged},
		{ID: entity.ID{3}, Kind: entity.FindingCode, Lifecycle: entity.FindingUncertain},
	}, out.Findings)
	i := slices.IndexFunc(out.Factors, func(f entity.TechnicalFactor) bool { return f.Factor == _identityUncertain })
	require.GreaterOrEqual(t, i, 0)
	assert.Equal(t, []entity.ID{{1}, {3}}, out.Factors[i].FindingIDs)
	assert.Zero(t, out.Factors[i].NewCount)
	assert.Zero(t, out.Factors[i].ResolvedCount)
}

func TestCompareReports_DatabaseAndLockfileFactors(t *testing.T) {
	t.Parallel()
	b, h, ids := comparisonFixture()
	for i := range b.Occurrences {
		b.Occurrences[i].Kind = entity.FindingDependency
	}
	for i := range h.Occurrences {
		h.Occurrences[i].Kind = entity.FindingDependency
	}
	b.DBVersion, h.DBVersion = "db1", "db2"
	q := entity.TechnicalComparisonQuery{BaseSHA: b.SHA, HeadSHA: b.SHA, Kind: entity.AnalysisSCA}
	h.SHA = q.HeadSHA
	out := CompareReports(q, &b, &h, ids)
	assert.False(t, out.Comparable)
	assert.Equal(t, []entity.TechnicalFactor{{Factor: _dbChanged, NewCount: 1, ResolvedCount: 1, FindingIDs: []entity.ID{{1}, {3}}}}, out.Factors)
	assert.Equal(t, entity.FindingNewlyObserved, out.Findings[2].Lifecycle)

	h.SHA, h.DBVersion, h.DependencyDelta = "head", b.DBVersion, entity.DependencyDeltaComputed
	h.DependencyBaseSHA = b.SHA
	h.Occurrences[1].DependencyDelta = entity.DependencyChanged
	q.HeadSHA = h.SHA
	out = CompareReports(q, &b, &h, ids)
	assert.True(t, out.Comparable)
	assert.Equal(t, []entity.TechnicalFactor{
		{Factor: _codeChanged, ResolvedCount: 1, FindingIDs: []entity.ID{{1}}},
		{Factor: _dependencyChanged, NewCount: 1, FindingIDs: []entity.ID{{3}}},
	}, out.Factors)
	h.DependencyBaseSHA = "older"
	out = CompareReports(q, &b, &h, ids)
	assert.Equal(t, []entity.TechnicalFactor{{Factor: _codeChanged, NewCount: 1, ResolvedCount: 1, FindingIDs: []entity.ID{{1}, {3}}}}, out.Factors)
}

func states(a, b, c string) map[entity.ID]string {
	out := make(map[entity.ID]string)
	for i, state := range []string{a, b, c} {
		if state != "" {
			out[entity.ID{byte(i + 1)}] = state
		}
	}
	return out
}

func comparisonFixture() (base, head entity.TechnicalReport, identities []entity.OccurrenceIdentity) {
	base = entity.TechnicalReport{
		ID: entity.ID{10}, SHA: "base", ProfileID: "tool-default", Producer: "tool", ProducerVersion: "1", Mode: "builtin",
		RulesHash: []byte{1}, ConfigHash: []byte{2}, ScopeHash: []byte{3}, Completeness: "complete", ExitClass: "findings",
	}
	head = base
	head.ID, head.SHA = entity.ID{11}, "head"
	occ := func(id byte, rule string) entity.ReportOccurrence {
		return entity.ReportOccurrence{ID: entity.ID{id}, Kind: entity.FindingCode, RuleID: rule}
	}
	base.Occurrences = []entity.ReportOccurrence{occ(21, "removed"), occ(22, "present")}
	head.Occurrences = []entity.ReportOccurrence{occ(23, "present"), occ(24, "added")}
	identities = []entity.OccurrenceIdentity{
		{OccurrenceID: entity.ID{21}, FindingIdentityID: entity.ID{1}, MatchKind: entity.MatchExact},
		{OccurrenceID: entity.ID{22}, FindingIdentityID: entity.ID{2}, MatchKind: entity.MatchExact},
		{OccurrenceID: entity.ID{23}, FindingIdentityID: entity.ID{2}, MatchKind: entity.MatchExact},
		{OccurrenceID: entity.ID{24}, FindingIdentityID: entity.ID{3}, MatchKind: entity.MatchExact},
	}
	return base, head, identities
}
