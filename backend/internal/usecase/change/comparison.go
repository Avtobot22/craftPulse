package change

import (
	"bytes"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_profileChanged = "profile_changed"
	_rulesChanged   = "rules_changed"
	_scopeChanged   = "scope_changed"
	_baseMissing    = "base_scan_missing"
	_headMissing    = "head_scan_missing"
	_scanIncomplete = "scan_incomplete"

	_codeChanged       = "code_changed"
	_dependencyChanged = "dependency_changed"
	_dbChanged         = "db_changed"
	_sourceRecovered   = "source_recovered"
	_sourceLost        = "source_lost"
	_identityUncertain = "identity_uncertain"
)

// CompareReports compares two reports with canonical identities already assigned by change.identify.
func CompareReports(q entity.TechnicalComparisonQuery, base, head *entity.TechnicalReport, identities []entity.OccurrenceIdentity) entity.TechnicalComparison {
	if base != nil && base.SHA != q.BaseSHA {
		base = nil
	}
	if head != nil && head.SHA != q.HeadSHA {
		head = nil
	}
	out := entity.TechnicalComparison{Base: technicalRevision(q.BaseSHA, base), Head: technicalRevision(q.HeadSHA, head)}
	causes := reportCauses(q.Kind, base, head)
	out.IncomparableReasons = causes.reasons
	out.Comparable = len(causes.reasons) == 0
	byID := make(map[entity.ID]entity.OccurrenceIdentity, len(identities))
	for _, id := range identities {
		byID[id.OccurrenceID] = id
	}
	b, h := comparisonFindings(base, byID), comparisonFindings(head, byID)
	return compareFindings(out, base, head, &causes, &b, &h)
}

func technicalRevision(sha string, r *entity.TechnicalReport) entity.TechnicalRevision {
	out := entity.TechnicalRevision{SHA: sha}
	if r != nil && !r.Rejected {
		out.ReportID = &r.ID
	}
	return out
}

type comparisonCauses struct {
	reasons []string
	factors map[string]*entity.TechnicalFactor
	dbOnly  bool
}

func (c *comparisonCauses) factor(kind string) *entity.TechnicalFactor {
	if c.factors[kind] == nil {
		c.factors[kind] = &entity.TechnicalFactor{Factor: kind}
	}
	return c.factors[kind]
}

func reportCauses(kind string, base, head *entity.TechnicalReport) comparisonCauses {
	out := comparisonCauses{factors: make(map[string]*entity.TechnicalFactor)}
	if base == nil {
		out.reasons = append(out.reasons, _baseMissing)
	}
	if head == nil {
		out.reasons = append(out.reasons, _headMissing)
	}
	if base != nil && !base.Complete() || head != nil && !head.Complete() {
		out.reasons = append(out.reasons, _scanIncomplete)
	}
	if !base.Complete() && head.Complete() {
		out.factor(_sourceRecovered)
	}
	if base.Complete() && !head.Complete() {
		out.factor(_sourceLost)
	}
	if base == nil || head == nil || base.Rejected || head.Rejected {
		return out
	}
	out.compareProvenance(kind, base, head)
	return out
}

func (c *comparisonCauses) compareProvenance(kind string, b, h *entity.TechnicalReport) {
	profile := b.Producer != h.Producer || b.ProfileID != h.ProfileID || b.Mode != h.Mode
	rules := b.ProducerVersion != h.ProducerVersion || !bytes.Equal(b.RulesHash, h.RulesHash) || !bytes.Equal(b.ConfigHash, h.ConfigHash)
	scope := b.Language != h.Language || !bytes.Equal(b.ScopeHash, h.ScopeHash)
	db := b.DBVersion != h.DBVersion
	if profile {
		c.reasons = append(c.reasons, _profileChanged)
	}
	if rules || db {
		c.reasons = append(c.reasons, _rulesChanged)
	}
	if scope {
		c.reasons = append(c.reasons, _scopeChanged)
	}
	if rules {
		c.factor(_rulesChanged)
	}
	if db {
		c.factor(_dbChanged)
	}
	if scope {
		c.factor(_scopeChanged)
	}
	if b.SHA != h.SHA {
		c.factor(_codeChanged)
	}
	known := hasComparisonProvenance(kind, b) && hasComparisonProvenance(kind, h)
	if !known && !slices.Contains(c.reasons, _scanIncomplete) {
		c.reasons = append(c.reasons, _scanIncomplete)
	}
	c.dbOnly = db && known && !profile && !rules && !scope && b.SHA == h.SHA && b.Complete() && h.Complete()
}

func hasComparisonProvenance(kind string, r *entity.TechnicalReport) bool {
	return r.Producer != "" && len(r.RulesHash) != 0 && len(r.ConfigHash) != 0 && len(r.ScopeHash) != 0 &&
		(kind != entity.AnalysisSCA && kind != entity.AnalysisImage || r.DBVersion != "")
}

type findingRule struct{ kind, rule string }

type comparisonFinding struct {
	ID                entity.ID
	Rule              findingRule
	Uncertain         bool
	DependencyChanged bool
}

type reportFindings struct {
	byID      map[entity.ID]comparisonFinding
	uncertain map[findingRule]bool
}

func comparisonFindings(r *entity.TechnicalReport, identities map[entity.ID]entity.OccurrenceIdentity) reportFindings {
	out := reportFindings{byID: make(map[entity.ID]comparisonFinding), uncertain: make(map[findingRule]bool)}
	if r == nil {
		return out
	}
	for i := range r.Occurrences {
		o := &r.Occurrences[i]
		id := identities[o.ID]
		rule := findingRule{kind: o.Kind, rule: o.RuleID}
		if id.FindingIdentityID.IsZero() {
			out.uncertain[rule] = true
			continue
		}
		entry := out.byID[id.FindingIdentityID]
		entry.ID, entry.Rule = id.FindingIdentityID, rule
		entry.Uncertain = entry.Uncertain || id.MatchKind == entity.MatchUncertain
		if entry.Uncertain {
			out.uncertain[rule] = true
		}
		entry.DependencyChanged = entry.DependencyChanged || r.DependencyDelta == entity.DependencyDeltaComputed &&
			(o.DependencyDelta == entity.DependencyAdded || o.DependencyDelta == entity.DependencyChanged)
		out.byID[entry.ID] = entry
	}
	return out
}

func compareFindings(out entity.TechnicalComparison, base, head *entity.TechnicalReport, causes *comparisonCauses, b, h *reportFindings) entity.TechnicalComparison {
	if len(b.uncertain)+len(h.uncertain) > 0 {
		causes.factor(_identityUncertain)
	}
	for id, finding := range h.byID {
		previous, present := b.byID[id]
		if finding.DependencyChanged && head.DependencyBaseSHA == out.Base.SHA && head.SHA != out.Base.SHA {
			causes.factor(_dependencyChanged)
		}
		uncertain := finding.Uncertain || previous.Uncertain || !present && b.uncertain[finding.Rule]
		state := entity.FindingNewlyObserved
		switch {
		case uncertain:
			state = entity.FindingUncertain
			causes.factor(_identityUncertain).FindingIDs = append(causes.factor(_identityUncertain).FindingIDs, id)
		case out.Comparable && present:
			state = entity.FindingUnchanged
		case out.Comparable:
			state = entity.FindingNew
			causes.recordFinding(base, head, finding, state)
		case causes.dbOnly && !present:
			causes.recordFinding(base, head, finding, entity.FindingNew)
		}
		out.Findings = append(out.Findings, entity.TechnicalFinding{ID: id, Kind: finding.Rule.kind, Lifecycle: state})
	}
	for id, finding := range b.byID {
		if _, present := h.byID[id]; present {
			continue
		}
		state := entity.FindingOpen
		switch {
		case finding.Uncertain || h.uncertain[finding.Rule]:
			state = entity.FindingUncertain
			causes.factor(_identityUncertain).FindingIDs = append(causes.factor(_identityUncertain).FindingIDs, id)
		case out.Comparable:
			state = entity.FindingResolved
			causes.recordFinding(base, head, finding, state)
		case causes.dbOnly:
			causes.recordFinding(base, head, finding, entity.FindingResolved)
		}
		out.Findings = append(out.Findings, entity.TechnicalFinding{ID: id, Kind: finding.Rule.kind, Lifecycle: state})
	}
	for _, kind := range []string{_codeChanged, _dependencyChanged, _rulesChanged, _dbChanged, _scopeChanged, _sourceRecovered, _sourceLost, _identityUncertain} {
		if f := causes.factors[kind]; f != nil {
			slices.SortFunc(f.FindingIDs, compareIDs)
			out.Factors = append(out.Factors, *f)
		}
	}
	slices.SortFunc(out.Findings, func(a, b entity.TechnicalFinding) int { return compareIDs(a.ID, b.ID) })
	return out
}

func (c *comparisonCauses) recordFinding(base, head *entity.TechnicalReport, f comparisonFinding, state string) {
	if c.dbOnly {
		addFactorFinding(c.factor(_dbChanged), f.ID, state)
		return
	}
	if f.DependencyChanged && state == entity.FindingNew && base.SHA != head.SHA && head.DependencyBaseSHA == base.SHA {
		addFactorFinding(c.factor(_dependencyChanged), f.ID, state)
	}
	if base.SHA != head.SHA && f.Rule.kind != entity.FindingDependency {
		addFactorFinding(c.factor(_codeChanged), f.ID, state)
	}
}

func addFactorFinding(f *entity.TechnicalFactor, id entity.ID, state string) {
	f.FindingIDs = append(f.FindingIDs, id)
	if state == entity.FindingNew {
		f.NewCount++
	} else {
		f.ResolvedCount++
	}
}
