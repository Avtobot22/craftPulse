package change

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _compareTechnicalResults = "compareTechnicalResults"

// CompareTechnicalResults checks repository and security access before comparing the requested revisions.
func (u *UseCase) CompareTechnicalResults(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.TechnicalComparisonQuery) (entity.TechnicalComparison, error) {
	c, err := u.d.Components.ConfirmedComponent(ctx, s.OrganizationID, q.ComponentID)
	if err != nil {
		return entity.TechnicalComparison{}, err
	}
	area := entity.FindingAccess{AreaID: c.RepositoryAreaID, Kind: entity.FindingCode}
	security := entity.FindingAccess{AreaID: c.RepositoryAreaID, Kind: entity.FindingSecurity}
	allowed, err := u.d.FindingRights.VisibleFindings(ctx, s, scope, _compareTechnicalResults, []entity.FindingAccess{area, security})
	if err != nil {
		return entity.TechnicalComparison{}, err
	}
	if !allowed[area] {
		return entity.TechnicalComparison{}, entity.ErrNotFound
	}
	if !allowed[security] && securityAnalysis(q.Kind) {
		return entity.TechnicalComparison{
			Base: technicalRevision(q.BaseSHA, nil), Head: technicalRevision(q.HeadSHA, nil),
			Notes: []entity.AccessNote{{Code: entity.NoteRestricted}},
		}, nil
	}
	reports := entity.TechnicalReportScope{AreaID: c.RepositoryAreaID, ComponentID: c.ID, Kind: q.Kind, SHAs: []string{q.BaseSHA, q.HeadSHA}}
	return u.compareTechnical(ctx, s.OrganizationID, q, reports, allowed[security])
}

// CompareMergeRequestReports reads the exact base and head of a configured report stream for the MR calculation.
func (u *UseCase) CompareMergeRequestReports(ctx context.Context, org entity.ID, q entity.TechnicalComparisonQuery, language, profileID string) (entity.TechnicalComparison, error) {
	c, err := u.d.Components.ConfirmedComponent(ctx, org, q.ComponentID)
	if err != nil {
		return entity.TechnicalComparison{}, err
	}
	reports := entity.TechnicalReportScope{
		AreaID: c.RepositoryAreaID, ComponentID: c.ID, Kind: q.Kind, SHAs: []string{q.BaseSHA, q.HeadSHA}, Language: &language, ProfileID: &profileID,
	}
	return u.compareTechnical(ctx, org, q, reports, true)
}

func securityAnalysis(kind string) bool {
	switch kind {
	case entity.AnalysisSAST, entity.AnalysisSCA, entity.AnalysisSecrets, entity.AnalysisIaC, entity.AnalysisImage:
		return true
	}
	return false
}

func (u *UseCase) compareTechnical(ctx context.Context, org entity.ID, q entity.TechnicalComparisonQuery, scope entity.TechnicalReportScope, security bool) (entity.TechnicalComparison, error) {
	var out entity.TechnicalComparison
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		base, head, err := u.pairReports(ctx, org, q, scope)
		if err != nil {
			return err
		}
		out, err = u.compareLoadedReports(ctx, org, q, base, head, security)
		return err
	})
	return out, err
}

func (u *UseCase) pairReports(ctx context.Context, org entity.ID, q entity.TechnicalComparisonQuery, scope entity.TechnicalReportScope) (base, head *entity.TechnicalReport, err error) {
	list, err := u.d.Reports.TechnicalReports(ctx, org, scope)
	if err != nil {
		return nil, nil, err
	}
	head = findTechnicalReport(list, func(r *entity.TechnicalReport) bool { return r.SHA == q.HeadSHA })
	match := func(r *entity.TechnicalReport) bool {
		return r.SHA == q.BaseSHA && (head == nil || r.ID != head.ID)
	}
	base = findTechnicalReport(list, func(r *entity.TechnicalReport) bool {
		return match(r) && r.Complete() && sameTechnicalRow(r, head)
	})
	if base != nil {
		return base, head, nil
	}
	base, err = u.technicalBaseline(ctx, org, q, scope, head)
	if err != nil || base != nil {
		return base, head, err
	}
	base = findTechnicalReport(list, func(r *entity.TechnicalReport) bool { return match(r) && sameTechnicalRow(r, head) })
	if base == nil && scope.ProfileID == nil {
		base = findTechnicalReport(list, match)
	}
	if base == nil && head != nil && head.SHA == q.BaseSHA && head.Complete() {
		base = head
	}
	return base, head, nil
}

func sameTechnicalRow(r, head *entity.TechnicalReport) bool {
	return head == nil || r.Language == head.Language && r.ProfileID == head.ProfileID
}

func findTechnicalReport(reports []entity.TechnicalReport, test func(*entity.TechnicalReport) bool) *entity.TechnicalReport {
	for i := range reports {
		if test(&reports[i]) {
			return &reports[i]
		}
	}
	return nil
}

func (u *UseCase) technicalBaseline(ctx context.Context, org entity.ID, q entity.TechnicalComparisonQuery, scope entity.TechnicalReportScope, head *entity.TechnicalReport) (*entity.TechnicalReport, error) {
	baselines, err := u.d.Repo.CurrentBaselines(ctx, org, []entity.ID{scope.ComponentID})
	if err != nil {
		return nil, err
	}
	scope.SHAs = nil
	for i := range baselines {
		b := &baselines[i]
		if b.AnalysisKind == q.Kind && (head == nil || b.Language == head.Language && b.ReportID != head.ID) {
			scope.ReportIDs = append(scope.ReportIDs, b.ReportID)
		}
	}
	if len(scope.ReportIDs) == 0 {
		return nil, nil
	}
	list, err := u.d.Reports.TechnicalReports(ctx, org, scope)
	if err != nil {
		return nil, err
	}
	return findTechnicalReport(list, func(r *entity.TechnicalReport) bool {
		return r.SHA == q.BaseSHA && r.Complete() && sameTechnicalRow(r, head)
	}), nil
}

func (u *UseCase) compareLoadedReports(ctx context.Context, org entity.ID, q entity.TechnicalComparisonQuery, base, head *entity.TechnicalReport, security bool) (entity.TechnicalComparison, error) {
	reports := []*entity.TechnicalReport{base, head}
	ids := make([]entity.ID, 0, len(reports))
	for _, r := range reports {
		if r != nil && !r.Rejected && !slices.Contains(ids, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	occurrences, err := u.d.Reports.TechnicalOccurrences(ctx, org, ids)
	if err != nil {
		return entity.TechnicalComparison{}, err
	}
	var keys []entity.ID
	var restricted bool
	for _, r := range reports {
		if r == nil {
			continue
		}
		r.Occurrences = nil
		found := occurrences[r.ID]
		for i := range found {
			o := &found[i]
			if !security && entity.SensitiveFinding(o.Kind) {
				restricted = true
				continue
			}
			r.Occurrences = append(r.Occurrences, *o)
			keys = append(keys, o.ID)
		}
	}
	identities, err := u.d.Repo.OccurrenceIdentities(ctx, org, keys)
	if err != nil {
		return entity.TechnicalComparison{}, err
	}
	out := CompareReports(q, base, head, identities)
	if restricted {
		out.Notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
	}
	return out, nil
}
