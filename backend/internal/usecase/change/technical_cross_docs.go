package change

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// documentationComparisonKey records the provenance required to compare a repeat check.
func documentationComparisonKey(head *entity.TechnicalReport, identities []entity.OccurrenceIdentity) []byte {
	if !hasComparisonProvenance(entity.AnalysisDocs, head) ||
		slices.ContainsFunc(identities, func(i entity.OccurrenceIdentity) bool { return i.MatchKind == entity.MatchUncertain }) {
		return nil
	}
	raw, _ := json.Marshal(struct {
		Format, Language, Producer, Version, Profile, Mode string
		Scope, Rules, Config                               []byte
	}{head.Format, head.Language, head.Producer, head.ProducerVersion, head.ProfileID, head.Mode, head.ScopeHash, head.RulesHash, head.ConfigHash})
	sum := sha256.Sum256(raw)
	return sum[:]
}

// Recheck the same profile and stream against the last different head with an observed document check.
// An incomplete or incompatible check cannot establish that a violation disappeared.
func recheckDocumentation(row *entity.TechnicalSummaryRevision, history []entity.TechnicalSummaryRevision, at time.Time) []entity.TechnicalStream {
	streams := slices.Clone(row.Streams)
	for i := range streams {
		current := &streams[i]
		if !current.Complete || len(current.DocumentationKey) == 0 {
			continue
		}
		for j := range history {
			older := &history[j]
			if older.HeadSHA == row.HeadSHA || !older.AppliesAt(at) || older.ProjectID != row.ProjectID {
				continue
			}
			index := slices.IndexFunc(older.Streams, func(s entity.TechnicalStream) bool {
				return s.ExpectedReport == current.ExpectedReport
			})
			if index < 0 {
				continue
			}
			previous := &older.Streams[index]
			if previous.Complete && bytes.Equal(current.DocumentationKey, previous.DocumentationKey) {
				recheckViolations(current, previous)
			}
			break
		}
	}
	return streams
}

func recheckViolations(current, previous *entity.TechnicalStream) {
	prior := make(map[entity.ID]bool, len(previous.Findings))
	for i := range previous.Findings {
		f := &previous.Findings[i]
		if f.Lifecycle != entity.FindingResolved {
			prior[f.FindingID] = true
		}
	}
	found := make(map[entity.ID]bool, len(current.Findings))
	findings := make([]entity.SummaryFinding, 0, len(current.Findings))
	for i := range current.Findings {
		f := current.Findings[i]
		if f.Lifecycle == entity.FindingResolved {
			continue
		}
		f.Lifecycle = entity.FindingNew
		if prior[f.FindingID] {
			f.Lifecycle = entity.FindingUnchanged
		}
		found[f.FindingID] = true
		findings = append(findings, f)
	}
	for i := range previous.Findings {
		f := previous.Findings[i]
		if f.Lifecycle != entity.FindingResolved && !found[f.FindingID] {
			f.Lifecycle = entity.FindingResolved
			findings = append(findings, f)
		}
	}
	current.Comparable, current.Findings = true, findings
}
