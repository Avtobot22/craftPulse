package change

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _identityMethod is the version of the rules that match the findings of reports to canonical findings; another rule
// is a new version, and its identities do not meet the ones of this one.
const _identityMethod = 1

// Reports is normalize: the reports of a receipt with their findings and the reports and branches a baseline reads.
type Reports interface {
	ReceiptReports(ctx context.Context, orgID, receiptID entity.ID) ([]entity.ReportFindings, error)
	ReportsByID(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ReportFindings, error)
	LastBranchReports(ctx context.Context, orgID entity.ID, queries []entity.BranchReportQuery) ([]entity.ReportFindings, error)
	DefaultBranches(ctx context.Context, orgID entity.ID, repositories []entity.ID) (map[entity.ID]string, error)
}

// Complete ends the job in the transaction of its result.
type Complete = func(ctx context.Context) error

// RunIdentify is the job change.identify: the findings of the reports of a receipt get their canonical findings and a
// complete successful report of the main branch becomes the baseline of its row of the matrix. A finding identified
// before keeps its identity, so the job may run again.
func (u *UseCase) RunIdentify(ctx context.Context, job entity.ChangeIdentifyJob, complete Complete) error {
	org := job.OrganizationID
	reports, err := u.d.Reports.ReceiptReports(ctx, org, job.ReceiptID)
	if err != nil {
		return err
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if len(reports) > 0 {
			seq, err := u.d.Tenants.NextSeq(ctx, org)
			if err != nil {
				return err
			}
			for i := range reports {
				if err := u.identify(ctx, org, &reports[i], seq); err != nil {
					return err
				}
				if err := u.baseline(ctx, org, &reports[i]); err != nil {
					return err
				}
			}
		}
		return complete(ctx)
	})
}

// identify matches the findings of a report not identified yet (BR-011). The same fingerprint in the area is exact.
// Without it, the same rule and content elsewhere is moved when one canonical finding has them and uncertain when
// several do; uncertain takes the oldest, so the finding gets no second canonical one. A finding of neither path nor
// package is uncertain. Any other finding starts a canonical finding of its own.
func (u *UseCase) identify(ctx context.Context, org entity.ID, r *entity.ReportFindings, seq int64) error {
	if len(r.Occurrences) == 0 {
		return nil
	}
	ids := make([]entity.ID, len(r.Occurrences))
	for i := range r.Occurrences {
		ids[i] = r.Occurrences[i].ID
	}
	assigned, err := u.d.Repo.AssignedOccurrences(ctx, org, ids)
	if err != nil {
		return err
	}
	pending := slices.DeleteFunc(slices.Clone(r.Occurrences), func(o entity.ReportOccurrence) bool { return slices.Contains(assigned, o.ID) })
	if len(pending) == 0 {
		return nil
	}
	fingerprints, contents := make([][]byte, 0, len(pending)), [][]byte{}
	for i := range pending {
		fingerprints = append(fingerprints, pending[i].Fingerprint)
		if c := pending[i].ContentFingerprint; c != nil {
			contents = append(contents, c)
		}
	}
	known, err := u.d.Repo.IdentitiesByFingerprint(ctx, org, r.AreaID, _identityMethod, fingerprints)
	if err != nil {
		return err
	}
	var candidates []entity.FindingIdentity
	if len(contents) > 0 {
		if candidates, err = u.d.Repo.IdentitiesByContent(ctx, org, r.AreaID, _identityMethod, contents); err != nil {
			return err
		}
	}
	matched := map[entity.ID]bool{}
	for _, id := range known {
		matched[id] = true
	}
	at := reportTime(r, u.d.Clock.Now())
	out := make([]entity.OccurrenceIdentity, 0, len(pending))
	for i := range pending {
		o := &pending[i]
		if id, ok := known[string(o.Fingerprint)]; ok {
			out = append(out, entity.OccurrenceIdentity{OccurrenceID: o.ID, FindingIdentityID: id, MatchKind: entity.MatchExact})
			continue
		}
		same := slices.DeleteFunc(slices.Clone(candidates), func(c entity.FindingIdentity) bool {
			return o.ContentFingerprint == nil || matched[c.ID] || c.RuleID != o.RuleID || !bytes.Equal(c.ContentFingerprint, o.ContentFingerprint)
		})
		if len(same) > 0 {
			kind := entity.MatchMoved
			if len(same) > 1 {
				kind = entity.MatchUncertain
			}
			out = append(out, entity.OccurrenceIdentity{OccurrenceID: o.ID, FindingIdentityID: same[0].ID, MatchKind: kind})
			continue
		}
		id, err := u.d.Repo.InsertFindingIdentity(ctx, org, &entity.FindingIdentity{
			AreaID: r.AreaID, MethodVersion: _identityMethod, Fingerprint: o.Fingerprint, ContentFingerprint: o.ContentFingerprint,
			RuleID: o.RuleID, Kind: o.Kind, FirstReportID: r.ID, FirstSeenAt: at,
		})
		if err != nil {
			return err
		}
		known[string(o.Fingerprint)], matched[id] = id, true
		kind := entity.MatchExact
		if o.Path == "" && o.PackagePURL == "" {
			kind = entity.MatchUncertain
		}
		out = append(out, entity.OccurrenceIdentity{OccurrenceID: o.ID, FindingIdentityID: id, MatchKind: kind})
	}
	return u.d.Repo.InsertOccurrenceIdentities(ctx, org, out, at, seq)
}

// baseline makes a report the baseline of its row of the matrix: a report of the main branch of its repository,
// complete, with success or findings, of an attempt that ran to its end and of a row of the matrix in force. A
// partial, failed or interrupted run gives no baseline and leaves the one in force; a report that ran before the
// baseline in force, or at the same moment but became known earlier, does not replace it (FR-035 A3, V-24).
func (u *UseCase) baseline(ctx context.Context, org entity.ID, r *entity.ReportFindings) error {
	if r.ComponentID == nil || r.DefaultBranch == "" || r.RefName != r.DefaultBranch || r.Completeness != "complete" || r.Interrupted ||
		(r.ExitClass != "success" && r.ExitClass != "findings") {
		return nil
	}
	version, ok, err := u.rowVersion(ctx, org, r)
	if err != nil || !ok {
		return err
	}
	b := entity.Baseline{
		ComponentID: *r.ComponentID, Language: r.Language, AnalysisKind: r.Kind, ProfileVersionID: version, ReportID: r.ID,
		EstablishedAt: reportTime(r, u.d.Clock.Now()),
	}
	current, err := u.d.Repo.LockCurrentBaseline(ctx, org, b.ComponentID, b.Language, b.AnalysisKind)
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return u.d.Repo.ReplaceBaseline(ctx, org, nil, &b)
	case err != nil:
		return err
	case current.ReportID == r.ID || current.EstablishedAt.After(b.EstablishedAt):
		return nil
	case current.EstablishedAt.Equal(b.EstablishedAt):
		older, err := u.d.Reports.ReportsByID(ctx, org, []entity.ID{current.ReportID})
		if err != nil {
			return err
		}
		if len(older) == 1 && older[0].KnownSeq >= r.KnownSeq {
			return nil
		}
	}
	return u.d.Repo.ReplaceBaseline(ctx, org, &current, &b)
}

// rowVersion is the version of the matrix in force of a project of the repository of a report that has the row of
// its component, language and kind.
func (u *UseCase) rowVersion(ctx context.Context, org entity.ID, r *entity.ReportFindings) (entity.ID, bool, error) {
	links, err := u.d.Components.ProjectAreas(ctx, org, u.d.Clock.Now())
	if err != nil {
		return entity.ID{}, false, err
	}
	var projects []entity.ID
	for _, l := range links {
		if l.To == r.AreaID && !slices.Contains(projects, l.From) {
			projects = append(projects, l.From)
		}
	}
	slices.SortFunc(projects, compareIDs)
	for _, p := range projects {
		v, err := u.d.Repo.LatestProfileVersion(ctx, org, p)
		if errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return entity.ID{}, false, err
		}
		if slices.ContainsFunc(v.Entries, func(e entity.ProfileEntry) bool {
			return e.ComponentID == *r.ComponentID && e.Language == r.Language && e.AnalysisKind == r.Kind
		}) {
			return v.ID, true, nil
		}
	}
	return entity.ID{}, false, nil
}

// reportTime is when a report ran: its end, its start, or else the moment it is read.
func reportTime(r *entity.ReportFindings, now time.Time) time.Time {
	switch {
	case r.FinishedAt != nil:
		return *r.FinishedAt
	case r.StartedAt != nil:
		return *r.StartedAt
	}
	return now
}

// Baselines are the baselines of the rows of the matrix in force of a project: ready with the report and its SHA,
// pending without a complete run of the main branch, failed when the last run of the main branch failed (UI-52).
func (u *UseCase) Baselines(ctx context.Context, s entity.Subject, projectID entity.ID) ([]entity.BaselineState, error) {
	org := s.OrganizationID
	var out []entity.BaselineState
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		v, err := u.d.Repo.LatestProfileVersion(ctx, org, projectID)
		if errors.Is(err, entity.ErrNotFound) {
			out = []entity.BaselineState{}
			return nil
		}
		if err != nil {
			return err
		}
		components, err := u.d.Components.ProjectComponents(ctx, org, projectID, u.d.Clock.Now())
		if err != nil {
			return err
		}
		out, err = u.baselineStates(ctx, org, v.Entries, components)
		return err
	})
	return out, err
}

func (u *UseCase) baselineStates(ctx context.Context, org entity.ID, entries []entity.ProfileEntry, components []entity.Component) ([]entity.BaselineState, error) {
	areaOf := make(map[entity.ID]entity.ID, len(components))
	var areas []entity.ID
	ids := make([]entity.ID, 0, len(components))
	for i := range components {
		c := &components[i]
		areaOf[c.ID] = c.RepositoryAreaID
		ids = append(ids, c.ID)
		if !slices.Contains(areas, c.RepositoryAreaID) {
			areas = append(areas, c.RepositoryAreaID)
		}
	}
	branches, err := u.d.Reports.DefaultBranches(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	baselines, err := u.d.Repo.CurrentBaselines(ctx, org, ids)
	if err != nil {
		return nil, err
	}
	reportIDs := make([]entity.ID, len(baselines))
	for i := range baselines {
		reportIDs[i] = baselines[i].ReportID
	}
	reports, err := u.d.Reports.ReportsByID(ctx, org, reportIDs)
	if err != nil {
		return nil, err
	}
	var queries []entity.BranchReportQuery
	for _, e := range entries {
		if branch := branches[areaOf[e.ComponentID]]; branch != "" {
			queries = append(queries, entity.BranchReportQuery{ComponentID: e.ComponentID, Language: e.Language, Kind: e.AnalysisKind, RefName: branch})
		}
	}
	last, err := u.d.Reports.LastBranchReports(ctx, org, queries)
	if err != nil {
		return nil, err
	}
	out := make([]entity.BaselineState, 0, len(entries))
	for _, e := range entries {
		st := entity.BaselineState{ComponentID: e.ComponentID, Language: e.Language, AnalysisKind: e.AnalysisKind, Branch: branches[areaOf[e.ComponentID]]}
		if i := slices.IndexFunc(baselines, func(b entity.Baseline) bool { return sameRow(e, b.ComponentID, b.Language, b.AnalysisKind) }); i >= 0 {
			b := &baselines[i]
			st.State, st.ReportID, st.EstablishedAt = entity.BaselineReady, &b.ReportID, &b.EstablishedAt
			if k := slices.IndexFunc(reports, func(r entity.ReportFindings) bool { return r.ID == b.ReportID }); k >= 0 {
				st.SHA = reports[k].SHA
			}
			out = append(out, st)
			continue
		}
		st.State, st.ReasonCode = entity.BaselinePending, entity.BaselineRunMissing
		if k := slices.IndexFunc(last, func(r entity.ReportFindings) bool { return sameRow(e, *r.ComponentID, r.Language, r.Kind) }); k >= 0 {
			switch r := &last[k]; {
			case r.ExitClass == "failed" || r.ExitClass == "timeout":
				st.State, st.ReasonCode = entity.BaselineFailed, entity.BaselineScanFailed
			case r.Completeness != "complete" || r.ExitClass != "success" && r.ExitClass != "findings":
				st.ReasonCode = entity.BaselineScanIncomplete
			}
		}
		out = append(out, st)
	}
	return out, nil
}

func sameRow(e entity.ProfileEntry, component entity.ID, language, kind string) bool {
	return e.ComponentID == component && e.Language == language && e.AnalysisKind == kind
}

// FindingIdentity is a canonical finding by its ID with its area and kind; entity.ErrNotFound without one.
func (u *UseCase) FindingIdentity(ctx context.Context, orgID, id entity.ID) (entity.FindingIdentity, error) {
	var out entity.FindingIdentity
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.FindingIdentity(ctx, orgID, id)
		return err
	})
	return out, err
}

// OccurrenceIdentities are the canonical findings and the match of the findings of reports among ids.
func (u *UseCase) OccurrenceIdentities(ctx context.Context, orgID entity.ID, occurrences []entity.ID) ([]entity.OccurrenceIdentity, error) {
	var out []entity.OccurrenceIdentity
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.OccurrenceIdentities(ctx, orgID, occurrences)
		return err
	})
	return out, err
}

// CurrentBaselines are the baselines in force of components, by component, language and kind of analysis.
func (u *UseCase) CurrentBaselines(ctx context.Context, orgID entity.ID, components []entity.ID) ([]entity.Baseline, error) {
	var out []entity.Baseline
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CurrentBaselines(ctx, orgID, components)
		return err
	})
	return out, err
}
